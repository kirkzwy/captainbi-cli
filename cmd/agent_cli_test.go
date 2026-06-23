package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kirkzwy/captainbi-cli/internal/auth"
	"github.com/kirkzwy/captainbi-cli/internal/client"
	"github.com/kirkzwy/captainbi-cli/internal/core"
	internalerrs "github.com/kirkzwy/captainbi-cli/internal/errs"
	outfmt "github.com/kirkzwy/captainbi-cli/internal/output"
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

func TestSchemaHelpDocumentsOpenAIToolFormat(t *testing.T) {
	globals = globalOptions{}
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"schema", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "--format openai-tool") {
		t.Fatalf("schema help does not document openai-tool:\n%s", out.String())
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
	if meta["partial"] != false || meta["has_more"] != false || meta["streaming"] != false || meta["format"] != "json" {
		t.Fatalf("missing stable meta defaults: %#v", meta)
	}
}

func TestMachineNonJSONWritesPureDataAndFinalMeta(t *testing.T) {
	root := NewRootCmd()
	globals = globalOptions{machine: true, format: "csv"}
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	value := map[string]any{
		"data":          []any{map[string]any{"id": 1, "extra": "kept"}},
		"pages_fetched": 1,
		"partial":       false,
		"has_more":      true,
		"next_page":     2,
	}
	if err := writeValue(root, value, []string{"id"}, ""); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "id,extra\n1,kept\n" {
		t.Fatalf("stdout must contain pure full-field CSV: %q", stdout.String())
	}
	var status map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &status); err != nil {
		t.Fatalf("stderr must end with machine JSON: %v\n%s", err, stderr.String())
	}
	meta := status["meta"].(map[string]any)
	if status["ok"] != true || meta["format"] != "csv" || meta["has_more"] != true || meta["next_page"] != float64(2) || meta["rows"] != float64(1) {
		t.Fatalf("unexpected non-JSON status: %#v", status)
	}
}

func TestMachineControlNonJSONWritesFinalMeta(t *testing.T) {
	root := NewRootCmd()
	globals = globalOptions{machine: true, format: "table"}
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	if err := writeControlValue(root, map[string]any{"status": "ok", "count": 2}, outfmt.Options{Format: "table", Machine: true}, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "status") || !strings.Contains(stdout.String(), "ok") {
		t.Fatalf("missing table data: %q", stdout.String())
	}
	var status map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &status); err != nil || status["ok"] != true {
		t.Fatalf("missing control metadata: %#v err=%v", status, err)
	}
}

func TestOutputFileStatusKeepsPaginationMetaAndPrivateMode(t *testing.T) {
	root := NewRootCmd()
	path := filepath.Join(t.TempDir(), "goods.ndjson")
	globals = globalOptions{machine: true, format: "ndjson", outputFile: path}
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	value := map[string]any{
		"data":          []any{map[string]any{"id": 1}, map[string]any{"id": 2}},
		"pages_fetched": 1,
		"partial":       true,
		"has_more":      true,
		"next_page":     2,
	}
	if err := writeValue(root, value, []string{"id"}, ""); err != nil {
		t.Fatal(err)
	}
	// #nosec G304 -- path is created under t.TempDir for this test.
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "{\"id\":1}\n{\"id\":2}\n" {
		t.Fatalf("output file = %q", contents)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("output mode = %o, want 600", info.Mode().Perm())
		}
	}
	var status map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	data := status["data"].(map[string]any)
	meta := status["meta"].(map[string]any)
	if data["output_file"] != path || data["format"] != "ndjson" || meta["partial"] != true || meta["has_more"] != true || meta["next_page"] != float64(2) {
		t.Fatalf("output status lost metadata: %#v", status)
	}
}

