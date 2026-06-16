package cmd

import (
	"bytes"
	"encoding/json"
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
	if got["ok"] != false || got["error_code"] != "AUTH" || got["kind"] != "auth" {
		t.Fatalf("unexpected envelope: %#v", got)
	}
	if _, ok := got["retryable"]; !ok {
		t.Fatalf("missing retryable field: %#v", got)
	}
	if got["hint"] == "" {
		t.Fatalf("missing hint field: %#v", got)
	}
	errorObject, ok := got["error"].(map[string]any)
	if !ok || errorObject["kind"] != "auth" || errorObject["subtype"] != "AUTH" {
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

func TestShortcutHelpIncludesAgentFlags(t *testing.T) {
	globals = globalOptions{}
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"+goods", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("shortcut help failed: %v", err)
	}
	help := out.String()
	for _, flag := range []string{"--page-all", "--max-records", "--resume-from-page", "--dry-run"} {
		if !strings.Contains(help, flag) {
			t.Fatalf("missing %s in help:\n%s", flag, help)
		}
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
