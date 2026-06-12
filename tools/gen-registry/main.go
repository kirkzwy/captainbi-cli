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
)

type spec struct {
	Paths map[string]map[string]operation `json:"paths"`
}

type operation struct {
	Tags       []string    `json:"tags"`
	Summary    string      `json:"summary"`
	Parameters []parameter `json:"parameters"`
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
	TableColumns          []string `json:"tableColumns,omitempty"`
}

type param struct {
	Name        string `json:"name"`
	Flag        string `json:"flag"`
	Location    string `json:"location"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
	Default     any    `json:"default,omitempty"`
	Max         int    `json:"max,omitempty"`
}

type paging struct {
	Type       string `json:"type"`
	MaxRows    int    `json:"maxRows,omitempty"`
	TotalField string `json:"totalField,omitempty"`
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
	must(os.WriteFile(*out, append(b, '\n'), 0o644))
	must(os.WriteFile(*docs, []byte(renderDocs(reg)), 0o644))
}

func readSource(source string) ([]byte, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		resp, err := http.Get(source)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		return io.ReadAll(resp.Body)
	}
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
				Pagination:            paginationFor(op.Parameters),
				TableColumns:          tableColumnsFor(domain, p),
			}
			for _, raw := range op.Parameters {
				if strings.EqualFold(raw.Name, "authorization") {
					continue
				}
				p := param{
					Name:        raw.Name,
					Flag:        flagName(raw.Name),
					Location:    raw.In,
					Type:        fmt.Sprint(raw.Schema["type"]),
					Required:    raw.Required,
					Description: raw.Description,
				}
				if p.Name == "page" {
					p.Default = 1
				}
				if p.Name == "rows" {
					p.Default = 100
					p.Max = 100
				}
				m.Params = append(m.Params, p)
			}
			services[domain].Methods = append(services[domain].Methods, m)
		}
	}
	order := []string{"goods", "sales", "finance", "fba", "ads", "monitor"}
	reg := registry{Version: "0.1.0", Source: source}
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

func paginationFor(params []parameter) paging {
	names := map[string]bool{}
	for _, p := range params {
		names[p.Name] = true
	}
	if names["page"] && names["rows"] {
		return paging{Type: "page_rows", MaxRows: 100, TotalField: "max_result"}
	}
	if names["start_modified_time"] || names["end_modified_time"] || names["start_report_time"] || names["end_report_time"] {
		return paging{Type: "modified_time_window"}
	}
	if names["report_date"] {
		return paging{Type: "report_date"}
	}
	return paging{Type: "none"}
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

var notFlagChars = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func flagName(name string) string {
	name = strings.ReplaceAll(name, "_", "-")
	name = notFlagChars.ReplaceAllString(name, "-")
	return strings.Trim(strings.ToLower(name), "-")
}

func renderDocs(reg registry) string {
	var b strings.Builder
	b.WriteString("# CaptainBI Endpoints\n\n")
	b.WriteString("| Domain | Command | Method | Path | Risk | Pagination | Summary |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, svc := range reg.Services {
		for _, m := range svc.Methods {
			fmt.Fprintf(&b, "| %s | `cbi %s %s` | %s | `%s` | %s | %s | %s |\n", svc.Domain, svc.Domain, m.CommandName, m.HTTPMethod, m.FullPath, m.RiskLevel, m.Pagination.Type, strings.ReplaceAll(m.Summary, "|", "\\|"))
		}
	}
	return b.String()
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
