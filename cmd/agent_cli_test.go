package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kirkzwy/captainbi-cli/internal/auth"
	"github.com/kirkzwy/captainbi-cli/internal/client"
	"github.com/kirkzwy/captainbi-cli/internal/core"
	internalerrs "github.com/kirkzwy/captainbi-cli/internal/errs"
)

func TestSchemaOpenAIToolFormat(t *testing.T) {
	globals = globalOptions{}
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--format", "openai-tool", "schema", "finance.store-daily"})
	if err := root.Execute(); err != nil {
		t.Fatalf("schema command failed: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out.String())
	}
	if got["type"] != "function" {
		t.Fatalf("expected openai function schema, got %#v", got["type"])
	}
}

func TestToolsExportOpenAI(t *testing.T) {
	globals = globalOptions{}
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"tools", "export", "--format", "openai"})
	if err := root.Execute(); err != nil {
		t.Fatalf("tools export failed: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out.String())
	}
	if len(got) != 65 {
		t.Fatalf("expected 65 tools, got %d", len(got))
	}
}

func TestMachineErrorEnvelope(t *testing.T) {
	globals = globalOptions{machine: true}
	var out bytes.Buffer
	writeError(&out, typed("auth", "authentication failed"), 2)
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out.String())
	}
	if got["ok"] != false || got["error_code"] != "AUTH_TOKEN_REFRESH_FAILED" || got["kind"] != "auth" {
		t.Fatalf("unexpected envelope: %#v", got)
	}
	if _, ok := got["retryable"]; !ok {
		t.Fatalf("missing retryable field: %#v", got)
	}
	if got["hint"] == "" {
		t.Fatalf("missing hint field: %#v", got)
	}
	errorObject, ok := got["error"].(map[string]any)
	if !ok || errorObject["kind"] != "auth" || errorObject["subtype"] != "AUTH_TOKEN_REFRESH_FAILED" {
		t.Fatalf("missing nested error contract: %#v", got)
	}
	meta, ok := got["meta"].(map[string]any)
	if !ok || meta["exit_code"] != float64(2) {
		t.Fatalf("missing nested meta contract: %#v", got)
	}
}

func TestExitCodeStatus429(t *testing.T) {
	if got := exitCode(&client.StatusError{StatusCode: 429}); got != 4 {
		t.Fatalf("429 exit code = %d", got)
	}
}

func TestCBIAgentEnablesMachineErrorEnvelope(t *testing.T) {
	t.Setenv("CBI_AGENT", "1")
	globals = globalOptions{}
	var out bytes.Buffer
	writeError(&out, typed("auth", "authentication failed"), 2)
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out.String())
	}
	if got["ok"] != false {
		t.Fatalf("expected machine error envelope, got %#v", got)
	}
}

func TestMachineSuccessEnvelope(t *testing.T) {
	root := NewRootCmd()
	globals = globalOptions{machine: true, format: "json"}
	var out bytes.Buffer
	root.SetOut(&out)
	err := writeValue(root, map[string]any{
		"data":          []any{map[string]any{"sku": "A"}},
		"pages_fetched": float64(1),
	}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out.String())
	}
	if got["ok"] != true || got["data"] == nil || got["meta"] == nil {
		t.Fatalf("missing success envelope: %#v", got)
	}
	meta := got["meta"].(map[string]any)
	if meta["rows"] != float64(1) || meta["pages_fetched"] != float64(1) {
		t.Fatalf("unexpected success meta: %#v", meta)
	}
}

func TestShortcutHelpIncludesAgentFlagsAndExamples(t *testing.T) {
	for _, name := range []string{"+shops", "+sites", "+orders", "+goods", "+finance-daily", "+inventory", "+ads-campaigns", "+ads-campaign-report", "+reviews", "+store-transactions"} {
		t.Run(name, func(t *testing.T) {
			globals = globalOptions{}
			root := NewRootCmd()
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&bytes.Buffer{})
			root.SetArgs([]string{name, "--help"})
			if err := root.Execute(); err != nil {
				t.Fatalf("shortcut help failed: %v", err)
			}
			help := out.String()
			for _, text := range []string{"Examples:", "--machine", "--summary", "--dry-run"} {
				if !strings.Contains(help, text) {
					t.Fatalf("missing %s in help:\n%s", text, help)
				}
			}
		})
	}
}