func TestOutputFileRenderFailurePreservesExistingFile(t *testing.T) {
	root := NewRootCmd()
	path := filepath.Join(t.TempDir(), "existing.json")
	if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	globals = globalOptions{machine: true, format: "json", outputFile: path}
	err := writeValue(root, map[string]any{"data": []any{make(chan int)}}, nil, "")
	if err == nil {
		t.Fatal("unsupported value should fail rendering")
	}
	// #nosec G304 -- path is created under t.TempDir for this test.
	contents, readErr := os.ReadFile(path)
	if readErr != nil || string(contents) != "original\n" {
		t.Fatalf("existing file changed after failed render: %q err=%v", contents, readErr)
	}
	matches, globErr := filepath.Glob(filepath.Join(filepath.Dir(path), ".existing.json.tmp-*"))
	if globErr != nil || len(matches) != 0 {
		t.Fatalf("temporary files remain: %#v err=%v", matches, globErr)
	}
}

func TestStreamingNDJSONOutputFileAndStatus(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		data := []any{}
		if page == 1 {
			data = append(data, map[string]any{"id": 1})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "ok", "data": data})
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "stream.ndjson")
	root := NewRootCmd()
	root.SetContext(context.Background())
	globals = globalOptions{machine: true, format: "ndjson", outputFile: path}
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	c := newTestClient(server.URL)
	err := writeStreamingNDJSON(root, c, pageRowsMethod(), client.Request{Method: "GET", Path: "/items", Query: map[string]string{"rows": "1"}}, channelTarget{Alias: "main", ID: "channel"}, requestOptions{pageAll: true, pageDelay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G304 -- path is created under t.TempDir for this test.
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "{\"id\":1}\n" || stderr.Len() != 0 {
		t.Fatalf("stream output=%q stderr=%q", contents, stderr.String())
	}
	var status map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	meta := status["meta"].(map[string]any)
	if meta["streaming"] != true || meta["rows"] != float64(1) || meta["pages_fetched"] != float64(2) || meta["output_file"] != path {
		t.Fatalf("stream status = %#v", status)
	}
}

func TestStreamingNDJSONVerboseStatusIsLastStderrLine(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		data := []any{}
		if page == 1 {
			data = append(data, map[string]any{"id": 1})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "ok", "data": data})
	}))
	defer server.Close()
	root := NewRootCmd()
	root.SetContext(context.Background())
	globals = globalOptions{machine: true, format: "ndjson", verbose: true}
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	if err := writeStreamingNDJSON(root, newTestClient(server.URL), pageRowsMethod(), client.Request{Method: "GET", Path: "/items", Query: map[string]string{"rows": "1"}}, channelTarget{Alias: "main", ID: "channel"}, requestOptions{pageAll: true, pageDelay: time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	if len(lines) < 3 || !strings.Contains(lines[0], "stream page=1 rows=1") || !strings.Contains(lines[len(lines)-2], "streaming=true") {
		t.Fatalf("missing verbose diagnostic before status: %q", stderr.String())
	}
	var status map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &status); err != nil || status["ok"] != true {
		t.Fatalf("last stderr line is not success status: %q err=%v", lines[len(lines)-1], err)
	}
}

func TestNDJSONStreamingEligibilityFallsBackForTransformsAndChannels(t *testing.T) {
	globals = globalOptions{format: "ndjson"}
	method := pageRowsMethod()
	base := requestOptions{pageAll: true}
	if !shouldStreamNDJSON(method, base, []channelTarget{{Alias: "main"}}) {
		t.Fatal("eligible NDJSON page-all request should stream")
	}
	for name, configure := range map[string]func(){
		"jq":       func() { base.jq = ".data" },
		"summary":  func() { globals.summary = true },
		"limit":    func() { globals.limit = 1 },
		"channels": func() {},
	} {
		t.Run(name, func(t *testing.T) {
			globals = globalOptions{format: "ndjson"}
			base = requestOptions{pageAll: true}
			configure()
			targets := []channelTarget{{Alias: "main"}}
			if name == "channels" {
				targets = append(targets, channelTarget{Alias: "second"})
			}
			if shouldStreamNDJSON(method, base, targets) {
				t.Fatalf("%s request should use aggregate fallback", name)
			}
		})
	}
}

