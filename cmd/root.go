package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
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

const version = "0.1.0-dev"

type globalOptions struct {
	format        string
	machine       bool
	openChannelID string
	rateLimit     int
}

type requestOptions struct {
	dryRun    bool
	jq        string
	pageAll   bool
	pageLimit int
	pageDelay time.Duration
}

var globals globalOptions

func Execute() int {
	root := NewRootCmd()
	if err := root.Execute(); err != nil {
		code := exitCode(err)
		writeError(os.Stderr, err, code)
		return code
	}
	return 0
}

func NewRootCmd() *cobra.Command {
	reg, regErr := registry.Load()
	cmd := &cobra.Command{
		Use:           "cbi",
		Aliases:       []string{"captainbi"},
		Short:         "CaptainBI OpenAPI command-line client",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().StringVar(&globals.format, "format", "json", "output format: json|ndjson|table|csv")
	cmd.PersistentFlags().BoolVar(&globals.machine, "machine", false, "machine mode: pure structured output and semantic exit codes")
	cmd.PersistentFlags().StringVar(&globals.openChannelID, "open-channel-id", "", "CaptainBI OpenChannelId; can also use CAPTAINBI_OPEN_CHANNEL_ID")
	cmd.PersistentFlags().IntVar(&globals.rateLimit, "rate-limit", 0, "requests per minute; default 20 or CAPTAINBI_RATE_LIMIT")

	cmd.AddCommand(newConfigCmd())
	cmd.AddCommand(newAuthCmd())
	cmd.AddCommand(newAPICmd())
	cmd.AddCommand(newSchemaCmd(reg, regErr))
	cmd.AddCommand(newDoctorCmd(reg, regErr))
	cmd.AddCommand(newCompletionCmd(cmd))
	if regErr == nil {
		registerServiceCommands(cmd, reg)
		registerShortcuts(cmd)
	}
	return cmd
}

func loadConfig() (*core.Config, error) {
	cfg, err := core.LoadConfig()
	if err != nil {
		return nil, err
	}
	if globals.openChannelID != "" {
		cfg.OpenChannelID = globals.openChannelID
	}
	if globals.rateLimit > 0 {
		cfg.RateLimit = globals.rateLimit
	}
	return cfg, nil
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Manage CaptainBI CLI configuration"}
	var clientID, baseURL, openChannelID string
	var secretStdin bool
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize client_id/client_secret configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := core.LoadConfig()
			if err != nil {
				return err
			}
			if clientID != "" {
				cfg.ClientID = clientID
			}
			if baseURL != "" {
				cfg.BaseURL = baseURL
			}
			if openChannelID != "" {
				cfg.OpenChannelID = openChannelID
			}
			if cfg.ClientID == "" {
				return typed("auth", "client_id is required; pass --client-id or set CAPTAINBI_CLIENT_ID")
			}
			if secretStdin {
				b, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return err
				}
				secret := strings.TrimSpace(string(b))
				if secret == "" {
					return typed("auth", "client_secret from stdin is empty")
				}
				if err := core.SaveClientSecret(cfg.ClientID, secret); err != nil {
					return typed("auth", "failed to save client_secret to keychain: "+err.Error())
				}
			}
			if err := core.SaveConfig(cfg); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "configuration saved")
			return nil
		},
	}
	initCmd.Flags().StringVar(&clientID, "client-id", "", "CaptainBI APPID/client_id")
	initCmd.Flags().BoolVar(&secretStdin, "client-secret-stdin", false, "read client_secret from stdin")
	initCmd.Flags().StringVar(&baseURL, "base-url", core.DefaultBaseURL, "API base URL")
	initCmd.Flags().StringVar(&openChannelID, "open-channel-id", "", "default OpenChannelId")
	cmd.AddCommand(initCmd)
	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show non-sensitive configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			view := map[string]any{
				"client_id":       security.RedactValue(cfg.ClientID),
				"base_url":        cfg.BaseURL,
				"open_channel_id": security.RedactValue(cfg.OpenChannelID),
				"rate_limit":      cfg.RateLimit,
				"token_expiry":    cfg.TokenExpiry,
			}
			return outfmt.Write(cmd.OutOrStdout(), view, outfmt.Options{Format: globals.format, Machine: globals.machine}, nil)
		},
	}
	cmd.AddCommand(showCmd)
	return cmd
}

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Token management"}
	cmd.AddCommand(&cobra.Command{
		Use:   "token",
		Short: "Fetch or refresh access_token",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if _, err := auth.GetToken(cmd.Context(), cfg, true); err != nil {
				return typed("auth", err.Error())
			}
			return outfmt.Write(cmd.OutOrStdout(), map[string]any{
				"status":       "ok",
				"token_expiry": cfg.TokenExpiry,
			}, outfmt.Options{Format: globals.format, Machine: globals.machine}, nil)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show token status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			return outfmt.Write(cmd.OutOrStdout(), map[string]any{
				"configured":           cfg.ClientID != "",
				"has_cached_token":     cfg.AccessToken != "",
				"token_expiry":         cfg.TokenExpiry,
				"token_seconds_left":   int(time.Until(cfg.TokenExpiry).Seconds()),
				"default_open_channel": cfg.OpenChannelID != "",
			}, outfmt.Options{Format: globals.format, Machine: globals.machine}, nil)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "logout",
		Short: "Remove cached token and keychain secret",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := core.LoadConfig()
			if err != nil {
				return err
			}
			core.DeleteClientSecret(cfg.ClientID)
			cfg.AccessToken = ""
			cfg.TokenExpiry = time.Time{}
			if err := core.SaveConfig(cfg); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "logged out")
			return nil
		},
	})
	return cmd
}