func TestSkillDescriptionsUseRoutingContract(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "skills", "*", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no skill files found")
	}
	for _, file := range files {
		t.Run(filepath.Base(filepath.Dir(file)), func(t *testing.T) {
			b, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			text := string(b)
			start := strings.Index(text, "description:")
			if start < 0 {
				t.Fatalf("missing description in %s", file)
			}
			line := strings.Split(text[start:], "\n")[0]
			for _, want := range []string{"WHEN use for", "WHEN NOT"} {
				if !strings.Contains(line, want) {
					t.Fatalf("description must contain %q: %s", want, line)
				}
			}
		})
	}
}

func TestUnsafeInputPathHasStableSubtype(t *testing.T) {
	_, err := readSafeInputFile(filepath.Join(t.TempDir(), "params.json"))
	if err == nil {
		t.Fatal("expected absolute input path to be rejected")
	}
	if got := errorSubtype(err); got != internalerrs.InputPathUnsafe {
		t.Fatalf("subtype=%q want %q", got, internalerrs.InputPathUnsafe)
	}
	if hint := hintForError(err); !strings.Contains(hint, "stdin") {
		t.Fatalf("expected actionable stdin hint, got %q", hint)
	}
}

func TestStableErrorSubtypes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"missing channel", typedH("business", "OpenChannelId is required; pass --open-channel-id", ""), "CHANNEL_MISSING"},
		{"required flag", typedH("business", "required flag --report-date is missing", ""), "VALIDATION_REQUIRED_FLAG"},
		{"bad param", typedH("business", "flag --rows must be <= 100", ""), "VALIDATION_BAD_PARAM"},
		{"invalid client", &auth.TokenError{StatusCode: 400, ErrorCode: "invalid_client", ErrorDescription: "Invalid client authentication"}, "AUTH_INVALID_CLIENT"},
		{"rate limit", &client.StatusError{StatusCode: http.StatusTooManyRequests}, "RATE_LIMIT_EXCEEDED"},
		{"server", &client.StatusError{StatusCode: http.StatusBadGateway}, "HTTP_5XX"},
		{"invalid channel", &client.StatusError{StatusCode: http.StatusUnauthorized, Body: map[string]any{"code": 100903, "msg": "open_channel_id 未找到"}}, "CHANNEL_INVALID"},
		{"api business", &client.StatusError{StatusCode: http.StatusBadRequest, Body: map[string]any{"code": 4001, "msg": "bad request"}}, "API_BUSINESS_ERROR"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			globals = globalOptions{machine: true}
			var out bytes.Buffer
			writeError(&out, tc.err, exitCode(tc.err))
			var got map[string]any
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("invalid json: %v\n%s", err, out.String())
			}
			errorObject := got["error"].(map[string]any)
			if errorObject["subtype"] != tc.want || got["error_code"] != tc.want {
				t.Fatalf("subtype = %#v top = %#v, want %s; envelope %#v", errorObject["subtype"], got["error_code"], tc.want, got)
			}
			if errorObject["hint"] == "" {
				t.Fatalf("missing hint for %s: %#v", tc.want, got)
			}
		})
	}
}

func TestMachineErrorEnvelopeOAuthFields(t *testing.T) {
	globals = globalOptions{machine: true}
	var out bytes.Buffer
	writeError(&out, &auth.TokenError{StatusCode: 400, ErrorCode: "invalid_client", ErrorDescription: "Invalid client authentication"}, 2)
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out.String())
	}
	if got["api_code"] != "invalid_client" || got["api_msg"] != "Invalid client authentication" || got["hint"] == "" {
		t.Fatalf("unexpected envelope: %#v", got)
	}
}

func TestRowsMaxValidation(t *testing.T) {
	globals = globalOptions{}
	root := NewRootCmd()
	var stderr bytes.Buffer
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&stderr)
	root.SetArgs([]string{"--machine", "goods", "list", "--rows", "999", "--page", "1", "--start-modified-time", "1", "--end-modified-time", "2"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected rows max validation error")
	}
	var out bytes.Buffer
	writeError(&out, err, exitCode(err))
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out.String())
	}
	if got["message"] != "flag --rows must be <= 100" || got["hint"] == "" {
		t.Fatalf("unexpected validation envelope: %#v", got)
	}
}

func TestBadTimestampRejectedBeforeAPI(t *testing.T) {
	globals = globalOptions{}
	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--machine", "goods", "list", "--rows", "100", "--page", "1", "--start-modified-time", "not-a-number", "--end-modified-time", "2"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "must be an integer") {
		t.Fatalf("expected local timestamp validation, got %v", err)
	}
}

