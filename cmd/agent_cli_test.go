package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

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
}

func TestExitCodeStatus429(t *testing.T) {
	if got := exitCode(&client.StatusError{StatusCode: 429}); got != 4 {
		t.Fatalf("429 exit code = %d", got)
	}
}
