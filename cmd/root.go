package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kirkzwy/captainbi-cli/internal/approval"
	"github.com/kirkzwy/captainbi-cli/internal/auth"
	"github.com/kirkzwy/captainbi-cli/internal/client"
	"github.com/kirkzwy/captainbi-cli/internal/core"
	internalerrs "github.com/kirkzwy/captainbi-cli/internal/errs"
	outfmt "github.com/kirkzwy/captainbi-cli/internal/output"
	"github.com/kirkzwy/captainbi-cli/internal/registry"
	"github.com/kirkzwy/captainbi-cli/internal/security"
)

var version = "0.3.0-dev"

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
	dryRun       bool
	jq           string
	pageAll      bool
	pageLimit    int
	pageDelay    time.Duration
	maxRecords   int
	resumePage   int
	resumeWindow int
	resumeOffset int
	rangeStart   string
	rangeEnd     string
	confirm      bool
	yes          bool
	confirmHash  string
}

type paginationFlags struct {
	pageAll      bool
	pageLimit    int
	pageDelay    int
	maxRecords   int
	resumePage   int
	resumeWindow int
	resumeOffset int
	rangeStart   string
	rangeEnd     string
}

func (p paginationFlags) apply(opts requestOptions) requestOptions {
	opts.pageAll = p.pageAll
	opts.pageLimit = p.pageLimit
	opts.pageDelay = time.Duration(p.pageDelay) * time.Millisecond
	opts.maxRecords = p.maxRecords
	opts.resumePage = p.resumePage
	opts.resumeWindow = p.resumeWindow
	opts.resumeOffset = p.resumeOffset
	opts.rangeStart = p.rangeStart
	opts.rangeEnd = p.rangeEnd
	return opts
}

func addPaginationFlags(cmd *cobra.Command, p *paginationFlags) {
	cmd.Flags().BoolVar(&p.pageAll, "page-all", false, "fetch all pages and configured date/time windows")
	cmd.Flags().IntVar(&p.pageLimit, "page-limit", 10, "max total pages to fetch with --page-all; 0 means unlimited")
	cmd.Flags().IntVar(&p.pageDelay, "page-delay", 3000, "delay in milliseconds between requests")
	cmd.Flags().IntVar(&p.maxRecords, "max-records", 0, "stop page-all after collecting this many records")
	cmd.Flags().IntVar(&p.resumePage, "resume-from-page", 1, "start page for --page-all resume")
	cmd.Flags().IntVar(&p.resumeWindow, "resume-from-window", 1, "start range window for --page-all resume")
	cmd.Flags().IntVar(&p.resumeOffset, "resume-offset", 0, "skip records already returned from the resumed page")
	cmd.Flags().StringVar(&p.rangeStart, "range-start", "", "first YYYYMMDD or YYYYMM report_date for batch retrieval")
	cmd.Flags().StringVar(&p.rangeEnd, "range-end", "", "last YYYYMMDD or YYYYMM report_date for batch retrieval")
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
	cmd.PersistentFlags().IntVar(&globals.rateLimit, "rate-limit", 0, "requests per minute; default 250 or CAPTAINBI_RATE_LIMIT")
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
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "configuration saved")
			return err
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
	cmd.AddCommand(newWriteAllowlistCmd())
	cmd.AddCommand(&cobra.Command{
		Use:   "rate-limit <requests-per-minute>",
		Short: "Persist the CaptainBI request limit for this machine",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := strconv.Atoi(args[0])
			if err != nil || value <= 0 {
				return typedH("business", "rate limit must be a positive integer", "run cbi config rate-limit 250 or set CAPTAINBI_RATE_LIMIT")
			}
			cfg, err := core.LoadConfig()
			if err != nil {
				return err
			}
			cfg.RateLimit = value
			if err := core.SaveConfig(cfg); err != nil {
				return err
			}
			return writeValue(cmd, map[string]any{"ok": true, "rate_limit_per_minute": value}, nil, "")
		},
	})
	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show non-sensitive configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			view := map[string]any{
				"client_id":             security.RedactValue(cfg.ClientID),
				"base_url":              cfg.BaseURL,
				"open_channel_id":       security.RedactValue(cfg.OpenChannelID),
				"rate_limit":            cfg.RateLimit,
				"token_expiry":          cfg.TokenExpiry,
				"channels_count":        len(cfg.Channels),
				"write_allowlist_count": len(cfg.WriteAllowlist),
				"plain_secret":          cfg.PlainSecretHint != "",
			}
			return outfmt.Write(cmd.OutOrStdout(), view, outfmt.Options{Format: globals.format, Machine: globals.machine}, nil)
		},
	}
	cmd.AddCommand(showCmd)
	return cmd
}

func newWriteAllowlistCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "write-allowlist", Short: "Manage Agent dangerous-write command policy"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List allowlisted dangerous-write command references",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			return writeValue(cmd, cfg.WriteAllowlist, nil, "")
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "add <domain.command>",
		Short: "Allow an Agent to execute this registered dangerous write after approval",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := canonicalWriteReference(args[0])
			if err != nil {
				return err
			}
			cfg, err := core.LoadConfig()
			if err != nil {
				return err
			}
			cfg.WriteAllowlist = append(cfg.WriteAllowlist, ref)
			cfg.WriteAllowlist = sortedUnique(cfg.WriteAllowlist)
			if err := core.SaveConfig(cfg); err != nil {
				return err
			}
			return writeValue(cmd, map[string]any{"ok": true, "command": ref}, nil, "")
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "remove <domain.command>",
		Short: "Remove an Agent dangerous-write command permission",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := canonicalWriteReference(args[0])
			if err != nil {
				return err
			}
			cfg, err := core.LoadConfig()
			if err != nil {
				return err
			}
			filtered := cfg.WriteAllowlist[:0]
			for _, value := range cfg.WriteAllowlist {
				if value != ref {
					filtered = append(filtered, value)
				}
			}
			cfg.WriteAllowlist = filtered
			if err := core.SaveConfig(cfg); err != nil {
				return err
			}
			return writeValue(cmd, map[string]any{"ok": true, "command": ref}, nil, "")
		},
	})
	return cmd
}

func canonicalWriteReference(input string) (string, error) {
	reg, err := registry.Load()
	if err != nil {
		return "", err
	}
	method, ok := reg.Find(input)
	if !ok || method.RiskLevel == "read" {
		return "", typedH("business", "write command reference was not found: "+input, "use cbi schema <domain.command> and allow only a registered write command")
	}
	ref, ok := reg.ReferenceFor(method)
	if !ok {
		return "", typedH("business", "could not resolve canonical write command reference", "update or reset the Registry, then retry")
	}
	return ref, nil
}

func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
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
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "logged out")
			return err
		},
	})
	return cmd
}