func newAPICmd() *cobra.Command {
	var params, data, jq string
	var dryRun, pageAll bool
	var pageLimit, pageDelay int
	cmd := &cobra.Command{
		Use:   "api <METHOD> <PATH>",
		Short: "Call any CaptainBI OpenAPI endpoint",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			query, body, err := parseMaps(params, data, cmd.InOrStdin())
			if err != nil {
				return err
			}
			req := client.Request{Method: strings.ToUpper(args[0]), Path: args[1], Query: query, Body: body}
			return runRequest(cmd, registry.Method{HTTPMethod: req.Method, FullPath: req.Path, RiskLevel: "read", Pagination: registry.Pagination{Type: "none"}}, req, requestOptions{dryRun: dryRun, jq: jq, pageAll: pageAll, pageLimit: pageLimit, pageDelay: time.Duration(pageDelay) * time.Millisecond})
		},
	}
	cmd.Flags().StringVar(&params, "params", "", "query parameters JSON; supports - for stdin")
	cmd.Flags().StringVar(&data, "data", "", "request body JSON; supports - for stdin")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print request without sending it")
	cmd.Flags().StringVar(&jq, "jq", "", "gojq expression to filter JSON output")
	cmd.Flags().BoolVar(&pageAll, "page-all", false, "automatically fetch all pages for page_rows endpoints")
	cmd.Flags().IntVar(&pageLimit, "page-limit", 10, "max pages to fetch with --page-all; 0 means unlimited")
	cmd.Flags().IntVar(&pageDelay, "page-delay", 3000, "delay in milliseconds between pages")
	return cmd
}

func newSchemaCmd(reg *registry.Registry, regErr error) *cobra.Command {
	var jq string
	cmd := &cobra.Command{
		Use:   "schema <domain.command>",
		Short: "Show endpoint parameter schema",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if regErr != nil {
				return regErr
			}
			m, ok := reg.Find(args[0])
			if !ok {
				return fmt.Errorf("schema %q not found", args[0])
			}
			return outfmt.Write(cmd.OutOrStdout(), m, outfmt.Options{Format: globals.format, Machine: globals.machine, JQ: jq}, nil)
		},
	}
	cmd.Flags().StringVar(&jq, "jq", "", "gojq expression to filter JSON output")
	return cmd
}