func TestInvalidChannelUsesChannelHintAndBusinessExit(t *testing.T) {
	err := &client.StatusError{StatusCode: http.StatusUnauthorized, Body: map[string]any{"code": 100903, "msg": "open_channel_id 未找到"}}
	if got := exitCode(err); got != 1 {
		t.Fatalf("channel error exit code = %d", got)
	}
	globals = globalOptions{machine: true}
	var out bytes.Buffer
	writeError(&out, err, exitCode(err))
	var got map[string]any
	if unmarshalErr := json.Unmarshal(out.Bytes(), &got); unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}
	if got["kind"] != "business" || !strings.Contains(got["hint"].(string), "channel alias") {
		t.Fatalf("unexpected channel envelope: %#v", got)
	}
}

func TestWriteDryRunIssuesBoundApproval(t *testing.T) {
	t.Setenv("CAPTAINBI_CONFIG_DIR", t.TempDir())
	t.Setenv("CBI_AGENT", "1")
	globals = globalOptions{}
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"--machine", "--format", "json", "--open-channel-id", "test-channel",
		"finance", "set-cost", "--data", `{"data":[{"sku":"TEST","purchasing_cost":"1.00","purchasing_cost_currency_code":1,"fba_cost":1,"fba_cost_currency_code":1,"fbm_cost":1,"fbm_cost_currency_code":1}]}`, "--dry-run",
	})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	data := envelope["data"].(map[string]any)
	approval := data["approval"].(map[string]any)
	if approval["request_hash"] == "" || data["content_type"] != "multipart/form-data" {
		t.Fatalf("unexpected dry-run: %#v", envelope)
	}
}

func TestAgentWriteRequiresConfirmRequest(t *testing.T) {
	t.Setenv("CAPTAINBI_CONFIG_DIR", t.TempDir())
	t.Setenv("CBI_AGENT", "1")
	globals = globalOptions{}
	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"--machine", "--open-channel-id", "test-channel",
		"fba", "sync-shipment", "--shipment-ids", "FBA1",
	})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "confirm-request") {
		t.Fatalf("expected Agent confirmation error, got %v", err)
	}
	if errorSubtype(err) != "CONFIRMATION_REQUIRED" {
		t.Fatalf("unexpected subtype %s", errorSubtype(err))
	}
}

func TestAgentWriteSafeDoesNotAutoApprove(t *testing.T) {
	t.Setenv("CAPTAINBI_CONFIG_DIR", t.TempDir())
	t.Setenv("CBI_AGENT", "1")
	globals = globalOptions{}
	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{
		"--machine", "--open-channel-id", "test-channel",
		"goods", "set-operate-user", "--goods-id", "1", "--operation-user-admin-id", "2",
	})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "confirm-request") {
		t.Fatalf("expected Agent write_safe confirmation error, got %v", err)
	}
}

func TestWritesRejectChannelAll(t *testing.T) {
	t.Setenv(core.EnvConfigDir, t.TempDir())
	t.Setenv("CBI_AGENT", "1")
	if err := core.SaveConfig(&core.Config{
		BaseURL:   "https://example.invalid",
		RateLimit: 20,
		Channels:  map[string]string{"one": "channel-1", "two": "channel-2"},
	}); err != nil {
		t.Fatal(err)
	}
	globals = globalOptions{}
	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--machine", "--channel", "all", "fba", "sync-shipment", "--shipment-ids", "FBA1", "--dry-run"})
	err := root.Execute()
	if err == nil || errorSubtype(err) != "WRITE_MULTI_CHANNEL_FORBIDDEN" {
		t.Fatalf("expected multi-channel write rejection, got subtype=%s err=%v", errorSubtype(err), err)
	}
}

func TestUnknownRawWriteCannotBypassRisk(t *testing.T) {
	globals = globalOptions{}
	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"api", "POST", "/v1/unknown", "--data", `{}`})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unsafe-raw-write") {
		t.Fatalf("expected raw write protection, got %v", err)
	}
}

func TestRawParamsPreserveLargeIntegers(t *testing.T) {
	query, _, err := parseMaps(inputOptions{params: `{"report_date":20260621,"page":1}`}, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	if query["report_date"] != "20260621" || query["page"] != "1" {
		t.Fatalf("raw params were reformatted: %#v", query)
	}
}

func TestRequestBodyRejectsFractionalInteger(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"quantity": map[string]any{"type": "integer"},
		},
	}
	err := validateRequestBody(schema, map[string]any{"quantity": 1.5})
	if err == nil || !strings.Contains(err.Error(), "must be an integer") {
		t.Fatalf("expected integer validation error, got %v", err)
	}
}