func newAPICmd() *cobra.Command {
	var params, data, paramsFile, dataFile, jq, contentType, confirmHash string
	var dryRun, paramsStdin, dataStdin, confirm, yes, unsafeRawWrite bool
	var pagination paginationFlags
	cmd := &cobra.Command{
		Use:   "api <METHOD> <PATH>",
		Short: "Call any CaptainBI OpenAPI endpoint",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			query, body, err := parseMaps(inputOptions{params: params, data: data, paramsFile: paramsFile, dataFile: dataFile, paramsStdin: paramsStdin, dataStdin: dataStdin}, cmd.InOrStdin())
			if err != nil {
				return err
			}
			methodName := strings.ToUpper(args[0])
			reg, regErr := registry.Load()
			m, known := registry.Method{}, false
			if regErr == nil {
				m, known = reg.Find(args[1])
				known = known && m.HTTPMethod == methodName
			}
			if !known {
				risk := "read"
				if methodName != http.MethodGet && methodName != http.MethodHead {
					risk = "write_dangerous"
					if !unsafeRawWrite && !dryRun {
						return typedH("confirmation_required", "unknown raw write requires --unsafe-raw-write and a dry-run preview", "inspect the endpoint contract, rerun with --dry-run, then explicitly allow the raw write")
					}
				}
				m = registry.Method{HTTPMethod: methodName, FullPath: args[1], CommandName: "raw-api", RiskLevel: risk, Pagination: registry.Pagination{Type: "none"}}
			}
			if contentType != "" {
				m.ContentType = contentType
			}
			if known && m.RequestBodySchema != nil {
				if m.RequestBodyRequired && body == nil {
					return typedH("business", "request body is required", "run cbi schema for the endpoint and provide --data")
				}
				if body != nil {
					if err := validateRequestBody(m.RequestBodySchema, body); err != nil {
						return err
					}
				}
			}
			req := client.Request{Method: methodName, Path: args[1], Query: query, Body: body, ContentType: m.ContentType}
			return runRequest(cmd, m, req, pagination.apply(requestOptions{dryRun: dryRun, jq: jq, confirm: confirm, yes: yes, confirmHash: confirmHash}))
		},
	}
	cmd.Flags().StringVar(&params, "params", "", "query parameters JSON; supports - for stdin")
	cmd.Flags().StringVar(&data, "data", "", "request body JSON; supports - for stdin")
	cmd.Flags().BoolVar(&paramsStdin, "params-stdin", false, "read query parameter JSON from stdin")
	cmd.Flags().BoolVar(&dataStdin, "data-stdin", false, "read request body JSON from stdin")
	cmd.Flags().StringVar(&paramsFile, "params-file", "", "read query parameter JSON from file")
	cmd.Flags().StringVar(&dataFile, "data-file", "", "read request body JSON from file")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print request without sending it")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "confirm a dangerous raw write")
	cmd.Flags().StringVar(&confirmHash, "confirm-request", "", "confirm the exact request hash produced by --dry-run")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the prompt for a known write_safe endpoint")
	cmd.Flags().BoolVar(&unsafeRawWrite, "unsafe-raw-write", false, "allow an unknown non-GET raw API write after preview")
	cmd.Flags().StringVar(&contentType, "content-type", "", "request content type override for unknown raw APIs")
	cmd.Flags().StringVar(&jq, "jq", "", "gojq expression to filter JSON output")
	addPaginationFlags(cmd, &pagination)
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
			effectiveReg, registryInfo, loadErr := registry.LoadWithInfo()
			if loadErr != nil {
				return loadErr
			}
			dir, _ := core.ConfigDir()
			rateStatus, _ := client.RateLimitStatus(cfg)
			configWritable := true
			configWriteError := ""
			if err := core.CheckConfigDirWritable(); err != nil {
				configWritable = false
				configWriteError = security.RedactString(err.Error())
			}
			return outfmt.Write(cmd.OutOrStdout(), map[string]any{
				"config_loads":              true,
				"config_dir":                dir,
				"config_dir_overridden":     os.Getenv(core.EnvConfigDir) != "",
				"config_dir_writable":       configWritable,
				"config_dir_error":          configWriteError,
				"config_dir_recommendation": "grant write access to the directory or set CAPTAINBI_CONFIG_DIR to a private writable path",
				"client_configured":         cfg.ClientID != "",
				"has_access_token":          cfg.AccessToken != "",
				"keyring_available":         core.KeyringAvailable(),
				"headless_secret_supported": os.Getenv(core.EnvClientSecret) != "" || os.Getenv(core.EnvAccessToken) != "" || cfg.PlainSecretFile != "",
				"headless_recommendation":   "use CAPTAINBI_ACCESS_TOKEN, CAPTAINBI_CLIENT_SECRET, or cbi config init --client-secret-file",
				"registry_methods":          len(effectiveReg.AllMethods()),
				"registry_services":         len(effectiveReg.Services),
				"registry_version":          effectiveReg.Version,
				"registry_embedded_version": registryInfo.EmbeddedVersion,
				"registry_overridden":       registryInfo.Overridden,
				"registry_override_path":    registryInfo.OverridePath,
				"registry_warning":          registryInfo.Warning,
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
			var params, data, paramsFile, dataFile, jq, confirmHash string
			var dryRun, confirm, yes, paramsStdin, dataStdin bool
			var pagination paginationFlags
			endpointCmd := &cobra.Command{
				Use:   m.CommandName,
				Short: m.Summary,
				RunE: func(cmd *cobra.Command, args []string) error {
					query, body, err := collectEndpointInput(cmd, m, params, data)
					if err != nil {
						return err
					}
					req := client.Request{Method: m.HTTPMethod, Path: m.FullPath, Query: query, Body: body, ContentType: m.ContentType}
					return runRequest(cmd, m, req, pagination.apply(requestOptions{dryRun: dryRun, jq: jq, confirm: confirm, yes: yes, confirmHash: confirmHash}))
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
			endpointCmd.Flags().StringVar(&confirmHash, "confirm-request", "", "confirm the exact request hash produced by --dry-run")
			endpointCmd.Flags().BoolVar(&yes, "yes", false, "skip prompt for write_safe commands")
			endpointCmd.Flags().StringVar(&jq, "jq", "", "gojq expression to filter JSON output")
			addPaginationFlags(endpointCmd, &pagination)
			svcCmd.AddCommand(endpointCmd)
		}
		root.AddCommand(svcCmd)
	}
}

func registerShortcuts(root *cobra.Command) {
	shortcut := func(use, short, domainRef, example string, configure func(*cobra.Command, map[string]*string)) *cobra.Command {
		values := map[string]*string{}
		var dryRun bool
		var jq string
		var pagination paginationFlags
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
				req := client.Request{Method: m.HTTPMethod, Path: m.FullPath, Query: map[string]string{}, ContentType: m.ContentType}
				if m.RequestBodySchema != nil && m.HTTPMethod != http.MethodGet && m.HTTPMethod != http.MethodHead {
					req.Body = map[string]any{}
				}
				for _, p := range m.Params {
					switch p.Name {
					case "page":
						setRequestParam(&req, m, p.Name, 1)
					case "rows":
						setRequestParam(&req, m, p.Name, 100)
					}
				}
				for name, ptr := range values {
					if ptr != nil && *ptr != "" {
						value := any(*ptr)
						for _, p := range m.Params {
							if p.Name == name {
								converted, err := convertParamValue(p, *ptr)
								if err != nil {
									return err
								}
								value = converted
								break
							}
						}
						setRequestParam(&req, m, name, value)
					}
				}
				if pagination.rangeStart != "" && pagination.rangeEnd != "" && requestParam(req, m, "report_date") == "" {
					setRequestParam(&req, m, "report_date", pagination.rangeStart)
				}
				for _, p := range m.Params {
					if (p.Location == "query" || p.Location == "form") && p.Required && requestParam(req, m, p.Name) == "" {
						return typedH("business", "required shortcut flag is missing for "+p.Name, "pass the required shortcut flag shown in --help")
					}
				}
				if m.RequestBodySchema != nil {
					if err := validateRequestBody(m.RequestBodySchema, requestValidationBody(req, m)); err != nil {
						return err
					}
				}
				return runRequest(cmd, m, req, pagination.apply(requestOptions{dryRun: dryRun, jq: jq}))
			},
		}
		c.Flags().BoolVar(&dryRun, "dry-run", false, "print request without sending it")
		c.Flags().StringVar(&jq, "jq", "", "gojq expression to filter JSON output")
		addPaginationFlags(c, &pagination)
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
	root.AddCommand(shortcut("+ads-campaign-report", "Get advertising campaign report", "ads.advertise-campaign-report", "  cbi --channel main +ads-campaign-report --date 20260615 --summary --machine\n  cbi --channel main +ads-campaign-report --date 20260615 --output-file ads-campaigns.json --machine", func(cmd *cobra.Command, values map[string]*string) {
		date := ""
		values["report_date"] = &date
		cmd.Flags().StringVar(&date, "date", "", "report date in YYYYMMDD format")
	}))
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
	bodyMap := map[string]any{}
	if body != nil {
		var ok bool
		bodyMap, ok = body.(map[string]any)
		if !ok {
			return nil, nil, typedH("business", "request body must be a JSON object", "pass an object with --data, --data-stdin, or --data-file")
		}
	}
	hasExplicitBody := data != "" || dataFile != "" || dataStdin
	rangeStart, _ := cmd.Flags().GetString("range-start")
	rangeEnd, _ := cmd.Flags().GetString("range-end")
	for _, p := range m.Params {
		if p.Location != "query" && p.Location != "form" {
			continue
		}
		v, _ := cmd.Flags().GetString(p.Flag)
		changed := cmd.Flags().Changed(p.Flag)
		if v == "" && !changed {
			v = defaultString(p.Default)
		}
		if p.Name == "report_date" && v == "" && rangeStart != "" && rangeEnd != "" {
			v = rangeStart
		}
		if p.Required && v == "" && bodyMap[p.Name] == nil {
			return nil, nil, typedH("business", fmt.Sprintf("required flag --%s is missing", p.Flag), "run the command with --help and pass all required flags")
		}
		if err := validateParamValue(p, v); err != nil {
			return nil, nil, err
		}
		if p.Location == "query" && v != "" {
			query[p.Name] = v
		}
		if p.Location == "form" && v != "" {
			if hasExplicitBody && changed {
				return nil, nil, typedH("business", fmt.Sprintf("cannot combine --data input with body flag --%s", p.Flag), "use either generated body flags or one --data source")
			}
			if bodyMap[p.Name] == nil || changed {
				converted, err := convertParamValue(p, v)
				if err != nil {
					return nil, nil, err
				}
				bodyMap[p.Name] = converted
			}
		}
	}
	if m.RequestBodySchema != nil {
		validationBody := requestValidationBody(client.Request{Method: m.HTTPMethod, Query: query, Body: bodyMap}, m)
		if err := validateRequestBody(m.RequestBodySchema, validationBody); err != nil {
			return nil, nil, err
		}
		if m.FullPath == "/v1/open_fba/sync_shipment" {
			ids := strings.Split(strings.TrimSpace(fmt.Sprint(bodyMap["shipment_ids"])), ",")
			if len(ids) > 5000 {
				return nil, nil, typedH("business", "shipment_ids supports at most 5000 IDs", "split the synchronization into batches of at most 5000 shipment IDs")
			}
		}
		if m.RequestBodyRequired && len(bodyMap) == 0 {
			return nil, nil, typedH("business", "request body is required", "run cbi schema for the endpoint and provide the documented body fields")
		}
		if m.HTTPMethod == http.MethodGet || m.HTTPMethod == http.MethodHead {
			return query, nil, nil
		}
		return query, bodyMap, nil
	}
	return query, body, nil
}

