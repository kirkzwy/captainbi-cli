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

var version = "0.2.3-dev"

type globalOptions struct {
	format        string
	machine       bool
	openChannelID string
	channel       string
	channelFile   string
	rateLimit     int
	outputFile    string
	limit         int
	summary       bool
	verbose       bool
	debug         bool
	auditLog      string
}

type requestOptions struct {
	dryRun     bool
	jq         string
	pageAll    bool
	pageLimit  int
	pageDelay  time.Duration
	maxRecords int
	resumePage int
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
	cmd.PersistentFlags().StringVar(&globals.channel, "channel", "", "channel alias from config; use all to run configured channel set")
	cmd.PersistentFlags().StringVar(&globals.channelFile, "channel-file", "", "JSON file containing channel aliases or OpenChannelId values")
	cmd.PersistentFlags().IntVar(&globals.rateLimit, "rate-limit", 0, "requests per minute; default 20 or CAPTAINBI_RATE_LIMIT")
	cmd.PersistentFlags().StringVar(&globals.outputFile, "output-file", "", "write command data output to file instead of stdout")
	cmd.PersistentFlags().IntVar(&globals.limit, "limit", 0, "limit rows written to stdout or output file")
	cmd.PersistentFlags().BoolVar(&globals.summary, "summary", false, "write a compact summary instead of full rows")
	cmd.PersistentFlags().BoolVar(&globals.verbose, "verbose", false, "write redacted request diagnostics to stderr")
	cmd.PersistentFlags().BoolVar(&globals.debug, "debug", false, "write redacted request and response diagnostics to stderr")
	cmd.PersistentFlags().StringVar(&globals.auditLog, "audit-log", "", "append redacted audit records to this file")

	cmd.AddCommand(newConfigCmd())
	cmd.AddCommand(newAuthCmd())
	cmd.AddCommand(newAPICmd())
	cmd.AddCommand(newSchemaCmd(reg, regErr))
	cmd.AddCommand(newDoctorCmd(reg, regErr))
	cmd.AddCommand(newCompletionCmd(cmd))
	cmd.AddCommand(newToolsCmd(reg, regErr))
	cmd.AddCommand(newRegistryCmd(reg, regErr))
	cmd.AddCommand(newRateLimitCmd())
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
	var clientID, baseURL, openChannelID, secretFile string
	var secretStdin, secretFromEnv, nonInteractive bool
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
			if secretFile != "" {
				cfg.PlainSecretFile = secretFile
				cfg.UsePlainSecret = true
				cfg.PlainSecretHint = "file:" + secretFile
			}
			if cfg.ClientID == "" && os.Getenv(core.EnvAccessToken) == "" {
				return typedH("auth", "client_id is required; pass --client-id or set CAPTAINBI_CLIENT_ID", "run cbi config init --client-id <CAPTAINBI_CLIENT_ID> --client-secret-stdin --non-interactive")
			}
			if secretFromEnv && os.Getenv(core.EnvClientSecret) == "" {
				return typedH("auth", "CAPTAINBI_CLIENT_SECRET is required when using --client-secret-from-env", "export CAPTAINBI_CLIENT_SECRET or use --client-secret-file")
			}
			if secretFromEnv {
				cfg.UsePlainSecret = false
				cfg.PlainSecretHint = "env:" + core.EnvClientSecret
			}
			if nonInteractive && !secretStdin && !secretFromEnv && secretFile == "" && os.Getenv(core.EnvAccessToken) == "" {
				return typedH("auth", "non-interactive init requires --client-secret-stdin, --client-secret-from-env, --client-secret-file, or CAPTAINBI_ACCESS_TOKEN", "provide one non-interactive secret source")
			}
			if secretStdin {
				b, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return err
				}
				secret := strings.TrimSpace(string(b))
				if secret == "" {
					return typedH("auth", "client_secret from stdin is empty", "pipe the CaptainBI client_secret into stdin")
				}
				if err := core.SaveClientSecret(cfg.ClientID, secret); err != nil {
					return typedH("auth", "failed to save client_secret to keychain: "+err.Error(), "use --client-secret-file or CAPTAINBI_CLIENT_SECRET in headless environments")
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
	initCmd.Flags().BoolVar(&secretFromEnv, "client-secret-from-env", false, "read client_secret from CAPTAINBI_CLIENT_SECRET")
	initCmd.Flags().StringVar(&secretFile, "client-secret-file", "", "read client_secret from file at runtime")
	initCmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "fail instead of prompting; suitable for agents and CI")
	initCmd.Flags().StringVar(&baseURL, "base-url", core.DefaultBaseURL, "API base URL")
	initCmd.Flags().StringVar(&openChannelID, "open-channel-id", "", "default OpenChannelId")
	cmd.AddCommand(initCmd)
	cmd.AddCommand(newChannelsCmd())
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
				"channels_count":  len(cfg.Channels),
				"plain_secret":    cfg.PlainSecretHint != "",
			}
			return outfmt.Write(cmd.OutOrStdout(), view, outfmt.Options{Format: globals.format, Machine: globals.machine}, nil)
		},
	}
	cmd.AddCommand(showCmd)
	return cmd
}

func newChannelsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "channels", Short: "Manage OpenChannelId aliases"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List configured channel aliases",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			rows := []map[string]any{}
			for alias, id := range cfg.Channels {
				rows = append(rows, map[string]any{"alias": alias, "open_channel_id": security.RedactValue(id)})
			}
			return writeValue(cmd, rows, nil, "")
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "add <alias> <open-channel-id>",
		Short: "Add or update a channel alias",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := core.LoadConfig()
			if err != nil {
				return err
			}
			if cfg.Channels == nil {
				cfg.Channels = map[string]string{}
			}
			cfg.Channels[args[0]] = args[1]
			if err := core.SaveConfig(cfg); err != nil {
				return err
			}
			return writeValue(cmd, map[string]any{"ok": true, "alias": args[0]}, nil, "")
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "remove <alias>",
		Short: "Remove a channel alias",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := core.LoadConfig()
			if err != nil {
				return err
			}
			delete(cfg.Channels, args[0])
			if err := core.SaveConfig(cfg); err != nil {
				return err
			}
			return writeValue(cmd, map[string]any{"ok": true, "alias": args[0]}, nil, "")
		},
	})
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
				return &client.Error{Kind: "auth", Err: err}
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
	var params, data, paramsFile, dataFile, jq string
	var dryRun, pageAll, paramsStdin, dataStdin bool
	var pageLimit, pageDelay, maxRecords, resumePage int
	cmd := &cobra.Command{
		Use:   "api <METHOD> <PATH>",
		Short: "Call any CaptainBI OpenAPI endpoint",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			query, body, err := parseMaps(inputOptions{params: params, data: data, paramsFile: paramsFile, dataFile: dataFile, paramsStdin: paramsStdin, dataStdin: dataStdin}, cmd.InOrStdin())
			if err != nil {
				return err
			}
			req := client.Request{Method: strings.ToUpper(args[0]), Path: args[1], Query: query, Body: body}
			return runRequest(cmd, registry.Method{HTTPMethod: req.Method, FullPath: req.Path, RiskLevel: "read", Pagination: registry.Pagination{Type: "none"}}, req, requestOptions{dryRun: dryRun, jq: jq, pageAll: pageAll, pageLimit: pageLimit, pageDelay: time.Duration(pageDelay) * time.Millisecond, maxRecords: maxRecords, resumePage: resumePage})
		},
	}
	cmd.Flags().StringVar(&params, "params", "", "query parameters JSON; supports - for stdin")
	cmd.Flags().StringVar(&data, "data", "", "request body JSON; supports - for stdin")
	cmd.Flags().BoolVar(&paramsStdin, "params-stdin", false, "read query parameter JSON from stdin")
	cmd.Flags().BoolVar(&dataStdin, "data-stdin", false, "read request body JSON from stdin")
	cmd.Flags().StringVar(&paramsFile, "params-file", "", "read query parameter JSON from file")
	cmd.Flags().StringVar(&dataFile, "data-file", "", "read request body JSON from file")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print request without sending it")
	cmd.Flags().StringVar(&jq, "jq", "", "gojq expression to filter JSON output")
	cmd.Flags().BoolVar(&pageAll, "page-all", false, "automatically fetch all pages for page_rows endpoints")
	cmd.Flags().IntVar(&pageLimit, "page-limit", 10, "max pages to fetch with --page-all; 0 means unlimited")
	cmd.Flags().IntVar(&pageDelay, "page-delay", 3000, "delay in milliseconds between pages")
	cmd.Flags().IntVar(&maxRecords, "max-records", 0, "stop page-all after collecting this many records")
	cmd.Flags().IntVar(&resumePage, "resume-from-page", 1, "start page for --page-all resume")
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
			if globals.format == "openai-tool" {
				return outfmt.Write(cmd.OutOrStdout(), openAIToolSchema(m), outfmt.Options{Format: "json", Machine: globals.machine, JQ: jq}, nil)
			}
			return outfmt.Write(cmd.OutOrStdout(), schemaView(m), outfmt.Options{Format: globals.format, Machine: globals.machine, JQ: jq}, nil)
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
			dir, _ := core.ConfigDir()
			rateStatus, _ := client.RateLimitStatus(cfg)
			return outfmt.Write(cmd.OutOrStdout(), map[string]any{
				"config_loads":              true,
				"client_configured":         cfg.ClientID != "",
				"has_access_token":          cfg.AccessToken != "",
				"keyring_available":         core.KeyringAvailable(),
				"headless_secret_supported": os.Getenv(core.EnvClientSecret) != "" || os.Getenv(core.EnvAccessToken) != "" || cfg.PlainSecretFile != "",
				"headless_recommendation":   "use CAPTAINBI_ACCESS_TOKEN, CAPTAINBI_CLIENT_SECRET, or cbi config init --client-secret-file",
				"registry_methods":          len(reg.AllMethods()),
				"registry_services":         len(reg.Services),
				"rate_limit_per_minute":     cfg.RateLimit,
				"rate_limit_lock_file":      dir + "/rate_limiter.lock",
				"rate_limit_state_file":     dir + "/rate_limiter.next",
				"rate_limit_wait_ms":        rateStatus["wait_ms"],
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
			var params, data, paramsFile, dataFile, jq string
			var dryRun, confirm, yes, pageAll, paramsStdin, dataStdin bool
			var pageLimit, pageDelay, maxRecords, resumePage int
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
					return runRequest(cmd, m, req, requestOptions{dryRun: dryRun, jq: jq, pageAll: pageAll, pageLimit: pageLimit, pageDelay: time.Duration(pageDelay) * time.Millisecond, maxRecords: maxRecords, resumePage: resumePage})
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
			endpointCmd.Flags().BoolVar(&paramsStdin, "params-stdin", false, "read query parameter JSON from stdin")
			endpointCmd.Flags().BoolVar(&dataStdin, "data-stdin", false, "read request body JSON from stdin")
			endpointCmd.Flags().StringVar(&paramsFile, "params-file", "", "read query parameter JSON from file")
			endpointCmd.Flags().StringVar(&dataFile, "data-file", "", "read request body JSON from file")
			endpointCmd.Flags().BoolVar(&dryRun, "dry-run", false, "print request without sending it")
			endpointCmd.Flags().BoolVar(&confirm, "confirm", false, "confirm dangerous or sync-triggering write")
			endpointCmd.Flags().BoolVar(&yes, "yes", false, "skip prompt for write_safe commands")
			endpointCmd.Flags().StringVar(&jq, "jq", "", "gojq expression to filter JSON output")
			endpointCmd.Flags().BoolVar(&pageAll, "page-all", false, "automatically fetch all pages for page_rows endpoints")
			endpointCmd.Flags().IntVar(&pageLimit, "page-limit", 10, "max pages to fetch with --page-all; 0 means unlimited")
			endpointCmd.Flags().IntVar(&pageDelay, "page-delay", 3000, "delay in milliseconds between pages")
			endpointCmd.Flags().IntVar(&maxRecords, "max-records", 0, "stop page-all after collecting this many records")
			endpointCmd.Flags().IntVar(&resumePage, "resume-from-page", 1, "start page for --page-all resume")
			svcCmd.AddCommand(endpointCmd)
		}
		root.AddCommand(svcCmd)
	}
}

