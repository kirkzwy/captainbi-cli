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
				return nil, typed("business", "no configured channel aliases; run `cbi config channels add <alias> <open-channel-id>`")
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
		return nil, typed("business", "OpenChannelId is required; pass --open-channel-id, --channel, --channel-file, or configure CAPTAINBI_OPEN_CHANNEL_ID")
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
	return nil, typed("business", "channel file must be JSON object, array of strings, or array of {alias,open_channel_id}")
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
		if v, ok := resp["partial"]; ok {
			out["partial"] = v
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
		return outfmt.Write(cmd.OutOrStdout(), map[string]any{
			"ok":          true,
			"output_file": globals.outputFile,
			"rows":        rowCount(value),
		}, outfmt.Options{Format: "json", Machine: globals.machine}, nil)
	}
	return outfmt.Write(cmd.OutOrStdout(), value, outfmt.Options{Format: globals.format, Machine: globals.machine, JQ: jq}, columns)
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
	for _, row := range rows {
		for key := range row {
			fields[key]++
		}
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return map[string]any{
		"ok":     true,
		"rows":   len(rows),
		"fields": keys,
	}
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
	var te *typedErr
	if errors.As(err, &te) {
		return strings.ToUpper(te.kind)
	}
	var ce *client.Error
	if errors.As(err, &ce) {
		return strings.ToUpper(ce.Kind)
	}
	var se *client.StatusError
	if errors.As(err, &se) {
		if se.StatusCode == 429 {
			return "RATE_LIMIT"
		}
		return fmt.Sprintf("HTTP_%d", se.StatusCode)
	}
	return "BUSINESS"
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