func requestValidationBody(req client.Request, m registry.Method) map[string]any {
	body, _ := req.Body.(map[string]any)
	if m.HTTPMethod != http.MethodGet && m.HTTPMethod != http.MethodHead {
		return body
	}
	validation := map[string]any{}
	for _, p := range m.Params {
		if value := req.Query[p.Name]; value != "" {
			converted, err := convertParamValue(p, value)
			if err == nil {
				validation[p.Name] = converted
			}
		}
	}
	return validation
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
	if m.RiskLevel != "read" && (globals.channel == "all" || len(targets) > 1) {
		return typedH("confirmation_required", "write operations do not support --channel all or multi-channel files", "write to one configured channel alias at a time and approve each payload separately")
	}
	allowlistRequired := m.RiskLevel == "write_dangerous" || m.RiskLevel == "sync_trigger"
	allowlisted := !allowlistRequired || writeAllowed(cfg, m)
	if opts.dryRun {
		views := []map[string]any{}
		for _, target := range targets {
			view := dryRunView(req, m, target.ID, target.Alias)
			if m.RiskLevel != "read" {
				view["policy"] = map[string]any{
					"allowlist_required": allowlistRequired,
					"allowlisted":        allowlisted,
					"allow_command":      writeReference(m),
				}
				record, err := approval.Issue(approvalPayload(req, m, target.ID))
				if err != nil {
					return typedH("business", "could not store write preview approval: "+err.Error(), "make CAPTAINBI_CONFIG_DIR writable, then rerun --dry-run")
				}
				view["approval"] = map[string]any{
					"required":     true,
					"request_hash": record.RequestHash,
					"expires_at":   record.ExpiresAt.Format(time.RFC3339),
					"confirm_flag": "--confirm-request",
				}
			}
			views = append(views, view)
		}
		if len(views) == 1 {
			return writeValue(cmd, views[0], nil, opts.jq)
		}
		return writeValue(cmd, views, nil, opts.jq)
	}
	if agentMode() && allowlistRequired && !allowlisted {
		return internalerrs.New("confirmation_required", internalerrs.WriteNotAllowlisted, "Agent write command is not allowlisted: "+writeReference(m), internalerrs.Hint(internalerrs.WriteNotAllowlisted))
	}
	if err := enforceRisk(cmd, m, req, targets[0], opts); err != nil {
		return err
	}
	c := client.New(cfg, func(ctx context.Context, force bool) (string, error) { return auth.GetToken(ctx, cfg, force) })
	if len(targets) > 1 {
		results := []map[string]any{}
		for _, target := range targets {
			req.OpenChannelID = target.ID
			resp, err := executeRequest(cmd.Context(), c, m, req, opts)
			results = append(results, channelResult(target, resp, err))
			writeAudit(m, target, err, opts.confirmHash)
		}
		return writeValue(cmd, map[string]any{"ok": true, "channels": results}, nil, opts.jq)
	}
	req.OpenChannelID = targets[0].ID
	resp, err := executeRequest(cmd.Context(), c, m, req, opts)
	writeAudit(m, targets[0], err, opts.confirmHash)
	if err != nil {
		return err
	}
	if globals.verbose || globals.debug {
		if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "request method=%s path=%s channel=%s rate_limit_wait_ms=%d\n", req.Method, req.Path, security.RedactValue(req.OpenChannelID), c.LastRateLimitWait().Milliseconds()); err != nil {
			return err
		}
	}
	if wait := c.LastRateLimitWait(); wait > 0 {
		resp["rate_limit_wait_ms"] = wait.Milliseconds()
	}
	return writeValue(cmd, resp, m.TableColumns, opts.jq)
}