func TestDocumentedGETBodyFieldsUseQueryTransport(t *testing.T) {
	t.Setenv("CAPTAINBI_CONFIG_DIR", t.TempDir())
	t.Setenv("CAPTAINBI_ACCESS_TOKEN", "test-token")
	t.Setenv("CAPTAINBI_RATE_LIMIT", "6000")
	t.Setenv("CBI_AGENT", "1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("report_date"); got != "20260621" {
			t.Errorf("report_date query = %q", got)
		}
		if r.Header.Get("content-type") != "" {
			t.Errorf("GET content-type = %q", r.Header.Get("content-type"))
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"msg":"ok","data":[]}`))
	}))
	defer server.Close()
	t.Setenv("CAPTAINBI_BASE_URL", server.URL)
	_, err := executeTestRoot([]string{
		"--machine", "--open-channel-id", "test-channel",
		"ads", "advertise-campaign-report", "--report-date", "20260621", "--page", "1", "--rows", "100",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAllWriteEndpointsDryRunAndMockExecution(t *testing.T) {
	t.Setenv("CAPTAINBI_CONFIG_DIR", t.TempDir())
	t.Setenv("CAPTAINBI_ACCESS_TOKEN", "test-token")
	t.Setenv("CAPTAINBI_RATE_LIMIT", "6000")
	t.Setenv("CBI_AGENT", "1")
	var mu sync.Mutex
	called := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("content-type"), "multipart/form-data;") {
			t.Errorf("%s content-type = %q", r.URL.Path, r.Header.Get("content-type"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("%s multipart parse: %v", r.URL.Path, err)
		}
		mu.Lock()
		called[r.URL.Path]++
		mu.Unlock()
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"msg":"ok","data":[]}`))
	}))
	defer server.Close()
	t.Setenv("CAPTAINBI_BASE_URL", server.URL)

	cases := []struct {
		name            string
		path            string
		requiresChannel bool
		args            []string
	}{
		{"edit group", "/v1/open_goods/edit_goods_group", false, []string{"goods", "edit-group", "--group-name", "Agent Test"}},
		{"set group", "/v1/open_goods/set_goods_group", false, []string{"goods", "set-group", "--group-id", "2", "--goods-id", "1"}},
		{"set operator", "/v1/open_goods/set_goods_operate_user", true, []string{"goods", "set-operate-user", "--goods-id", "1", "--operation-user-admin-id", "2"}},
		{"set operation mode", "/v1/open_user/set_channel_operation_mode", true, []string{"goods", "set-shop-operation-mode", "--operation-user-admin-id", "2"}},
		{"upload FBM", "/v1/open_order/upload_fbm_order_ship_info", true, []string{"sales", "upload-fbm-shipping", "--data", `{"data":[{"amazon_order_id":"ORDER","carrier_code":"UPS","shipping_method":"Ground","shipper_tracking_number":"TRACK","amazon_order_item_code":"ITEM","quantity":1}]}`}},
		{"set cost", "/v1/open_finance/set_goods_cost", true, []string{"finance", "set-cost", "--data", `{"data":[{"sku":"TEST","purchasing_cost":"1.00","purchasing_cost_currency_code":1,"fba_cost":1,"fba_cost_currency_code":1,"fbm_cost":1,"fbm_cost_currency_code":1}]}`}},
		{"set rule", "/v1/open_finance/set_rule", true, []string{"finance", "set-rule", "--data", `{"miaoshu":"Agent test","jine":1,"kaishi":"2026-06-21"}`}},
		{"sync shipment", "/v1/open_fba/sync_shipment", true, []string{"fba", "sync-shipment", "--shipment-ids", "FBA1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prefix := []string{"--machine", "--format", "json"}
			if tc.requiresChannel {
				prefix = append(prefix, "--open-channel-id", "test-channel")
			}
			dryArgs := append(append(append([]string{}, prefix...), tc.args...), "--dry-run")
			dryOut, err := executeTestRoot(dryArgs)
			if err != nil {
				t.Fatalf("dry-run: %v", err)
			}
			var envelope map[string]any
			if err := json.Unmarshal(dryOut, &envelope); err != nil {
				t.Fatal(err)
			}
			preview := envelope["data"].(map[string]any)
			approvalObject := preview["approval"].(map[string]any)
			hash := approvalObject["request_hash"].(string)
			actualArgs := append(append(append([]string{}, prefix...), tc.args...), "--confirm-request", hash)
			if _, err := executeTestRoot(actualArgs); err != nil {
				t.Fatalf("mock write: %v", err)
			}
		})
	}
	mu.Lock()
	defer mu.Unlock()
	for _, tc := range cases {
		if called[tc.path] != 1 {
			t.Errorf("%s calls = %d", tc.path, called[tc.path])
		}
	}
}

func executeTestRoot(args []string) ([]byte, error) {
	globals = globalOptions{}
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(args)
	err := root.Execute()
	return out.Bytes(), err
}
