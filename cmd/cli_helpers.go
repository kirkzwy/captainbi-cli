package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kirkzwy/captainbi-cli/internal/auth"
	"github.com/kirkzwy/captainbi-cli/internal/client"
	"github.com/kirkzwy/captainbi-cli/internal/core"
	outfmt "github.com/kirkzwy/captainbi-cli/internal/output"
	"github.com/kirkzwy/captainbi-cli/internal/registry"
	"github.com/kirkzwy/captainbi-cli/internal/security"
)

type channelTarget struct {
	Alias string
	ID    string
}

func resolveChannels(cfg *core.Config, direct string, required bool) ([]channelTarget, error) {
	if direct != "" {
		return []channelTarget{{Alias: "direct", ID: direct}}, nil
	}
	if globals.channelFile != "" {
		targets, err := loadChannelFile(globals.channelFile)
		if err != nil {
			return nil, err
		}
		if len(targets) > 0 {
			return targets, nil
		}
	}
	if globals.channel != "" {
		if globals.channel == "all" {
			if len(cfg.Channels) == 0 {
				return nil, typedH("business", "no configured channel aliases; run `cbi config channels add <alias> <open-channel-id>`", "run cbi +shops to discover OpenChannelId, then cbi config channels add <alias> <open-channel-id>")
			}
			aliases := make([]string, 0, len(cfg.Channels))
			for alias := range cfg.Channels {
				aliases = append(aliases, alias)
			}
			sort.Strings(aliases)
			targets := make([]channelTarget, 0, len(aliases))
			for _, alias := range aliases {
				targets = append(targets, channelTarget{Alias: alias, ID: cfg.Channels[alias]})
			}
			return targets, nil
		}
		if id, ok := cfg.Channels[globals.channel]; ok {
			return []channelTarget{{Alias: globals.channel, ID: id}}, nil
		}
		return []channelTarget{{Alias: "direct", ID: globals.channel}}, nil
	}
	if cfg.OpenChannelID != "" {
		return []channelTarget{{Alias: "default", ID: cfg.OpenChannelID}}, nil
	}
	if required {
		return nil, typedH("business", "OpenChannelId is required; pass --open-channel-id, --channel, --channel-file, or configure CAPTAINBI_OPEN_CHANNEL_ID", "run cbi +shops, then pass --channel <alias> or --open-channel-id <id>")
	}
	return []channelTarget{{}}, nil
}