func writeAllowed(cfg *core.Config, m registry.Method) bool {
	ref := writeReference(m)
	for _, allowed := range cfg.WriteAllowlist {
		if allowed == ref || allowed == m.FullPath {
			return true
		}
	}
	return false
}

func writeReference(m registry.Method) string {
	if reg, err := registry.Load(); err == nil {
		if ref, ok := reg.ReferenceFor(m); ok {
			return ref
		}
	}
	return m.FullPath
}

func dryRunView(req client.Request, m registry.Method, openChannelID, alias string) map[string]any {
	return map[string]any{
		"dry_run":      true,
		"method":       req.Method,
		"path":         req.Path,
		"query":        req.Query,
		"body":         req.Body,
		"content_type": req.ContentType,
		"risk_level":   m.RiskLevel,
		"channel":      alias,
		"headers":      map[string]any{"authorization": security.RedactValue("bearer token"), "OpenChannelId": security.RedactValue(openChannelID)},
	}
}

func setRequestParam(req *client.Request, m registry.Method, name string, value any) {
	for _, p := range m.Params {
		if p.Name != name {
			continue
		}
		if p.Location == "form" {
			body, _ := req.Body.(map[string]any)
			if body == nil {
				body = map[string]any{}
				req.Body = body
			}
			body[name] = value
			return
		}
		break
	}
	if req.Query == nil {
		req.Query = map[string]string{}
	}
	req.Query[name] = fmt.Sprint(value)
}