func TestInvalidFormatFailsBeforeHTTPRequest(t *testing.T) {
	called := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "ok", "data": []any{}})
	}))
	defer server.Close()
	t.Setenv(core.EnvConfigDir, t.TempDir())
	t.Setenv(core.EnvAccessToken, "test-token")
	t.Setenv(core.EnvBaseURL, server.URL)
	_, err := executeTestRoot([]string{"--machine", "--format", "pretty", "+sites"})
	if err == nil || errorSubtype(err) != internalerrs.ValidationBadParam {
		t.Fatalf("invalid format error = %v", err)
	}
	if called != 0 {
		t.Fatalf("invalid format sent %d HTTP requests", called)
	}
}

func TestOpenAIToolFormatIsSchemaOnly(t *testing.T) {
	_, err := executeTestRoot([]string{"--machine", "--format", "openai-tool", "+sites"})
	if err == nil || errorSubtype(err) != internalerrs.ValidationBadParam {
		t.Fatalf("openai-tool should fail outside schema: %v", err)
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
			for _, text := range []string{"Examples:", "--machine", "--summary", "--dry-run", "--range-start", "--resume-from-window"} {
				if !strings.Contains(help, text) {
					t.Fatalf("missing %s in help:\n%s", text, help)
				}
			}
		})
	}
}

func TestAdsCampaignsShortcutUsesRequiredQuery(t *testing.T) {
	seen := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, name := range []string{"start_modified_time", "end_modified_time", "type"} {
			seen[name] = r.URL.Query().Get(name)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "ok", "data": []any{}})
	}))
	defer server.Close()
	t.Setenv(core.EnvConfigDir, t.TempDir())
	t.Setenv(core.EnvAccessToken, "test-token")
	t.Setenv(core.EnvBaseURL, server.URL)
	t.Setenv(core.EnvOpenChannelID, "test-channel")
	t.Setenv(core.EnvRateLimit, "10000")

	if _, err := executeTestRoot([]string{"--machine", "+ads-campaigns", "--modified-since", "100", "--modified-until", "200", "--type", "1"}); err != nil {
		t.Fatalf("shortcut failed: %v", err)
	}
	if seen["start_modified_time"] != "100" || seen["end_modified_time"] != "200" || seen["type"] != "1" {
		t.Fatalf("shortcut query mismatch: %#v", seen)
	}
	if _, err := executeTestRoot([]string{"--machine", "+ads-campaigns", "--modified-since", "100", "--modified-until", "200"}); err == nil || errorSubtype(err) != internalerrs.ValidationRequiredFlag {
		t.Fatalf("missing type should fail locally: %v", err)
	}
	if _, err := executeTestRoot([]string{"--machine", "+ads-campaigns", "--modified-since", "100", "--modified-until", "200", "--type", "4"}); err == nil || errorSubtype(err) != internalerrs.ValidationBadParam {
		t.Fatalf("invalid type should fail locally: %v", err)
	}
}

func TestControlValueMachineEnvelopeKeepsCompatibilityFields(t *testing.T) {
	root := NewRootCmd()
	globals = globalOptions{machine: true, format: "json"}
	var out bytes.Buffer
	root.SetOut(&out)
	value := map[string]any{"status": "ok", "registry_methods": 65}
	if err := writeControlValue(root, value, outfmt.Options{Format: "json", Machine: true}, nil); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	data, ok := got["data"].(map[string]any)
	if got["ok"] != true || got["status"] != "ok" || got["registry_methods"] != float64(65) || !ok || data["status"] != "ok" {
		t.Fatalf("compatibility envelope mismatch: %#v", got)
	}
}

func TestSchemaMachineOutputHasEnvelopeAndCompatibilityFields(t *testing.T) {
	out, err := executeTestRoot([]string{"--machine", "schema", "goods.list"})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	data, ok := got["data"].(map[string]any)
	if got["domain_command"] != "goods.list" || !ok || data["domain_command"] != "goods.list" {
		t.Fatalf("schema compatibility envelope mismatch: %#v", got)
	}
}