func registerShortcuts(root *cobra.Command) {
	shortcut := func(use, short, domainRef, example string, configure func(*cobra.Command, map[string]*string)) *cobra.Command {
		values := map[string]*string{}
		var dryRun, pageAll bool
		var jq string
		var pageLimit, pageDelay, maxRecords, resumePage int
		c := &cobra.Command{
			Use:     use,
			Short:   short,
			Example: example,
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
				for name, ptr := range values {
					if ptr != nil && *ptr != "" {
						query[name] = *ptr
					}
				}
				for _, p := range m.Params {
					if p.Location == "query" && p.Required && query[p.Name] == "" {
						return typedH("business", "required shortcut flag is missing for "+p.Name, "pass the required shortcut flag shown in --help")
					}
				}
				return runRequest(cmd, m, client.Request{Method: m.HTTPMethod, Path: m.FullPath, Query: query}, requestOptions{dryRun: dryRun, jq: jq, pageAll: pageAll, pageLimit: pageLimit, pageDelay: time.Duration(pageDelay) * time.Millisecond, maxRecords: maxRecords, resumePage: resumePage})
			},
		}
		c.Flags().BoolVar(&dryRun, "dry-run", false, "print request without sending it")
		c.Flags().StringVar(&jq, "jq", "", "gojq expression to filter JSON output")
		c.Flags().BoolVar(&pageAll, "page-all", false, "automatically fetch all pages for page_rows endpoints")
		c.Flags().IntVar(&pageLimit, "page-limit", 10, "max pages to fetch with --page-all; 0 means unlimited")
		c.Flags().IntVar(&pageDelay, "page-delay", 3000, "delay in milliseconds between pages")
		c.Flags().IntVar(&maxRecords, "max-records", 0, "stop page-all after collecting this many records")
		c.Flags().IntVar(&resumePage, "resume-from-page", 1, "start page for --page-all resume")
		if configure != nil {
			configure(c, values)
		}
		return c
	}
	root.AddCommand(shortcut("+shops", "List shops", "goods.shops", "  cbi +shops --machine --format json\n  cbi +shops --summary --machine", nil))
	root.AddCommand(shortcut("+sites", "List sites", "goods.sites", "  cbi +sites --machine --format json", nil))
	root.AddCommand(shortcut("+orders", "List orders", "sales.orders", "  cbi --channel main +orders --start 1781424057 --end 1781510457 --summary --machine\n  cbi --channel all +orders --start 1781424057 --end 1781510457 --page-all --max-records 500 --machine", func(cmd *cobra.Command, values map[string]*string) {
		start, end := "", ""
		values["start_modified_time"] = &start
		values["end_modified_time"] = &end
		cmd.Flags().StringVar(&start, "start", "", "start modified timestamp")
		cmd.Flags().StringVar(&end, "end", "", "end modified timestamp")
	}))
	root.AddCommand(shortcut("+goods", "List goods", "goods.list", "  cbi --channel main +goods --modified-since 1781424057 --modified-until 1781510457 --summary --machine\n  cbi --channel main +goods --modified-since 1781424057 --modified-until 1781510457 --page-all --max-records 500 --machine", func(cmd *cobra.Command, values map[string]*string) {
		modifiedSince, end := "", ""
		values["start_modified_time"] = &modifiedSince
		values["end_modified_time"] = &end
		cmd.Flags().StringVar(&modifiedSince, "modified-since", "", "start modified timestamp")
		cmd.Flags().StringVar(&end, "modified-until", "", "end modified timestamp")
	}))
	root.AddCommand(shortcut("+finance-daily", "Get store daily finance report", "finance.store-daily", "  cbi --channel main +finance-daily --date 20260615 --summary --machine\n  cbi --channel all +finance-daily --date 20260615 --summary --machine", func(cmd *cobra.Command, values map[string]*string) {
		date := ""
		values["report_date"] = &date
		cmd.Flags().StringVar(&date, "date", "", "report date, for example 20260615")
	}))
	root.AddCommand(shortcut("+inventory", "List FBA inventory", "fba.inventory", "  cbi --channel main +inventory --modified-since 1781424057 --modified-until 1781510457 --summary --machine\n  cbi --channel main +inventory --modified-since 1781424057 --modified-until 1781510457 --page-all --max-records 500 --machine", func(cmd *cobra.Command, values map[string]*string) {
		modifiedSince, end := "", ""
		values["start_modified_time"] = &modifiedSince
		values["end_modified_time"] = &end
		cmd.Flags().StringVar(&modifiedSince, "modified-since", "", "start modified timestamp")
		cmd.Flags().StringVar(&end, "modified-until", "", "end modified timestamp")
	}))
	root.AddCommand(shortcut("+ads-campaigns", "List advertising campaigns", "ads.advertise-campaign", "  cbi --channel main +ads-campaigns --summary --machine\n  cbi --channel all +ads-campaigns --summary --machine", nil))
	root.AddCommand(shortcut("+ads-campaign-report", "Get advertising campaign report", "ads.advertise-campaign-report", "  cbi --channel main +ads-campaign-report --summary --machine\n  cbi --channel main +ads-campaign-report --output-file ads-campaigns.json --machine", nil))
	root.AddCommand(shortcut("+reviews", "List product reviews", "monitor.reviews", "  cbi --channel main +reviews --summary --machine\n  cbi --channel main +reviews --page-all --max-records 500 --machine", func(cmd *cobra.Command, values map[string]*string) {
		start, end := "", ""
		values["start_modified_time"] = &start
		values["end_modified_time"] = &end
		cmd.Flags().StringVar(&start, "start", "", "optional start modified timestamp")
		cmd.Flags().StringVar(&end, "end", "", "optional end modified timestamp")
	}))
	root.AddCommand(shortcut("+store-transactions", "List store finance transactions", "finance.store-transactions", "  cbi --channel main +store-transactions --start 20260601 --end 20260615 --summary --machine\n  cbi --channel main +store-transactions --start 20260601 --end 20260615 --page-all --max-records 500 --machine", func(cmd *cobra.Command, values map[string]*string) {
		start, end := "", ""
		values["start_report_time"] = &start
		values["end_report_time"] = &end
		cmd.Flags().StringVar(&start, "start", "", "start report date or timestamp")
		cmd.Flags().StringVar(&end, "end", "", "end report date or timestamp")
	}))
}