func requestParam(req client.Request, m registry.Method, name string) string {
	for _, p := range m.Params {
		if p.Name == name && p.Location == "form" {
			body, _ := req.Body.(map[string]any)
			if body == nil || body[name] == nil {
				return ""
			}
			return fmt.Sprint(body[name])
		}
	}
	return req.Query[name]
}

func validateParamValue(p registry.Param, value string) error {
	if value == "" {
		return nil
	}
	if p.Format == "int" || p.Format == "unix_seconds" || p.Type == "integer" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return typedH("business", fmt.Sprintf("flag --%s must be an integer", p.Flag), "pass a numeric value for this flag")
		}
		if p.Min > 0 && n < p.Min {
			return typedH("business", fmt.Sprintf("flag --%s must be >= %d", p.Flag, p.Min), fmt.Sprintf("use --%s %d or a larger value", p.Flag, p.Min))
		}
		if p.Max > 0 && n > p.Max {
			return typedH("business", fmt.Sprintf("flag --%s must be <= %d", p.Flag, p.Max), fmt.Sprintf("use --%s %d or a smaller value", p.Flag, p.Max))
		}
	}
	if p.Format == "number" {
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return typedH("business", fmt.Sprintf("flag --%s must be a number", p.Flag), "pass a numeric value for this flag")
		}
	}
	if p.Format == "boolean" {
		if _, err := strconv.ParseBool(value); err != nil {
			return typedH("business", fmt.Sprintf("flag --%s must be true or false", p.Flag), "pass true or false for this flag")
		}
	}
	if p.Format == "date" && !validDateValue(value) {
		return typedH("business", fmt.Sprintf("flag --%s must be YYYYMM, YYYYMMDD, or YYYY-MM-DD", p.Flag), "pass the date format shown in cbi schema")
	}
	if len(p.Enum) > 0 {
		matched := false
		for _, allowed := range p.Enum {
			if fmt.Sprint(allowed) == value {
				matched = true
				break
			}
		}
		if !matched {
			return typedH("business", fmt.Sprintf("flag --%s must be one of %v", p.Flag, p.Enum), "choose a documented enum value from cbi schema")
		}
	}
	return nil
}