func TestAuthStatusMachineOutputHasEnvelopeAndCompatibilityFields(t *testing.T) {
	t.Setenv(core.EnvConfigDir, t.TempDir())
	t.Setenv(core.EnvClientID, "test-client")
	t.Setenv(core.EnvAccessToken, "test-token")
	out, err := executeTestRoot([]string{"--machine", "auth", "status"})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	data, ok := got["data"].(map[string]any)
	if got["configured"] != true || got["has_cached_token"] != true || !ok || data["configured"] != true {
		t.Fatalf("auth compatibility envelope mismatch: %#v", got)
	}
}

func TestConfigAndRateControlCommandsUseCompatibleEnvelope(t *testing.T) {
	t.Setenv(core.EnvConfigDir, t.TempDir())
	t.Setenv(core.EnvAccessToken, "test-token")
	initOut, err := executeTestRoot([]string{"--machine", "config", "init", "--client-id", "test-client", "--non-interactive"})
	if err != nil {
		t.Fatal(err)
	}
	var initialized map[string]any
	if err := json.Unmarshal(initOut, &initialized); err != nil {
		t.Fatal(err)
	}
	if initialized["status"] != "saved" || initialized["data"].(map[string]any)["status"] != "saved" {
		t.Fatalf("config init compatibility envelope mismatch: %#v", initialized)
	}
	showOut, err := executeTestRoot([]string{"--machine", "config", "show"})
	if err != nil {
		t.Fatal(err)
	}
	var shown map[string]any
	if err := json.Unmarshal(showOut, &shown); err != nil {
		t.Fatal(err)
	}
	if shown["base_url"] == nil || shown["data"].(map[string]any)["base_url"] == nil {
		t.Fatalf("config show compatibility envelope mismatch: %#v", shown)
	}
	rateOut, err := executeTestRoot([]string{"--machine", "rate-limit", "status"})
	if err != nil {
		t.Fatal(err)
	}
	var rate map[string]any
	if err := json.Unmarshal(rateOut, &rate); err != nil {
		t.Fatal(err)
	}
	if rate["rate_limit_per_minute"] != float64(250) || rate["data"].(map[string]any)["rate_limit_per_minute"] != float64(250) {
		t.Fatalf("rate status compatibility envelope mismatch: %#v", rate)
	}
}