func loadChannelFile(path string) ([]channelTarget, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err == nil && len(m) > 0 {
		aliases := make([]string, 0, len(m))
		for alias := range m {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		targets := make([]channelTarget, 0, len(aliases))
		for _, alias := range aliases {
			targets = append(targets, channelTarget{Alias: alias, ID: m[alias]})
		}
		return targets, nil
	}
	var rows []struct {
		Alias         string `json:"alias"`
		OpenChannelID string `json:"open_channel_id"`
	}
	if err := json.Unmarshal(b, &rows); err == nil && len(rows) > 0 {
		targets := make([]channelTarget, 0, len(rows))
		for _, row := range rows {
			if row.OpenChannelID != "" {
				targets = append(targets, channelTarget{Alias: row.Alias, ID: row.OpenChannelID})
			}
		}
		return targets, nil
	}
	var ids []string
	if err := json.Unmarshal(b, &ids); err == nil && len(ids) > 0 {
		targets := make([]channelTarget, 0, len(ids))
		for i, id := range ids {
			targets = append(targets, channelTarget{Alias: fmt.Sprintf("channel_%d", i+1), ID: id})
		}
		return targets, nil
	}
	return nil, typedH("business", "channel file must be JSON object, array of strings, or array of {alias,open_channel_id}", "use {\"alias\":\"open_channel_id\"} or [{\"alias\":\"main\",\"open_channel_id\":\"...\"}]")
}

func channelResult(target channelTarget, resp map[string]any, err error) map[string]any {
	out := map[string]any{
		"channel":         target.Alias,
		"open_channel_id": security.RedactValue(target.ID),
		"ok":              err == nil,
	}
	if resp != nil {
		out["rows"] = rowCount(resp)
		if v, ok := resp["pages_fetched"]; ok {
			out["pages_fetched"] = v
		}
		if v, ok := resp["pages_failed"]; ok {
			out["pages_failed"] = v
		}
		if v, ok := resp["partial"]; ok {
			out["partial"] = v
		}
		if v, ok := resp["has_more"]; ok {
			out["has_more"] = v
		}
		if v, ok := resp["next_page"]; ok {
			out["next_page"] = v
		}
		if v, ok := resp["rate_limit_wait_ms"]; ok {
			out["rate_limit_wait_ms"] = v
		}
	}
	if err != nil {
		out["error_code"] = errorCode(err)
		out["message"] = security.RedactString(err.Error())
		if apiCode, apiMsg := apiErrorFields(err); apiCode != nil || apiMsg != "" {
			out["api_code"] = apiCode
			out["api_msg"] = security.RedactString(apiMsg)
		}
	}
	return out
}

func writeAudit(m registry.Method, target channelTarget, callErr error) {
	if globals.auditLog == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(globals.auditLog), 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(globals.auditLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	row := map[string]any{
		"time":            time.Now().Format(time.RFC3339),
		"command":         m.CommandName,
		"path":            m.FullPath,
		"method":          m.HTTPMethod,
		"channel":         target.Alias,
		"open_channel_id": security.RedactValue(target.ID),
		"exit_code":       0,
	}
	if callErr != nil {
		row["exit_code"] = exitCode(callErr)
		row["error_code"] = errorCode(callErr)
		if apiCode, _ := apiErrorFields(callErr); apiCode != nil {
			row["api_code"] = apiCode
		}
	}
	_ = json.NewEncoder(f).Encode(row)
}

func writeValue(cmd *cobra.Command, value any, columns []string, jq string) error {
	value = prepareValue(value)
	if globals.summary {
		value = summarizeValue(value)
	}
	if globals.outputFile != "" {
		f, err := os.Create(globals.outputFile)
		if err != nil {
			return err
		}
		if err := outfmt.Write(f, value, outfmt.Options{Format: globals.format, Machine: globals.machine, JQ: jq}, columns); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		view := map[string]any{
			"ok":          true,
			"output_file": globals.outputFile,
			"rows":        rowCount(value),
		}
		if agentMode() {
			return outfmt.Write(cmd.OutOrStdout(), successEnvelope(view), outfmt.Options{Format: "json", Machine: true}, nil)
		}
		return outfmt.Write(cmd.OutOrStdout(), view, outfmt.Options{Format: "json", Machine: globals.machine}, nil)
	}
	if agentMode() && strings.EqualFold(globals.format, "json") {
		value = successEnvelope(value)
	}
	return outfmt.Write(cmd.OutOrStdout(), value, outfmt.Options{Format: globals.format, Machine: globals.machine, JQ: jq}, columns)
}

func agentMode() bool {
	return globals.machine || os.Getenv("CBI_AGENT") == "1"
}

func successEnvelope(value any) map[string]any {
	meta := metaForValue(value)
	if _, ok := meta["hints"]; !ok {
		meta["hints"] = []string{}
	}
	if _, ok := meta["alerts"]; !ok {
		meta["alerts"] = []string{}
	}
	return map[string]any{
		"ok":   true,
		"data": value,
		"meta": meta,
	}
}

func metaForValue(value any) map[string]any {
	meta := map[string]any{
		"count":  rowCount(value),
		"rows":   rowCount(value),
		"hints":  hintsForValue(value),
		"alerts": []string{},
	}
	if globals.outputFile != "" {
		meta["output_file"] = globals.outputFile
	}
	if m, ok := value.(map[string]any); ok {
		copyMetaKey(meta, m, "pages_fetched")
		copyMetaKey(meta, m, "pages_failed")
		copyMetaKey(meta, m, "partial")
		copyMetaKey(meta, m, "has_more")
		copyMetaKey(meta, m, "next_page")
		copyMetaKey(meta, m, "rate_limit_wait_ms")
		copyMetaKey(meta, m, "channel")
		if channels, ok := m["channels"].([]map[string]any); ok {
			meta["channels"] = len(channels)
		} else if channels, ok := m["channels"].([]any); ok {
			meta["channels"] = len(channels)
		}
	}
	return meta
}

func copyMetaKey(meta, src map[string]any, key string) {
	if v, ok := src[key]; ok {
		meta[key] = v
	}
}

func hintsForValue(value any) []string {
	hints := []string{}
	if m, ok := value.(map[string]any); ok {
		if partial, _ := m["partial"].(bool); partial {
			hints = append(hints, "result is partial; use failed_at_page/resume_from_page with --resume-from-page to continue")
		}
		if outputFile := fmt.Sprint(m["output_file"]); outputFile != "" && outputFile != "<nil>" {
			hints = append(hints, "data was written to output_file; read that file for full rows")
		}
	}
	if globals.outputFile == "" && rowCount(value) > 1000 {
		hints = append(hints, "large output detected; prefer --format ndjson --output-file <file>")
	}
	return hints
}

func prepareValue(value any) any {
	if globals.limit <= 0 {
		return value
	}
	switch v := value.(type) {
	case []any:
		if len(v) > globals.limit {
			return v[:globals.limit]
		}
	case []map[string]any:
		if len(v) > globals.limit {
			return v[:globals.limit]
		}
	case map[string]any:
		if data, ok := v["data"].([]any); ok && len(data) > globals.limit {
			cp := copyMap(v)
			cp["data"] = data[:globals.limit]
			cp["limited"] = true
			cp["limit"] = globals.limit
			return cp
		}
	}
	return value
}

func summarizeValue(value any) map[string]any {
	rows := rowsFromAny(value)
	fields := map[string]int{}
	channels := map[string]int{}
	for _, row := range rows {
		for key := range row {
			fields[key]++
		}
		if channel, ok := row["channel"].(string); ok && channel != "" {
			channels[channel]++
		}
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := map[string]any{
		"ok":     true,
		"rows":   len(rows),
		"fields": keys,
	}
	if len(channels) > 0 {
		out["channels"] = channels
	}
	if m, ok := value.(map[string]any); ok {
		copyMetaKey(out, m, "pages_fetched")
		copyMetaKey(out, m, "pages_failed")
		copyMetaKey(out, m, "partial")
		copyMetaKey(out, m, "has_more")
		copyMetaKey(out, m, "next_page")
		copyMetaKey(out, m, "failed_at_page")
		copyMetaKey(out, m, "rate_limit_wait_ms")
		copyMetaKey(out, m, "page_all")
		copyMetaKey(out, m, "output_file")
	}
	return out
}

func rowCount(value any) int {
	return len(rowsFromAny(value))
}

func rowsFromAny(value any) []map[string]any {
	switch v := value.(type) {
	case []map[string]any:
		return v
	case []any:
		rows := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if row, ok := item.(map[string]any); ok {
				rows = append(rows, row)
			}
		}
		return rows
	case map[string]any:
		if data, ok := v["data"]; ok {
			return rowsFromAny(data)
		}
		return []map[string]any{v}
	default:
		return nil
	}
}

func copyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func errorCode(err error) string {
	return errorSubtype(err)
}

func errorSubtype(err error) string {
	var te *typedErr
	if errors.As(err, &te) {
		msg := strings.ToLower(te.msg)
		switch te.kind {
		case "auth":
			if strings.Contains(msg, "client_secret") || strings.Contains(msg, "client_id") || strings.Contains(msg, "credential") {
				return "AUTH_MISSING_CREDENTIALS"
			}
			return "AUTH_TOKEN_REFRESH_FAILED"
		case "confirmation_required":
			return "CONFIRMATION_REQUIRED"
		case "rate_limit":
			return "RATE_LIMIT_EXCEEDED"
		case "network":
			return "NETWORK_FAILED"
		case "business":
			if strings.Contains(msg, "openchannelid") || strings.Contains(msg, "channel aliases") {
				return "CHANNEL_MISSING"
			}
			if strings.Contains(msg, "required") && (strings.Contains(msg, "flag") || strings.Contains(msg, "shortcut")) {
				return "VALIDATION_REQUIRED_FLAG"
			}
			if strings.Contains(msg, "must be") || strings.Contains(msg, "invalid") || strings.Contains(msg, "json") {
				return "VALIDATION_BAD_PARAM"
			}
			return "API_BUSINESS_ERROR"
		default:
			return strings.ToUpper(te.kind)
		}
	}
	var ae *auth.TokenError
	if errors.As(err, &ae) {
		if ae.ErrorCode == "invalid_client" {
			return "AUTH_INVALID_CLIENT"
		}
		return "AUTH_TOKEN_REFRESH_FAILED"
	}
	var ce *client.Error
	if errors.As(err, &ce) {
		switch ce.Kind {
		case "auth":
			return errorSubtype(ce.Unwrap())
		case "rate_limit":
			return "RATE_LIMIT_EXCEEDED"
		case "network":
			return "NETWORK_FAILED"
		case "business":
			return "API_BUSINESS_ERROR"
		default:
			return strings.ToUpper(ce.Kind)
		}
	}
	var se *client.StatusError
	if errors.As(err, &se) {
		if se.StatusCode == 429 {
			return "RATE_LIMIT_EXCEEDED"
		}
		if se.StatusCode >= 500 {
			return "HTTP_5XX"
		}
		apiCode, apiMsg := apiErrorFields(se)
		msg := strings.ToLower(fmt.Sprint(apiCode) + " " + apiMsg)
		if strings.Contains(msg, "open_channel_id") || strings.Contains(msg, "openchannelid") {
			return "CHANNEL_INVALID"
		}
		return "API_BUSINESS_ERROR"
	}
	return "API_BUSINESS_ERROR"
}

func retryFields(err error) (bool, int64) {
	var ce *client.Error
	if errors.As(err, &ce) {
		return ce.Retryable, ce.RetryAfter.Milliseconds()
	}
	var se *client.StatusError
	if errors.As(err, &se) {
		return se.Retryable, se.RetryAfter.Milliseconds()
	}
	return false, 0
}

func apiErrorFields(err error) (any, string) {
	var ae *auth.TokenError
	if errors.As(err, &ae) {
		if ae.ErrorCode != "" || ae.ErrorDescription != "" {
			return ae.ErrorCode, ae.ErrorDescription
		}
		return ae.Code, ae.Msg
	}
	var se *client.StatusError
	if errors.As(err, &se) {
		return se.APICode(), se.APIMessage()
	}
	var ce *client.Error
	if errors.As(err, &ce) {
		return apiErrorFields(ce.Unwrap())
	}
	return nil, ""
}

func hintForError(err error) string {
	var te *typedErr
	if errors.As(err, &te) {
		if hint := te.Hint(); hint != "" {
			return hint
		}
		return hintForSubtype(errorSubtype(err))
	}
	var ae *auth.TokenError
	if errors.As(err, &ae) {
		if ae.ErrorCode == "invalid_client" {
			return "verify CaptainBI APPID/client_secret and ensure token requests include scope=all; this CLI sends scope=all automatically"
		}
		return "refresh credentials with cbi config init --client-secret-stdin"
	}
	var ce *client.Error
	if errors.As(err, &ce) {
		switch ce.Kind {
		case "auth":
			if hint := hintForError(ce.Unwrap()); hint != "" {
				return hint
			}
			return "run cbi auth status --machine, then refresh credentials with cbi config init --client-secret-stdin"
		case "rate_limit":
			return "retry after retry_after_ms or reduce --rate-limit"
		case "network":
			return "retry later and check network connectivity"
		}
		return hintForError(ce.Unwrap())
	}
	var se *client.StatusError
	if errors.As(err, &se) {
		if se.StatusCode == 429 {
			return "retry after retry_after_ms or reduce request frequency"
		}
		if se.StatusCode == 401 || se.StatusCode == 403 {
			return "refresh credentials with cbi auth token"
		}
		if se.StatusCode >= 500 {
			return "retry later; CaptainBI returned a server error"
		}
	}
	return hintForSubtype(errorSubtype(err))
}

func hintForSubtype(subtype string) string {
	switch subtype {
	case "AUTH_MISSING_CREDENTIALS":
		return "configure credentials with cbi config init --client-secret-stdin, --client-secret-from-env, --client-secret-file, or CAPTAINBI_ACCESS_TOKEN"
	case "AUTH_INVALID_CLIENT":
		return "verify CaptainBI APPID/client_secret and rerun cbi config init; token requests include scope=all automatically"
	case "AUTH_TOKEN_REFRESH_FAILED":
		return "run cbi auth status --machine, then refresh credentials with cbi auth token or cbi config init"
	case "CHANNEL_MISSING":
		return "run cbi +shops, then pass --channel <alias> or configure cbi config channels add <alias> <open_channel_id>"
	case "CHANNEL_INVALID":
		return "verify the channel alias or OpenChannelId with cbi +shops, then update cbi config channels"
	case "VALIDATION_REQUIRED_FLAG":
		return "run the command with --help and pass the required flag shown in Examples"
	case "VALIDATION_BAD_PARAM":
		return "fix the parameter value according to --help or cbi schema <domain.command>"
	case "RATE_LIMIT_EXCEEDED":
		return "wait retry_after_ms when present, reduce concurrency, or lower --rate-limit"
	case "HTTP_5XX":
		return "retry later; CaptainBI returned a server error"
	case "NETWORK_FAILED":
		return "retry later and check network or proxy settings"
	case "CONFIRMATION_REQUIRED":
		return "use --dry-run to preview, then add --confirm only after explicit user approval"
	case "API_BUSINESS_ERROR":
		return "read api_code/api_msg, fix the request parameters or channel, then retry"
	default:
		return ""
	}
}

func requestID(err error) string {
	var se *client.StatusError
	if errors.As(err, &se) && se.Body != nil {
		for _, key := range []string{"request_id", "requestId", "trace_id", "traceId"} {
			if v, ok := se.Body[key].(string); ok {
				return v
			}
		}
	}
	var ce *client.Error
	if errors.As(err, &ce) {
		return requestID(ce.Unwrap())
	}
	return ""
}