func collectEndpointInput(cmd *cobra.Command, m registry.Method, extraParams, data string) (map[string]string, any, error) {
	paramsFile, _ := cmd.Flags().GetString("params-file")
	dataFile, _ := cmd.Flags().GetString("data-file")
	paramsStdin, _ := cmd.Flags().GetBool("params-stdin")
	dataStdin, _ := cmd.Flags().GetBool("data-stdin")
	query, body, err := parseMaps(inputOptions{params: extraParams, data: data, paramsFile: paramsFile, dataFile: dataFile, paramsStdin: paramsStdin, dataStdin: dataStdin}, cmd.InOrStdin())
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
			return nil, nil, typedH("business", fmt.Sprintf("required flag --%s is missing", p.Flag), "run the command with --help and pass all required flags")
		}
		if p.Max > 0 && v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, nil, typedH("business", fmt.Sprintf("flag --%s must be an integer", p.Flag), "pass a numeric value for this flag")
			}
			if n > p.Max {
				return nil, nil, typedH("business", fmt.Sprintf("flag --%s must be <= %d", p.Flag, p.Max), fmt.Sprintf("use --%s %d or a smaller value", p.Flag, p.Max))
			}
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
	targets, err := resolveChannels(cfg, req.OpenChannelID, m.RequiresOpenChannelID)
	if err != nil {
		return err
	}
	if opts.dryRun {
		views := []map[string]any{}
		for _, target := range targets {
			views = append(views, dryRunView(req, target.ID, target.Alias))
		}
		if len(views) == 1 {
			return writeValue(cmd, views[0], nil, opts.jq)
		}
		return writeValue(cmd, views, nil, opts.jq)
	}
	c := client.New(cfg, func(ctx context.Context, force bool) (string, error) { return auth.GetToken(ctx, cfg, force) })
	if len(targets) > 1 {
		results := []map[string]any{}
		for _, target := range targets {
			req.OpenChannelID = target.ID
			resp, err := executeRequest(cmd.Context(), c, m, req, opts)
			results = append(results, channelResult(target, resp, err))
			writeAudit(m, target, err)
		}
		return writeValue(cmd, map[string]any{"ok": true, "channels": results}, nil, opts.jq)
	}
	req.OpenChannelID = targets[0].ID
	resp, err := executeRequest(cmd.Context(), c, m, req, opts)
	writeAudit(m, targets[0], err)
	if err != nil {
		return err
	}
	if globals.verbose || globals.debug {
		fmt.Fprintf(cmd.ErrOrStderr(), "request method=%s path=%s channel=%s rate_limit_wait_ms=%d\n", req.Method, req.Path, security.RedactValue(req.OpenChannelID), c.LastRateLimitWait().Milliseconds())
	}
	if wait := c.LastRateLimitWait(); wait > 0 {
		resp["rate_limit_wait_ms"] = wait.Milliseconds()
	}
	return writeValue(cmd, resp, m.TableColumns, opts.jq)
}

