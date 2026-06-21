package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

var sourceHTTPClient = &http.Client{Timeout: 30 * time.Second}

type spec struct {
	Paths map[string]map[string]operation `json:"paths"`
}

type operation struct {
	Tags        []string            `json:"tags"`
	Summary     string              `json:"summary"`
	Parameters  []parameter         `json:"parameters"`
	RequestBody *requestBody        `json:"requestBody"`
	Responses   map[string]response `json:"responses"`
}

type requestBody struct {
	Required bool             `json:"required"`
	Content  map[string]media `json:"content"`
}

type response struct {
	Content map[string]media `json:"content"`
}

type media struct {
	Schema map[string]any `json:"schema"`
}

type parameter struct {
	Name        string         `json:"name"`
	In          string         `json:"in"`
	Description string         `json:"description"`
	Required    bool           `json:"required"`
	Schema      map[string]any `json:"schema"`
}

type registry struct {
	Version  string    `json:"version"`
	Source   string    `json:"source"`
	Services []service `json:"services"`
}

type service struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Domain      string   `json:"domain"`
	Methods     []method `json:"methods"`
}

type method struct {
	Name                  string   `json:"name"`
	CommandName           string   `json:"commandName"`
	HTTPMethod            string   `json:"httpMethod"`
	FullPath              string   `json:"fullPath"`
	Summary               string   `json:"summary"`
	Params                []param  `json:"params,omitempty"`
	Pagination            paging   `json:"pagination"`
	RiskLevel             string   `json:"riskLevel"`
	RequiresOpenChannelID bool     `json:"requiresOpenChannelId"`
	ContentType           string   `json:"contentType,omitempty"`
	RequestBodyRequired   bool     `json:"requestBodyRequired,omitempty"`
	RequestBodySchema     any      `json:"requestBodySchema,omitempty"`
	SuccessCodes          []int    `json:"successCodes,omitempty"`
	TableColumns          []string `json:"tableColumns,omitempty"`
	ResponseSchema        any      `json:"responseSchema,omitempty"`
}