func TestChannelAllAggregatesPartialResults(t *testing.T) {
	t.Setenv(core.EnvConfigDir, t.TempDir())
	t.Setenv(core.EnvAccessToken, "test-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("OpenChannelId") == "bad-channel" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": -1, "msg": "bad channel", "data": []any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "ok", "data": []map[string]any{{"id": 1}, {"id": 2}}})
	}))
	defer server.Close()
	if err := core.SaveConfig(&core.Config{BaseURL: server.URL, RateLimit: 10000, Channels: map[string]string{"good": "good-channel", "bad": "bad-channel"}}); err != nil {
		t.Fatal(err)
	}
	out, err := executeTestRoot([]string{"--machine", "--channel", "all", "+goods", "--modified-since", "100", "--modified-until", "200"})
	if err != nil {
		t.Fatalf("partial batch should preserve successful data: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	meta := got["meta"].(map[string]any)
	data := got["data"].(map[string]any)
	channels := data["channels"].([]any)
	if got["ok"] != true || meta["partial"] != true || meta["channels_total"] != float64(2) || meta["channels_succeeded"] != float64(1) || meta["channels_failed"] != float64(1) || meta["rows"] != float64(2) || len(channels) != 2 {
		t.Fatalf("partial channel aggregate mismatch: %#v", got)
	}
	if rowCount(data) != 2 {
		t.Fatalf("channel aggregate row count=%d want 2", rowCount(data))
	}
}

func TestChannelAllFailsWhenEveryChannelFails(t *testing.T) {
	t.Setenv(core.EnvConfigDir, t.TempDir())
	t.Setenv(core.EnvAccessToken, "test-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": -1, "msg": "bad channel", "data": []any{}})
	}))
	defer server.Close()
	if err := core.SaveConfig(&core.Config{BaseURL: server.URL, RateLimit: 10000, Channels: map[string]string{"one": "bad-one", "two": "bad-two"}}); err != nil {
		t.Fatal(err)
	}
	_, err := executeTestRoot([]string{"--machine", "--channel", "all", "+goods", "--modified-since", "100", "--modified-until", "200"})
	if err == nil || exitCode(err) != 1 || errorSubtype(err) != internalerrs.ChannelBatchFailed {
		t.Fatalf("all-failed batch should return CHANNEL_BATCH_FAILED: %v", err)
	}
	globals = globalOptions{machine: true, format: "json"}
	var rendered bytes.Buffer
	writeError(&rendered, err, exitCode(err))
	var got map[string]any
	if unmarshalErr := json.Unmarshal(rendered.Bytes(), &got); unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}
	meta := got["meta"].(map[string]any)
	data := got["data"].(map[string]any)
	if got["ok"] != false || got["error_code"] != internalerrs.ChannelBatchFailed || meta["channels_failed"] != float64(2) || len(data["channels"].([]any)) != 2 {
		t.Fatalf("all-failed batch envelope mismatch: %#v", got)
	}
}