func newDoctorCmd(reg *registry.Registry, regErr error) *cobra.Command {
	cmd := &cobra.Command{Use: "doctor", Short: "Run local or contract checks"}
	cmd.AddCommand(&cobra.Command{
		Use:   "local",
		Short: "Run local-only checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if regErr != nil {
				return regErr
			}
			return outfmt.Write(cmd.OutOrStdout(), map[string]any{
				"config_loads":      true,
				"client_configured": cfg.ClientID != "",
				"registry_methods":  len(reg.AllMethods()),
				"registry_services": len(reg.Services),
			}, outfmt.Options{Format: globals.format, Machine: globals.machine}, nil)
		},
	})
	var sample int
	contract := &cobra.Command{
		Use:   "contract",
		Short: "Run limited live read-only checks against CaptainBI",
		RunE: func(cmd *cobra.Command, args []string) error {
			if regErr != nil {
				return regErr
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			c := client.New(cfg, func(ctx context.Context, force bool) (string, error) { return auth.GetToken(ctx, cfg, force) })
			checked := 0
			results := []map[string]any{}
			for _, m := range reg.AllMethods() {
				if checked >= sample {
					break
				}
				if m.RiskLevel != "read" || m.RequiresOpenChannelID {
					continue
				}
				resp, err := c.Do(cmd.Context(), client.Request{Method: m.HTTPMethod, Path: m.FullPath})
				results = append(results, map[string]any{"path": m.FullPath, "ok": err == nil, "error": errString(err), "code": resp["code"]})
				checked++
			}
			return outfmt.Write(cmd.OutOrStdout(), results, outfmt.Options{Format: globals.format, Machine: globals.machine}, nil)
		},
	}
	contract.Flags().IntVar(&sample, "sample", 5, "max read-only endpoints to check")
	cmd.AddCommand(contract)
	return cmd
}

func newCompletionCmd(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{Use: "completion", Short: "Generate shell completion"}
	cmd.AddCommand(&cobra.Command{Use: "bash", RunE: func(cmd *cobra.Command, args []string) error { return root.GenBashCompletion(cmd.OutOrStdout()) }})
	cmd.AddCommand(&cobra.Command{Use: "zsh", RunE: func(cmd *cobra.Command, args []string) error { return root.GenZshCompletion(cmd.OutOrStdout()) }})
	cmd.AddCommand(&cobra.Command{Use: "fish", RunE: func(cmd *cobra.Command, args []string) error { return root.GenFishCompletion(cmd.OutOrStdout(), true) }})
	return cmd
}

func registerServiceCommands(root *cobra.Command, reg *registry.Registry) {
	for _, svc := range reg.Services {
		service := svc
		svcCmd := &cobra.Command{Use: service.Domain, Short: service.DisplayName}
		for _, method := range service.Methods {
			m := method
			var params, data, jq string
			var dryRun, confirm, yes, pageAll bool
			var pageLimit, pageDelay int
			endpointCmd := &cobra.Command{
				Use:   m.CommandName,
				Short: m.Summary,
				RunE: func(cmd *cobra.Command, args []string) error {
					query, body, err := collectEndpointInput(cmd, m, params, data)
					if err != nil {
						return err
					}
					if err := enforceRisk(cmd, m, confirm, yes, dryRun); err != nil {
						return err
					}
					req := client.Request{Method: m.HTTPMethod, Path: m.FullPath, Query: query, Body: body}
					return runRequest(cmd, m, req, requestOptions{dryRun: dryRun, jq: jq, pageAll: pageAll, pageLimit: pageLimit, pageDelay: time.Duration(pageDelay) * time.Millisecond})
				},
			}
			for _, p := range m.Params {
				if p.Location == "header" && strings.EqualFold(p.Name, "authorization") {
					continue
				}
				if p.Location == "header" && strings.EqualFold(p.Name, "OpenChannelId") {
					continue
				}
				endpointCmd.Flags().String(p.Flag, defaultString(p.Default), p.Description)
			}
			endpointCmd.Flags().StringVar(&params, "params", "", "extra query parameters JSON")
			endpointCmd.Flags().StringVar(&data, "data", "", "request body JSON")
			endpointCmd.Flags().BoolVar(&dryRun, "dry-run", false, "print request without sending it")
			endpointCmd.Flags().BoolVar(&confirm, "confirm", false, "confirm dangerous or sync-triggering write")
			endpointCmd.Flags().BoolVar(&yes, "yes", false, "skip prompt for write_safe commands")
			endpointCmd.Flags().StringVar(&jq, "jq", "", "gojq expression to filter JSON output")
			endpointCmd.Flags().BoolVar(&pageAll, "page-all", false, "automatically fetch all pages for page_rows endpoints")
			endpointCmd.Flags().IntVar(&pageLimit, "page-limit", 10, "max pages to fetch with --page-all; 0 means unlimited")
			endpointCmd.Flags().IntVar(&pageDelay, "page-delay", 3000, "delay in milliseconds between pages")
			svcCmd.AddCommand(endpointCmd)
		}
		root.AddCommand(svcCmd)
	}
}