type param struct {
	Name        string `json:"name"`
	Flag        string `json:"flag"`
	Location    string `json:"location"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
	Format      string `json:"format,omitempty"`
	Default     any    `json:"default,omitempty"`
	Enum        []any  `json:"enum,omitempty"`
	Min         int    `json:"min,omitempty"`
	Max         int    `json:"max,omitempty"`
}

type paging struct {
	Type       string `json:"type"`
	MaxRows    int    `json:"maxRows,omitempty"`
	TotalField string `json:"totalField,omitempty"`
	RangeType  string `json:"rangeType,omitempty"`
	WindowDays int    `json:"windowDays,omitempty"`
}

func main() {
	source := flag.String("source", "https://doc.captainbi.com/openapi.json", "OpenAPI JSON source")
	out := flag.String("out", "internal/registry/captainbi_meta.json", "registry output path")
	docs := flag.String("docs", "docs/endpoints.md", "endpoints markdown output path")
	flag.Parse()

	body, err := readSource(*source)
	must(err)
	var s spec
	must(json.Unmarshal(body, &s))
	reg := buildRegistry(*source, s)
	b, err := json.MarshalIndent(reg, "", "  ")
	must(err)
	// #nosec G302,G306 -- generated Registry and endpoint documentation are public repository artifacts.
	must(os.WriteFile(*out, append(b, '\n'), 0o644))
	// #nosec G302 -- generated Registry metadata is a public repository artifact.
	must(os.Chmod(*out, 0o644))
	// #nosec G306 -- generated endpoint documentation is a public repository artifact.
	must(os.WriteFile(*docs, []byte(renderDocs(reg)), 0o644))
	// #nosec G302 -- generated endpoint documentation is a public repository artifact.
	must(os.Chmod(*docs, 0o644))
}

func readSource(source string) ([]byte, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		resp, err := sourceHTTPClient.Get(source)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("GET %s returned HTTP %d", source, resp.StatusCode)
		}
		const maxSpecBytes = 32 << 20
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxSpecBytes+1))
		if err != nil {
			return nil, err
		}
		if len(body) > maxSpecBytes {
			return nil, fmt.Errorf("OpenAPI response exceeds %d bytes", maxSpecBytes)
		}
		return body, nil
	}
	// #nosec G304 -- local source is an explicit developer CLI argument, never Agent-derived API data.
	return os.ReadFile(source)
}

func buildRegistry(source string, s spec) registry {
	services := map[string]*service{
		"goods":   {Name: "goods", DisplayName: "商品信息", Domain: "goods"},
		"sales":   {Name: "sales", DisplayName: "销售数据", Domain: "sales"},
		"finance": {Name: "finance", DisplayName: "财务数据", Domain: "finance"},
		"fba":     {Name: "fba", DisplayName: "FBA 数据", Domain: "fba"},
		"ads":     {Name: "ads", DisplayName: "广告数据", Domain: "ads"},
		"monitor": {Name: "monitor", DisplayName: "监控与口碑", Domain: "monitor"},
	}
	paths := make([]string, 0, len(s.Paths))
	for p := range s.Paths {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		for httpMethod, op := range s.Paths[p] {
			domain := domainFor(p, op)
			m := method{
				Name:                  strings.Trim(strings.ReplaceAll(p, "/", "."), "."),
				CommandName:           commandNameFor(p, op.Summary),
				HTTPMethod:            strings.ToUpper(httpMethod),
				FullPath:              p,
				Summary:               op.Summary,
				RiskLevel:             riskFor(p, strings.ToUpper(httpMethod)),
				RequiresOpenChannelID: requiresOpenChannel(op.Parameters),
				TableColumns:          tableColumnsFor(domain, p),
				ResponseSchema:        responseSchemaFor(op, domain, p),
				SuccessCodes:          []int{200},
			}
			for _, raw := range op.Parameters {
				if strings.EqualFold(raw.Name, "authorization") {
					continue
				}
				generated := param{
					Name:        raw.Name,
					Flag:        flagName(raw.Name),
					Location:    raw.In,
					Type:        fmt.Sprint(raw.Schema["type"]),
					Required:    raw.Required,
					Description: raw.Description,
					Format:      inferFormat(raw.Name, fmt.Sprint(raw.Schema["type"])),
					Enum:        anySlice(raw.Schema["enum"]),
				}
				applyParamDefaults(&generated)
				m.Params = append(m.Params, generated)
			}
			if contentType, bodySchema, ok := requestSchema(op.RequestBody); ok {
				if m.HTTPMethod != "GET" && m.HTTPMethod != "HEAD" {
					m.ContentType = normalizeContentType(contentType)
				}
				applyBodyOverrides(p, bodySchema)
				m.RequestBodySchema = bodySchema
				m.RequestBodyRequired = m.HTTPMethod != "GET" && m.HTTPMethod != "HEAD" && (op.RequestBody.Required || bodyRequiredFor(p))
				required := requiredSet(bodySchema, requiredBodyFields(p, bodySchema))
				if properties, ok := bodySchema["properties"].(map[string]any); ok {
					names := make([]string, 0, len(properties))
					for name := range properties {
						names = append(names, name)
					}
					sort.Strings(names)
					for _, name := range names {
						rawSchema, _ := properties[name].(map[string]any)
						typ := fmt.Sprint(rawSchema["type"])
						if typ == "array" || typ == "object" {
							continue
						}
						location := "form"
						if m.HTTPMethod == "GET" || m.HTTPMethod == "HEAD" {
							location = "query"
						}
						generated := param{
							Name:        name,
							Flag:        flagName(name),
							Location:    location,
							Type:        typ,
							Required:    required[name],
							Description: fmt.Sprint(rawSchema["description"]),
							Format:      inferFormat(name, typ),
							Enum:        anySlice(rawSchema["enum"]),
						}
						applyParamDefaults(&generated)
						m.Params = removeNonHeaderParam(m.Params, name)
						m.Params = append(m.Params, generated)
					}
				}
			}
			m.Pagination = paginationFor(m.Params)
			services[domain].Methods = append(services[domain].Methods, m)
		}
	}
	order := []string{"goods", "sales", "finance", "fba", "ads", "monitor"}
	reg := registry{Version: "0.3.0", Source: source}
	for _, name := range order {
		sort.Slice(services[name].Methods, func(i, j int) bool {
			return services[name].Methods[i].CommandName < services[name].Methods[j].CommandName
		})
		reg.Services = append(reg.Services, *services[name])
	}
	return reg
}

func domainFor(path string, op operation) string {
	if strings.Contains(path, "/open_cpc/") {
		return "ads"
	}
	if strings.Contains(path, "/open_order/") {
		return "sales"
	}
	if strings.Contains(path, "/open_fba/") || strings.Contains(path, "storage_fee") {
		return "fba"
	}
	if strings.Contains(path, "get_business_report") || strings.Contains(path, "monitoring") || strings.Contains(path, "reviews") || strings.Contains(path, "feedback") || strings.Contains(path, "hijacked") {
		return "monitor"
	}
	if strings.Contains(path, "/open_channel_finance/") || strings.Contains(path, "/open_finance/") || strings.Contains(path, "/open_goods_finance/") {
		return "finance"
	}
	return "goods"
}

func commandNameFor(path, summary string) string {
	overrides := map[string]string{
		"/v1/open_user/get_child_list":                                "operators",
		"/v1/open_user/get_channel_list":                              "shops",
		"/v1/open_user/get_site_list":                                 "sites",
		"/v1/open_user/set_channel_operation_mode":                    "set-shop-operation-mode",
		"/v1/open_goods/get_goods_list":                               "list",
		"/v1/open_goods/get_goods_item_list":                          "items",
		"/v1/open_goods/set_goods_operate_user":                       "set-operate-user",
		"/v1/open_goods/set_goods_group":                              "set-group",
		"/v1/open_goods/edit_goods_group":                             "edit-group",
		"/v1/open_goods_relevant/get_group_list":                      "groups",
		"/v1/open_goods_relevant/get_tags_list":                       "tags",
		"/v1/open_order/get_order_list":                               "orders",
		"/v1/open_order/get_return_report":                            "returns",
		"/v1/open_order/get_refund_list":                              "refunds",
		"/v1/open_order/upload_fbm_order_ship_info":                   "upload-fbm-shipping",
		"/v1/open_order/get_fbm_order_ship_info":                      "fbm-shipping-status",
		"/v1/open_channel_finance/get_analysis_by_order":              "store-daily",
		"/v1/open_channel_finance/get_month_analysis_by_order":        "store-monthly",
		"/v1/open_channel_finance/get_analysis_by_finance":            "store-daily-finance",
		"/v1/open_channel_finance/get_month_analysis_by_finance":      "store-monthly-finance",
		"/v1/open_channel_finance/get_transaction_data":               "store-transactions",
		"/v1/open_channel_finance/get_vat_report_list":                "vat",
		"/v1/open_finance/operating_expenses_breakdown":               "operating-expenses",
		"/v1/open_finance/get_payment_record":                         "payment-record",
		"/v1/open_finance/get_storewide_performance":                  "storewide-performance",
		"/v1/open_finance/get_store_performance":                      "store-performance",
		"/v1/open_finance/get_classify":                               "classify",
		"/v1/open_finance/set_rule":                                   "set-rule",
		"/v1/open_finance/set_goods_cost":                             "set-cost",
		"/v1/open_goods_finance/get_analysis_by_order":                "asin-daily",
		"/v1/open_goods_finance/get_month_analysis_by_order":          "asin-monthly",
		"/v1/open_goods_finance/get_analysis_by_finance":              "asin-daily-finance",
		"/v1/open_goods_finance/get_month_analysis_by_finance":        "asin-monthly-finance",
		"/v1/open_goods_finance/get_transaction_data":                 "asin-transactions",
		"/v1/open_goods_finance/get_claim_list":                       "claims",
		"/v1/open_finance/get_amazon_finance_storage_fee_report_list": "storage-fee",
		"/v1/open_fba/inventory_list":                                 "inventory",
		"/v1/open_fba/abnormal_distribution_fee":                      "abnormal-fee",
		"/v1/open_fba/get_amazon_shipment_list":                       "shipments",
		"/v1/open_fba/get_amazon_asin_monitor_list":                   "asin-monitor",
		"/v1/open_fba/sync_shipment":                                  "sync-shipment",
		"/v1/open_goods/get_business_report":                          "business-report",
		"/v1/open_goods/get_monitoring_list":                          "bad-review-summary",
		"/v1/open_goods/get_reviews_list":                             "reviews",
		"/v1/open_goods/get_feedback_monitoring":                      "feedback",
		"/v1/open_goods/get_followup_monitoring_list":                 "followup",
		"/v1/open_goods/get_hijacked_record":                          "hijacked-record",
	}
	if v, ok := overrides[path]; ok {
		return v
	}
	name := path[strings.LastIndex(path, "/")+1:]
	name = strings.TrimPrefix(name, "get_")
	name = strings.TrimPrefix(name, "set_")
	name = strings.ReplaceAll(name, "_", "-")
	return strings.Trim(name, "-")
}

func riskFor(path, httpMethod string) string {
	if httpMethod == "GET" {
		return "read"
	}
	switch path {
	case "/v1/open_user/set_channel_operation_mode", "/v1/open_goods/set_goods_operate_user":
		return "write_safe"
	case "/v1/open_fba/sync_shipment":
		return "sync_trigger"
	default:
		return "write_dangerous"
	}
}

func requiresOpenChannel(params []parameter) bool {
	for _, p := range params {
		if strings.EqualFold(p.Name, "OpenChannelId") {
			return true
		}
	}
	return false
}

func paginationFor(params []param) paging {
	names := map[string]bool{}
	for _, p := range params {
		names[p.Name] = true
	}
	result := paging{Type: "none"}
	if names["page"] && names["rows"] {
		result = paging{Type: "page_rows", MaxRows: 100, TotalField: "max_result"}
	}
	if names["start_modified_time"] || names["end_modified_time"] || names["start_report_time"] || names["end_report_time"] {
		result.RangeType = "modified_time_window"
		result.WindowDays = 31
		return result
	}
	if names["report_date"] {
		result.RangeType = "report_date"
		return result
	}
	return result
}

func tableColumnsFor(domain, path string) []string {
	switch {
	case strings.Contains(path, "get_channel_list"):
		return []string{"title", "open_channel_id", "site_id", "status"}
	case strings.Contains(path, "get_site_list"):
		return []string{"site_id", "site_name", "currency_code", "code"}
	case domain == "goods":
		return []string{"id", "sku", "asin", "title", "status", "modified_time"}
	case domain == "sales":
		return []string{"AmazonOrderId", "OrderStatus", "PurchaseDate", "Money_Amount", "Money_CurrencyCode"}
	case domain == "finance":
		return []string{"sku", "channel_id", "time", "sale_sales_quota", "cost_profit_profit", "cost_profit_profit_rate"}
	case domain == "fba":
		return []string{"sku", "asin", "title", "available_stock", "fulfillable_quantity"}
	case domain == "ads":
		return []string{"campaignId", "adGroupId", "impressions", "clicks", "cost", "acos", "roas"}
	case domain == "monitor":
		return []string{"title", "asin", "channel_id", "status", "modified_time"}
	default:
		return nil
	}
}

func responseSchemaFor(op operation, domain, path string) any {
	if success, ok := op.Responses["200"]; ok {
		if content, ok := success.Content["application/json"]; ok && len(content.Schema) > 0 {
			return content.Schema
		}
		for _, content := range success.Content {
			if len(content.Schema) > 0 {
				return content.Schema
			}
		}
	}
	props := map[string]any{
		"code": map[string]any{"type": "integer"},
		"msg":  map[string]any{"type": "string"},
		"data": map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
	}
	if strings.Contains(path, "get_storewide_performance") || strings.Contains(path, "get_store_performance") {
		props["data"] = map[string]any{"type": "object"}
	}
	columns := tableColumnsFor(domain, path)
	if len(columns) > 0 {
		itemProps := map[string]any{}
		for _, col := range columns {
			itemProps[col] = map[string]any{"type": "string"}
		}
		props["data"] = map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "object", "properties": itemProps},
		}
	}
	if strings.Contains(path, "get_channel_list") {
		props["data"] = arraySchema(map[string]string{"title": "string", "open_channel_id": "string", "site_id": "integer", "status": "integer"})
	}
	if strings.Contains(path, "get_site_list") {
		props["data"] = arraySchema(map[string]string{"site_id": "integer", "site_name": "string", "currency_code": "string", "code": "string"})
	}
	return map[string]any{"type": "object", "properties": props}
}

func requestSchema(body *requestBody) (string, map[string]any, bool) {
	if body == nil || len(body.Content) == 0 {
		return "", nil, false
	}
	for _, preferred := range []string{"application/form-data", "multipart/form-data", "application/json"} {
		if content, ok := body.Content[preferred]; ok && len(content.Schema) > 0 {
			return preferred, content.Schema, true
		}
	}
	keys := make([]string, 0, len(body.Content))
	for key := range body.Content {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if schema := body.Content[key].Schema; len(schema) > 0 {
			return key, schema, true
		}
	}
	return "", nil, false
}

func normalizeContentType(value string) string {
	if value == "application/form-data" {
		return "multipart/form-data"
	}
	return value
}

func requiredSet(schema map[string]any, overrides []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range anySlice(schema["required"]) {
		if name, ok := value.(string); ok {
			out[name] = true
		}
	}
	for _, name := range overrides {
		out[name] = true
	}
	if len(out) > 0 {
		required := make([]string, 0, len(out))
		for name := range out {
			required = append(required, name)
		}
		sort.Strings(required)
		schema["required"] = required
	}
	return out
}

func applyBodyOverrides(path string, schema map[string]any) {
	properties, _ := schema["properties"].(map[string]any)
	setNestedRequired := func(property string, fields []string) {
		container, _ := properties[property].(map[string]any)
		if len(container) == 0 {
			return
		}
		container["minItems"] = 1
		items, _ := container["items"].(map[string]any)
		if len(items) > 0 {
			items["required"] = fields
		}
	}
	switch path {
	case "/v1/open_order/upload_fbm_order_ship_info":
		setNestedRequired("data", []string{"amazon_order_id", "carrier_code", "shipping_method", "shipper_tracking_number", "amazon_order_item_code", "quantity"})
	case "/v1/open_finance/set_goods_cost":
		setNestedRequired("data", []string{"sku", "purchasing_cost_currency_code", "fba_cost_currency_code", "fbm_cost_currency_code", "purchasing_cost", "fba_cost", "fbm_cost"})
	case "/v1/open_finance/set_rule":
		if endDate, ok := properties["jieshuriqi"].(map[string]any); ok {
			endDate["type"] = "string"
		}
	}
}

func requiredBodyFields(path string, schema map[string]any) []string {
	switch path {
	case "/v1/open_goods/set_goods_operate_user":
		return []string{"goods_id", "operation_user_admin_id"}
	case "/v1/open_goods/set_goods_group":
		return []string{"goods_id", "group_id"}
	case "/v1/open_goods/edit_goods_group":
		return []string{"group_name"}
	case "/v1/open_order/upload_fbm_order_ship_info", "/v1/open_finance/set_goods_cost":
		return []string{"data"}
	case "/v1/open_fba/sync_shipment":
		return []string{"shipment_ids"}
	}
	if strings.Contains(path, "_report") || strings.Contains(path, "month_analysis") {
		properties, _ := schema["properties"].(map[string]any)
		if _, ok := properties["report_date"]; ok {
			return []string{"report_date"}
		}
	}
	return nil
}

func bodyRequiredFor(path string) bool {
	if path == "/v1/open_user/set_channel_operation_mode" {
		return false
	}
	return strings.Contains(path, "/set_") || strings.Contains(path, "/edit_") || strings.Contains(path, "/upload_") || strings.Contains(path, "/sync_")
}

func applyParamDefaults(p *param) {
	if p.Name == "page" {
		p.Default = 1
		p.Min = 1
	}
	if p.Name == "rows" {
		p.Default = 100
		p.Min = 1
		p.Max = 100
	}
}

func inferFormat(name, typ string) string {
	name = strings.ToLower(name)
	if strings.Contains(name, "modified_time") {
		return "unix_seconds"
	}
	if name == "report_date" || strings.HasSuffix(name, "_date") || strings.Contains(name, "riqi") {
		return "date"
	}
	if typ == "integer" {
		return "int"
	}
	if typ == "number" {
		return "number"
	}
	if typ == "boolean" {
		return "boolean"
	}
	return "string"
}

func anySlice(value any) []any {
	if values, ok := value.([]any); ok {
		return values
	}
	if values, ok := value.([]string); ok {
		out := make([]any, 0, len(values))
		for _, value := range values {
			out = append(out, value)
		}
		return out
	}
	return nil
}

func removeNonHeaderParam(params []param, name string) []param {
	out := params[:0]
	for _, p := range params {
		if p.Name == name && p.Location != "header" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func arraySchema(fields map[string]string) map[string]any {
	props := map[string]any{}
	for k, typ := range fields {
		props[k] = map[string]any{"type": typ}
	}
	return map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": props}}
}

var notFlagChars = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func flagName(name string) string {
	name = strings.ReplaceAll(name, "_", "-")
	name = notFlagChars.ReplaceAllString(name, "-")
	return strings.Trim(strings.ToLower(name), "-")
}

func renderDocs(reg registry) string {
	var b strings.Builder
	b.WriteString("# CaptainBI Endpoints\n\n")
	b.WriteString("| Domain | Command | Method | Path | Content Type | Required Inputs | Risk | Pagination | Summary |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, svc := range reg.Services {
		for _, m := range svc.Methods {
			fmt.Fprintf(&b, "| %s | `cbi %s %s` | %s | `%s` | %s | %s | %s | %s | %s |\n", svc.Domain, svc.Domain, m.CommandName, m.HTTPMethod, m.FullPath, emptyDash(m.ContentType), requiredInputs(m), m.RiskLevel, paginationLabel(m.Pagination), strings.ReplaceAll(m.Summary, "|", "\\|"))
		}
	}
	return b.String()
}

func paginationLabel(value paging) string {
	if value.RangeType == "" {
		return value.Type
	}
	if value.Type == "none" {
		return value.RangeType
	}
	return value.Type + "+" + value.RangeType
}

func requiredInputs(m method) string {
	values := []string{}
	seen := map[string]bool{}
	for _, p := range m.Params {
		if p.Required && !strings.EqualFold(p.Name, "OpenChannelId") {
			value := "`--" + p.Flag + "`"
			values = append(values, value)
			seen[p.Name] = true
		}
	}
	if schema, ok := m.RequestBodySchema.(map[string]any); ok {
		for _, name := range anySlice(schema["required"]) {
			text, _ := name.(string)
			if text != "" && !seen[text] {
				values = append(values, "`data."+text+"`")
			}
		}
	}
	if m.RequiresOpenChannelID {
		values = append([]string{"`--channel`"}, values...)
	}
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ", ")
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return "`" + value + "`"
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
