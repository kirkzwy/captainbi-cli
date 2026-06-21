package cmd

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	outfmt "github.com/kirkzwy/captainbi-cli/internal/output"
	"github.com/kirkzwy/captainbi-cli/internal/registry"
)

func newToolsCmd(reg *registry.Registry, regErr error) *cobra.Command {
	cmd := &cobra.Command{Use: "tools", Short: "Export Agent tool schemas"}
	var format string
	export := &cobra.Command{
		Use:   "export",
		Short: "Export CaptainBI endpoint tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			if regErr != nil {
				return regErr
			}
			switch format {
			case "openai":
				tools := make([]any, 0, len(reg.AllMethods()))
				for _, m := range reg.AllMethods() {
					tools = append(tools, openAIToolSchema(m))
				}
				return outfmt.Write(cmd.OutOrStdout(), tools, outfmt.Options{Format: "json", Machine: globals.machine}, nil)
			case "claude":
				tools := make([]any, 0, len(reg.AllMethods()))
				for _, m := range reg.AllMethods() {
					tools = append(tools, claudeToolSchema(m))
				}
				return outfmt.Write(cmd.OutOrStdout(), tools, outfmt.Options{Format: "json", Machine: globals.machine}, nil)
			default:
				return typed("business", "unsupported tools export format; supported: openai, claude")
			}
		},
	}
	export.Flags().StringVar(&format, "format", "openai", "tool schema format: openai|claude")
	cmd.AddCommand(export)
	return cmd
}

func openAIToolSchema(m registry.Method) map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        toolName(m),
			"description": toolDescription(m),
			"parameters":  requestJSONSchema(m),
		},
	}
}

func claudeToolSchema(m registry.Method) map[string]any {
	return map[string]any{
		"name":         toolName(m),
		"description":  toolDescription(m),
		"input_schema": requestJSONSchema(m),
	}
}

func toolDescription(m registry.Method) string {
	parts := []string{m.Summary, m.HTTPMethod + " " + m.FullPath}
	if m.RiskLevel != "" {
		parts = append(parts, "risk="+m.RiskLevel)
	}
	if m.Pagination.Type != "" {
		parts = append(parts, "pagination="+m.Pagination.Type)
	}
	return strings.Join(parts, " | ")
}

func requestJSONSchema(m registry.Method) map[string]any {
	properties := map[string]any{}
	required := []string{}
	for _, p := range m.Params {
		if p.Location == "header" && strings.EqualFold(p.Name, "authorization") {
			continue
		}
		if p.Location == "header" && strings.EqualFold(p.Name, "OpenChannelId") {
			continue
		}
		schema := map[string]any{"type": jsonSchemaType(p.Type)}
		if p.Description != "" {
			schema["description"] = p.Description
		}
		if p.Default != nil {
			schema["default"] = p.Default
		}
		if p.Max > 0 {
			schema["maximum"] = p.Max
		}
		if p.Min > 0 {
			schema["minimum"] = p.Min
		}
		if len(p.Enum) > 0 {
			schema["enum"] = p.Enum
		}
		if p.Format != "" && p.Format != "string" {
			schema["format"] = p.Format
		}
		properties[p.Flag] = schema
		if p.Required {
			required = append(required, p.Flag)
		}
	}
	if m.RequiresOpenChannelID {
		properties["open_channel_id"] = map[string]any{
			"type":        "string",
			"description": "CaptainBI OpenChannelId. Prefer configured --channel aliases for daily agent usage.",
		}
		required = append(required, "open_channel_id")
	}
	if m.RequestBodySchema != nil && m.HTTPMethod != "GET" && m.HTTPMethod != "HEAD" {
		properties["data"] = m.RequestBodySchema
		if requiresComplexBodyInput(m) {
			required = append(required, "data")
		}
		if m.RiskLevel != "read" {
			properties["dry_run"] = map[string]any{
				"type":        "boolean",
				"description": "Preview request without sending it.",
				"default":     true,
			}
			properties["confirm_request"] = map[string]any{
				"type":        "string",
				"description": "Exact request hash from a current dry-run preview. Use only after explicit user approval and without changing the request.",
			}
		}
	}
	if m.Pagination.Type != "none" {
		properties["page_all"] = map[string]any{"type": "boolean", "description": "Fetch all supported pages.", "default": false}
		properties["max_records"] = map[string]any{"type": "integer", "description": "Stop after this many records.", "minimum": 1}
		properties["resume_from_page"] = map[string]any{"type": "integer", "description": "Start page when resuming a page_all request.", "minimum": 1, "default": 1}
	}
	out := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func jsonSchemaType(t string) string {
	switch strings.ToLower(t) {
	case "integer", "number", "boolean", "array", "object":
		return strings.ToLower(t)
	default:
		return "string"
	}
}

func toolName(m registry.Method) string {
	name := "cbi_" + m.Name
	name = strings.Trim(name, "._-/")
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = regexp.MustCompile(`[^A-Za-z0-9_]+`).ReplaceAllString(name, "_")
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}

func schemaView(m registry.Method) map[string]any {
	return map[string]any{
		"domain_command":           fmt.Sprintf("%s.%s", domainFromName(m.Name), m.CommandName),
		"method":                   m.HTTPMethod,
		"path":                     m.FullPath,
		"summary":                  m.Summary,
		"params":                   m.Params,
		"pagination":               m.Pagination,
		"risk_level":               m.RiskLevel,
		"requires_open_channel_id": m.RequiresOpenChannelID,
		"content_type":             m.ContentType,
		"request_body_required":    m.RequestBodyRequired,
		"request_body_schema":      m.RequestBodySchema,
		"success_codes":            m.SuccessCodes,
		"table_columns":            m.TableColumns,
		"request_schema":           requestJSONSchema(m),
		"response_schema":          m.ResponseSchema,
	}
}

func requiresComplexBodyInput(m registry.Method) bool {
	if !m.RequestBodyRequired {
		return false
	}
	schema, ok := m.RequestBodySchema.(map[string]any)
	if !ok {
		return true
	}
	properties, _ := schema["properties"].(map[string]any)
	represented := map[string]bool{}
	for _, p := range m.Params {
		if p.Location == "form" {
			represented[p.Name] = true
		}
	}
	for _, name := range anyStrings(schema["required"]) {
		if !represented[name] {
			return true
		}
		if property, ok := properties[name].(map[string]any); ok {
			typ := fmt.Sprint(property["type"])
			if typ == "array" || typ == "object" {
				return true
			}
		}
	}
	return false
}

func domainFromName(name string) string {
	parts := strings.Split(name, ".")
	if len(parts) >= 2 {
		switch {
		case strings.Contains(parts[1], "cpc"):
			return "ads"
		case strings.Contains(parts[1], "order"):
			return "sales"
		case strings.Contains(parts[1], "fba"):
			return "fba"
		case strings.Contains(parts[1], "finance"):
			return "finance"
		default:
			return "goods"
		}
	}
	return ""
}