func dryRunView(req client.Request, openChannelID, alias string) map[string]any {
	return map[string]any{
		"method":  req.Method,
		"path":    req.Path,
		"query":   req.Query,
		"body":    req.Body,
		"channel": alias,
		"headers": map[string]any{"authorization": security.RedactValue("bearer token"), "OpenChannelId": security.RedactValue(openChannelID)},
	}
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
	startPage := opts.resumePage
	if startPage <= 0 {
		startPage = 1
	}
	all := []any{}
	var envelope map[string]any
	pagesFetched := 0
	pagesFailed := 0
	failedAtPage := 0
	partialError := ""
	hasMore := false
	nextPage := 0
	for page := startPage; ; page++ {
		if limit > 0 && pagesFetched >= limit {
			hasMore = true
			nextPage = page
			break
		}
		req.Query["page"] = strconv.Itoa(page)
		resp, err := c.Do(ctx, req)
		if err != nil {
			pagesFailed++
			failedAtPage = page
			partialError = err.Error()
			if len(all) == 0 {
				return nil, err
			}
			break
		}
		pagesFetched++
		if envelope == nil {
			envelope = resp
		}
		data, _ := resp["data"].([]any)
		if raw, ok := resp["data"]; ok && raw != nil && data == nil {
			err := typedH("business", "page_rows response data must be an array", "check the endpoint response contract before using --page-all")
			if len(all) == 0 {
				return nil, err
			}
			pagesFailed++
			failedAtPage = page
			partialError = err.Error()
			break
		}
		all = append(all, data...)
		if opts.maxRecords > 0 && len(all) >= opts.maxRecords {
			all = all[:opts.maxRecords]
			hasMore = len(data) >= rows
			if hasMore {
				nextPage = page + 1
			}
			break
		}
		maxResult := intFrom(resp[m.Pagination.TotalField])
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
	envelope["pages_fetched"] = pagesFetched
	envelope["pages_failed"] = pagesFailed
	envelope["partial"] = pagesFailed > 0
	envelope["has_more"] = hasMore
	if nextPage > 0 {
		envelope["next_page"] = nextPage
	}
	if failedAtPage > 0 {
		envelope["failed_at_page"] = failedAtPage
		envelope["partial_error"] = security.RedactString(partialError)
		envelope["has_more"] = true
		envelope["next_page"] = failedAtPage
	}
	envelope["resume_from_page"] = startPage
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
		if yes || agentMode() {
			return nil
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s is a write operation; continue? [y/N] ", m.CommandName)
		line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if strings.TrimSpace(strings.ToLower(line)) != "y" {
			return typedH("business", "operation cancelled", "rerun with --yes after confirming the write operation")
		}
		return nil
	}
	if !confirm {
		return typedH("confirmation_required", "this operation requires --confirm; use --dry-run to preview", "add --confirm only after the user explicitly approves this write or sync operation")
	}
	if m.RiskLevel == "sync_trigger" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: this operation may trigger external CaptainBI/Amazon synchronization")
	}
	return nil
}