func registerShortcuts(root *cobra.Command) {
	shortcut := func(use, short, domainRef string) *cobra.Command {
		return &cobra.Command{
			Use:   use,
			Short: short,
			RunE: func(cmd *cobra.Command, args []string) error {
				reg, err := registry.Load()
				if err != nil {
					return err
				}
				m, ok := reg.Find(domainRef)
				if !ok {
					return fmt.Errorf("shortcut target %s not found", domainRef)
				}
				query := map[string]string{}
				for _, p := range m.Params {
					if p.Location == "query" {
						if p.Name == "page" {
							query[p.Name] = "1"
						} else if p.Name == "rows" {
							query[p.Name] = "100"
						}
					}
				}
				return runRequest(cmd, m, client.Request{Method: m.HTTPMethod, Path: m.FullPath, Query: query}, requestOptions{})
			},
		}
	}
	root.AddCommand(shortcut("+shops", "List shops", "goods.shops"))
	root.AddCommand(shortcut("+sites", "List sites", "goods.sites"))
	root.AddCommand(shortcut("+orders", "List orders", "sales.orders"))
	root.AddCommand(shortcut("+goods", "List goods", "goods.list"))
	root.AddCommand(shortcut("+finance-daily", "Get store daily finance report", "finance.store-daily"))
}

func collectEndpointInput(cmd *cobra.Command, m registry.Method, extraParams, data string) (map[string]string, any, error) {
	query, body, err := parseMaps(extraParams, data, cmd.InOrStdin())
	if err != nil {
		return nil, nil, err
	}
	for _, p := range m.Params {
		if p.Location != "query" {
			continue
		}
		v, _ := cmd.Flags().GetString(p.Flag)
		if v == "" {
			v = defaultString(p.Default)
		}
		if p.Required && v == "" {
			return nil, nil, fmt.Errorf("required flag --%s is missing", p.Flag)
		}
		if v != "" {
			query[p.Name] = v
		}
	}
	return query, body, nil
}

func runRequest(cmd *cobra.Command, m registry.Method, req client.Request, opts requestOptions) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if req.OpenChannelID == "" {
		req.OpenChannelID = cfg.OpenChannelID
	}
	if m.RequiresOpenChannelID && req.OpenChannelID == "" {
		return typed("business", "OpenChannelId is required; pass --open-channel-id or set CAPTAINBI_OPEN_CHANNEL_ID")
	}
	if opts.dryRun {
		view := map[string]any{
			"method":  req.Method,
			"path":    req.Path,
			"query":   req.Query,
			"body":    req.Body,
			"headers": map[string]any{"authorization": security.RedactValue("bearer token"), "OpenChannelId": security.RedactValue(req.OpenChannelID)},
		}
		return outfmt.Write(cmd.OutOrStdout(), view, outfmt.Options{Format: globals.format, Machine: globals.machine}, nil)
	}
	c := client.New(cfg, func(ctx context.Context, force bool) (string, error) { return auth.GetToken(ctx, cfg, force) })
	resp, err := executeRequest(cmd.Context(), c, m, req, opts)
	if err != nil {
		return err
	}
	return outfmt.Write(cmd.OutOrStdout(), resp, outfmt.Options{Format: globals.format, Machine: globals.machine, JQ: opts.jq}, m.TableColumns)
}