func TestReportDateRangeSatisfiesRequiredFlag(t *testing.T) {
	seen := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Query().Get("report_date"))
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "ok", "data": []map[string]any{{"date": r.URL.Query().Get("report_date")}}})
	}))
	defer server.Close()
	t.Setenv(core.EnvConfigDir, t.TempDir())
	t.Setenv(core.EnvAccessToken, "test-token")
	t.Setenv(core.EnvBaseURL, server.URL)
	t.Setenv(core.EnvOpenChannelID, "test-channel")
	t.Setenv(core.EnvRateLimit, "10000")
	globals = globalOptions{}
	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"finance", "store-daily", "--page-all", "--range-start", "20260601", "--range-end", "20260603", "--page-delay", "1", "--machine"})
	if err := root.Execute(); err != nil {
		t.Fatalf("report date range command failed: %v", err)
	}
	if len(seen) != 3 || seen[0] != "20260601" || seen[2] != "20260603" {
		t.Fatalf("unexpected report dates: %#v", seen)
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
			// #nosec G304 -- file comes from the repository-local skills/*/SKILL.md glob above.
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

func TestWriteSmokeRequiresAmazonGoodsID(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "scripts", "smoke", "write_guarded.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if !strings.Contains(text, "CAPTAINBI_WRITE_AMAZON_GOODS_ID") || strings.Contains(text, "CAPTAINBI_WRITE_GOODS_ID") {
		t.Fatal("write smoke must require the explicit amazon_goods_id fixture variable")
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
		{"CaptainBI rate limit code", &client.StatusError{StatusCode: http.StatusUnauthorized, Body: map[string]any{"code": 100910, "msg": "too frequent"}}, "RATE_LIMIT_EXCEEDED"},
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

func TestCaptainBIRateLimitCodeUsesRateLimitEnvelope(t *testing.T) {
	err := &client.StatusError{StatusCode: http.StatusUnauthorized, Body: map[string]any{"code": 100910, "msg": "too frequent"}, Retryable: true}
	if got := exitCode(err); got != 4 {
		t.Fatalf("exit code=%d want 4", got)
	}
	globals = globalOptions{machine: true}
	var out bytes.Buffer
	writeError(&out, err, exitCode(err))
	var got map[string]any
	if unmarshalErr := json.Unmarshal(out.Bytes(), &got); unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}
	if got["kind"] != "rate_limit" || got["error_code"] != internalerrs.RateLimitExceeded || got["retryable"] != true {
		t.Fatalf("unexpected rate limit envelope: %#v", got)
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
	t.Setenv(core.EnvWriteAllowlist, "fba.sync-shipment")
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

func TestAgentDangerousWriteRequiresAllowlistBeforeConsumingApproval(t *testing.T) {
	t.Setenv(core.EnvConfigDir, t.TempDir())
	t.Setenv(core.EnvAccessToken, "test-token")
	t.Setenv(core.EnvRateLimit, "6000")
	t.Setenv("CBI_AGENT", "1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"code":200,"msg":"ok","data":[]}`)
	}))
	defer server.Close()
	t.Setenv(core.EnvBaseURL, server.URL)
	args := []string{"--machine", "goods", "set-group", "--group-id", "2", "--goods-id", "1"}
	dryOut, err := executeTestRoot(append(append([]string{}, args...), "--dry-run"))
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(dryOut, &envelope); err != nil {
		t.Fatal(err)
	}
	preview := envelope["data"].(map[string]any)
	policy := preview["policy"].(map[string]any)
	if policy["allowlist_required"] != true || policy["allowlisted"] != false {
		t.Fatalf("unexpected dry-run policy: %#v", policy)
	}
	hash := preview["approval"].(map[string]any)["request_hash"].(string)
	actual := append(append([]string{}, args...), "--confirm-request", hash)
	if _, err := executeTestRoot(actual); err == nil || errorSubtype(err) != internalerrs.WriteNotAllowlisted {
		t.Fatalf("expected write allowlist rejection, got %v", err)
	}
	t.Setenv(core.EnvWriteAllowlist, "goods.set-group")
	if _, err := executeTestRoot(actual); err != nil {
		t.Fatalf("approval should remain usable after allowlist correction: %v", err)
	}
}

func TestWriteAllowlistConfigCommands(t *testing.T) {
	t.Setenv(core.EnvConfigDir, t.TempDir())
	if _, err := executeTestRoot([]string{"--machine", "config", "write-allowlist", "add", "goods.set-group"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.WriteAllowlist) != 1 || cfg.WriteAllowlist[0] != "goods.set-group" {
		t.Fatalf("unexpected allowlist: %#v", cfg.WriteAllowlist)
	}
	if _, err := executeTestRoot([]string{"--machine", "config", "write-allowlist", "add", "goods.list"}); err == nil {
		t.Fatal("read command must not be accepted by write allowlist")
	}
	if _, err := executeTestRoot([]string{"--machine", "config", "write-allowlist", "remove", "goods.set-group"}); err != nil {
		t.Fatal(err)
	}
	cfg, err = core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.WriteAllowlist) != 0 {
		t.Fatalf("allowlist was not removed: %#v", cfg.WriteAllowlist)
	}
}

func TestConfigRateLimitCommand(t *testing.T) {
	t.Setenv(core.EnvConfigDir, t.TempDir())
	t.Setenv(core.EnvRateLimit, "")
	if _, err := executeTestRoot([]string{"--machine", "config", "rate-limit", "250"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RateLimit != 250 {
		t.Fatalf("rate limit=%d want 250", cfg.RateLimit)
	}
	if _, err := executeTestRoot([]string{"--machine", "config", "rate-limit", "invalid"}); err == nil || errorSubtype(err) != internalerrs.ValidationBadParam {
		t.Fatalf("expected validation error, got %v", err)
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
	t.Setenv(core.EnvWriteAllowlist, strings.Join([]string{
		"goods.edit-group", "goods.set-group", "sales.upload-fbm-shipping",
		"finance.set-cost", "finance.set-rule", "fba.sync-shipment",
	}, ","))
	var mu sync.Mutex
	called := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("content-type"), "multipart/form-data;") {
			t.Errorf("%s content-type = %q", r.URL.Path, r.Header.Get("content-type"))
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		// #nosec G120 -- MaxBytesReader above bounds the entire multipart body in this test server.
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
