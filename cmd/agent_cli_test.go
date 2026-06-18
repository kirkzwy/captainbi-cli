package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirkzwy/captainbi-cli/internal/auth"
	"github.com/kirkzwy/captainbi-cli/internal/client"
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