func executeRequest(ctx context.Context, c *client.Client, m registry.Method, req client.Request, opts requestOptions) (map[string]any, error) {
	if !opts.pageAll || m.Pagination.Type != "page_rows" {
		return c.Do(ctx, req)
	}
	if req.Query == nil {
		req.Query = map[string]string{}
	}
	if req.Query["rows"] == "" {
		req.Query["rows"] = "100"
	}
	rows, _ := strconv.Atoi(req.Query["rows"])
	if rows <= 0 {
		rows = 100
	}
	limit := opts.pageLimit
	if limit < 0 {
		limit = 10
	}
	delay := opts.pageDelay
	if delay <= 0 {
		delay = 3 * time.Second
	}
	all := []any{}
	var envelope map[string]any
	for page := 1; ; page++ {
		if limit > 0 && page > limit {
			break
		}
		req.Query["page"] = strconv.Itoa(page)
		resp, err := c.Do(ctx, req)
		if err != nil {
			return nil, err
		}
		if envelope == nil {
			envelope = resp
		}
		data, _ := resp["data"].([]any)
		all = append(all, data...)
		maxResult := intFrom(resp["max_result"])
		if len(data) < rows {
			break
		}
		if maxResult > 0 && len(all) >= maxResult {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	if envelope == nil {
		envelope = map[string]any{}
	}
	envelope["data"] = all
	envelope["page_all"] = true
	envelope["fetched_rows"] = len(all)
	return envelope, nil
}

func intFrom(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case float64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(x)
		return n
	default:
		return 0
	}
}

func enforceRisk(cmd *cobra.Command, m registry.Method, confirm, yes, dryRun bool) error {
	if dryRun || m.RiskLevel == "read" {
		return nil
	}
	if m.RiskLevel == "write_safe" {
		if yes || globals.machine {
			return nil
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s is a write operation; continue? [y/N] ", m.CommandName)
		line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if strings.TrimSpace(strings.ToLower(line)) != "y" {
			return typed("business", "operation cancelled")
		}
		return nil
	}
	if !confirm {
		return typed("business", "this operation requires --confirm; use --dry-run to preview")
	}
	if m.RiskLevel == "sync_trigger" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: this operation may trigger external CaptainBI/Amazon synchronization")
	}
	return nil
}

func parseMaps(params, data string, stdin io.Reader) (map[string]string, any, error) {
	query := map[string]string{}
	if params != "" {
		raw, err := readArg(params, stdin)
		if err != nil {
			return nil, nil, err
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			return nil, nil, err
		}
		for k, v := range m {
			query[k] = fmt.Sprint(v)
		}
	}
	var body any
	if data != "" {
		raw, err := readArg(data, stdin)
		if err != nil {
			return nil, nil, err
		}
		if err := json.Unmarshal([]byte(raw), &body); err != nil {
			return nil, nil, err
		}
	}
	return query, body, nil
}

func readArg(v string, stdin io.Reader) (string, error) {
	if v == "-" {
		b, err := io.ReadAll(stdin)
		return string(b), err
	}
	if strings.HasPrefix(v, "@") {
		b, err := os.ReadFile(strings.TrimPrefix(v, "@"))
		return string(b), err
	}
	return v, nil
}

func defaultString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		if x == float64(int(x)) {
			return strconv.Itoa(int(x))
		}
		return fmt.Sprint(x)
	default:
		return fmt.Sprint(x)
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

type typedErr struct {
	kind string
	msg  string
}

func typed(kind, msg string) error { return &typedErr{kind: kind, msg: msg} }
func (e *typedErr) Error() string  { return e.msg }

func exitCode(err error) int {
	var te *typedErr
	if errors.As(err, &te) {
		switch te.kind {
		case "auth":
			return 2
		case "network":
			return 3
		case "rate_limit":
			return 4
		default:
			return 1
		}
	}
	var ce *client.Error
	if errors.As(err, &ce) {
		switch ce.Kind {
		case "auth":
			return 2
		case "network":
			return 3
		case "rate_limit":
			return 4
		default:
			return 1
		}
	}
	return 1
}

func writeError(w io.Writer, err error, code int) {
	if globals.machine {
		kind := "business"
		var te *typedErr
		if errors.As(err, &te) {
			kind = te.kind
		}
		var ce *client.Error
		if errors.As(err, &ce) {
			kind = ce.Kind
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":      false,
			"code":    code,
			"kind":    kind,
			"message": security.RedactString(err.Error()),
		})
		return
	}
	fmt.Fprintln(w, security.RedactString(err.Error()))
}

func commandNames(cmd *cobra.Command) []string {
	var names []string
	for _, c := range cmd.Commands() {
		if !c.Hidden {
			names = append(names, c.Name())
		}
	}
	sort.Strings(names)
	return names
}