func convertParamValue(p registry.Param, value string) (any, error) {
	if err := validateParamValue(p, value); err != nil {
		return nil, err
	}
	switch p.Type {
	case "integer":
		return strconv.Atoi(value)
	case "number":
		return strconv.ParseFloat(value, 64)
	case "boolean":
		return strconv.ParseBool(value)
	default:
		return value, nil
	}
}

func validDateValue(value string) bool {
	if len(value) == 6 || len(value) == 8 {
		_, err := strconv.Atoi(value)
		return err == nil
	}
	if len(value) == 10 {
		_, err := time.Parse("2006-01-02", value)
		return err == nil
	}
	return false
}

func validateRequestBody(schemaValue, bodyValue any) error {
	schema, ok := schemaValue.(map[string]any)
	if !ok || len(schema) == 0 {
		return nil
	}
	return validateSchemaValue("body", schema, bodyValue)
}

func validateSchemaValue(path string, schema map[string]any, value any) error {
	if value == nil {
		return nil
	}
	typ := fmt.Sprint(schema["type"])
	switch typ {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return typedH("business", path+" must be an object", "inspect the request schema with cbi schema")
		}
		for _, required := range anyStrings(schema["required"]) {
			if object[required] == nil || fmt.Sprint(object[required]) == "" {
				return typedH("business", path+"."+required+" is required", "provide every required request body field shown by cbi schema")
			}
		}
		properties, _ := schema["properties"].(map[string]any)
		for name, item := range object {
			child, _ := properties[name].(map[string]any)
			if len(child) > 0 {
				if err := validateSchemaValue(path+"."+name, child, item); err != nil {
					return err
				}
			}
		}
	case "array":
		array, ok := value.([]any)
		if !ok {
			return typedH("business", path+" must be an array", "inspect the request schema with cbi schema")
		}
		if minItems := intFrom(schema["minItems"]); minItems > 0 && len(array) < minItems {
			return typedH("business", fmt.Sprintf("%s must contain at least %d item(s)", path, minItems), "provide at least one complete item according to cbi schema")
		}
		items, _ := schema["items"].(map[string]any)
		for index, item := range array {
			if err := validateSchemaValue(fmt.Sprintf("%s[%d]", path, index), items, item); err != nil {
				return err
			}
		}
	case "integer":
		switch number := value.(type) {
		case float64:
			if number != float64(int64(number)) {
				return typedH("business", path+" must be an integer", "fix the request body type according to cbi schema")
			}
		case int, int64:
		default:
			return typedH("business", path+" must be an integer", "fix the request body type according to cbi schema")
		}
	case "number":
		switch value.(type) {
		case float64, int:
		default:
			return typedH("business", path+" must be a number", "fix the request body type according to cbi schema")
		}
	case "string":
		if _, ok := value.(string); !ok {
			return typedH("business", path+" must be a string", "fix the request body type according to cbi schema")
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return typedH("business", path+" must be a boolean", "fix the request body type according to cbi schema")
		}
	}
	if values, ok := schema["enum"].([]any); ok && len(values) > 0 {
		matched := false
		for _, allowed := range values {
			if fmt.Sprint(allowed) == fmt.Sprint(value) {
				matched = true
				break
			}
		}
		if !matched {
			return typedH("business", path+" has an unsupported value", "choose an enum value shown by cbi schema")
		}
	}
	return nil
}