type inputOptions struct {
	params      string
	data        string
	paramsFile  string
	dataFile    string
	paramsStdin bool
	dataStdin   bool
}

func parseMaps(opts inputOptions, stdin io.Reader) (map[string]string, any, error) {
	query := map[string]string{}
	if opts.paramsStdin {
		opts.params = "-"
	}
	if opts.dataStdin {
		opts.data = "-"
	}
	if opts.paramsFile != "" {
		opts.params = "@" + opts.paramsFile
	}
	if opts.dataFile != "" {
		opts.data = "@" + opts.dataFile
	}
	if opts.params != "" {
		raw, err := readArg(opts.params, stdin)
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
	if opts.data != "" {
		raw, err := readArg(opts.data, stdin)
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
	hint string
}

func typed(kind, msg string) error        { return &typedErr{kind: kind, msg: msg, hint: hintFor(kind, msg)} }
func typedH(kind, msg, hint string) error { return &typedErr{kind: kind, msg: msg, hint: hint} }
func (e *typedErr) Error() string         { return e.msg }
func (e *typedErr) Hint() string          { return e.hint }
func hintFor(kind, msg string) string {
	switch kind {
	case "auth":
		return "run cbi auth status --machine, then refresh credentials with cbi config init --client-secret-stdin"
	case "rate_limit":
		return "retry later or lower --rate-limit"
	case "confirmation_required":
		return "use --dry-run to preview, then add --confirm after explicit approval"
	case "business":
		if strings.Contains(strings.ToLower(msg), "openchannelid") {
			return "pass --open-channel-id, --channel, --channel-file, or configure CAPTAINBI_OPEN_CHANNEL_ID"
		}
	}
	return ""
}

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
		case "confirmation_required":
			return 10
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
	var se *client.StatusError
	if errors.As(err, &se) {
		if se.StatusCode == 429 {
			return 4
		}
		if se.StatusCode == 401 || se.StatusCode == 403 {
			return 2
		}
		if se.StatusCode >= 500 {
			return 3
		}
	}
	return 1
}

func writeError(w io.Writer, err error, code int) {
	if agentMode() {
		kind := "business"
		var te *typedErr
		if errors.As(err, &te) {
			kind = te.kind
		}
		var ce *client.Error
		if errors.As(err, &ce) {
			kind = ce.Kind
		}
		var se *client.StatusError
		if errors.As(err, &se) {
			if se.StatusCode == 429 {
				kind = "rate_limit"
			} else if se.StatusCode == 401 || se.StatusCode == 403 {
				kind = "auth"
			} else if se.StatusCode >= 500 {
				kind = "network"
			}
		}
		retryable, retryAfterMS := retryFields(err)
		apiCode, apiMsg := apiErrorFields(err)
		subtype := errorCode(err)
		message := security.RedactString(err.Error())
		hint := security.RedactString(hintForError(err))
		requestID := requestID(err)
		errObj := map[string]any{
			"kind":           kind,
			"subtype":        subtype,
			"message":        message,
			"hint":           hint,
			"retryable":      retryable,
			"retry_after_ms": retryAfterMS,
			"api_code":       apiCode,
			"api_msg":        security.RedactString(apiMsg),
			"request_id":     requestID,
		}
		meta := map[string]any{
			"exit_code": code,
			"hints":     []string{},
			"alerts":    []string{},
		}
		if hint != "" {
			meta["hints"] = []string{hint}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":             false,
			"error_code":     subtype,
			"kind":           kind,
			"message":        message,
			"hint":           hint,
			"retryable":      retryable,
			"retry_after_ms": retryAfterMS,
			"api_code":       apiCode,
			"api_msg":        security.RedactString(apiMsg),
			"request_id":     requestID,
			"error":          errObj,
			"meta":           meta,
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