func anyStrings(value any) []string {
	values, _ := value.([]any)
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			out = append(out, text)
		}
	}
	if stringsValue, ok := value.([]string); ok {
		return stringsValue
	}
	return out
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

func enforceRisk(cmd *cobra.Command, m registry.Method, req client.Request, target channelTarget, opts requestOptions) error {
	if opts.dryRun || m.RiskLevel == "read" {
		return nil
	}
	if opts.confirmHash != "" {
		if err := approval.Verify(approvalPayload(req, m, target.ID), opts.confirmHash); err != nil {
			return typedH("confirmation_required", err.Error(), "rerun --dry-run, ask the user to approve the exact preview, then pass its current --confirm-request hash")
		}
		if err := approval.Consume(opts.confirmHash); err != nil {
			return typedH("confirmation_required", "confirm-request could not be consumed: "+err.Error(), "rerun --dry-run and do not send the write until the approval record can be consumed")
		}
		return nil
	}
	if agentMode() {
		return typedH("confirmation_required", "Agent writes require --confirm-request from a current dry-run preview", "run --dry-run, show the preview to the user, then use the approved request hash without changing the payload")
	}
	if m.RiskLevel == "write_safe" {
		if opts.yes {
			return nil
		}
		if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s is a write operation; continue? [y/N] ", m.CommandName); err != nil {
			return err
		}
		line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if strings.TrimSpace(strings.ToLower(line)) != "y" {
			return typedH("business", "operation cancelled", "rerun with --yes after confirming the write operation")
		}
		return nil
	}
	if !opts.confirm {
		return typedH("confirmation_required", "this operation requires --confirm; use --dry-run to preview", "add --confirm only after the user explicitly approves this write or sync operation")
	}
	if m.RiskLevel == "sync_trigger" {
		if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "warning: this operation may trigger external CaptainBI/Amazon synchronization"); err != nil {
			return err
		}
	}
	return nil
}

func approvalPayload(req client.Request, m registry.Method, channelID string) approval.Payload {
	registryVersion := "unknown"
	if current, err := registry.Load(); err == nil && current.Version != "" {
		registryVersion = current.Version
	}
	return approval.Payload{
		Method:          req.Method,
		Path:            req.Path,
		Query:           req.Query,
		Body:            req.Body,
		ContentType:     req.ContentType,
		ChannelID:       channelID,
		RiskLevel:       m.RiskLevel,
		RegistryVersion: registryVersion,
	}
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
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&m); err != nil {
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
		b, err := readSafeInputFile(strings.TrimPrefix(v, "@"))
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

func typed(kind, msg string) error {
	return internalerrs.New(kind, "", msg, hintFor(kind, msg))
}

func typedH(kind, msg, hint string) error {
	return internalerrs.New(kind, "", msg, hint)
}
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
	var te *internalerrs.Error
	if errors.As(err, &te) {
		switch te.Kind() {
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
		subtype := errorSubtype(err)
		if subtype == internalerrs.ChannelInvalid {
			return 1
		}
		if subtype == internalerrs.RateLimitExceeded {
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
		var te *internalerrs.Error
		if errors.As(err, &te) {
			kind = te.Kind()
		}
		var ce *client.Error
		if errors.As(err, &ce) {
			kind = ce.Kind
		}
		var se *client.StatusError
		if errors.As(err, &se) {
			subtype := errorSubtype(err)
			if subtype == internalerrs.ChannelInvalid {
				kind = "business"
			} else if subtype == internalerrs.RateLimitExceeded {
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
	_, _ = fmt.Fprintln(w, security.RedactString(err.Error()))
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
