package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	outfmt "github.com/kirkzwy/captainbi-cli/internal/output"
	"github.com/kirkzwy/captainbi-cli/internal/registry"
)

const openAPISource = "https://doc.captainbi.com/openapi.json"

var (
	registryMetadataSource = "https://raw.githubusercontent.com/kirkzwy/captainbi-cli/main/internal/registry/captainbi_meta.json"
	registryHTTPClient     = &http.Client{Timeout: 30 * time.Second}
)

func newRegistryCmd(reg *registry.Registry, regErr error) *cobra.Command {
	cmd := &cobra.Command{Use: "registry", Short: "Inspect and update endpoint registry metadata"}
	cmd.AddCommand(&cobra.Command{
		Use:   "check",
		Short: "Compare the effective registry with CaptainBI OpenAPI",
		RunE: func(cmd *cobra.Command, args []string) error {
			if regErr != nil {
				return regErr
			}
			effective, info, err := registry.LoadWithInfo()
			if err != nil {
				return err
			}
			count, err := fetchOpenAPIMethodCount(cmd.Context())
			if err != nil {
				return err
			}
			methods := len(effective.AllMethods())
			return writeControlValue(cmd, map[string]any{
				"ok":                     count == methods,
				"source":                 openAPISource,
				"openapi_methods":        count,
				"registry_methods":       methods,
				"registry_version":       effective.Version,
				"registry_is_stale":      count != methods,
				"registry_overridden":    info.Overridden,
				"registry_override_path": info.OverridePath,
				"registry_warning":       info.Warning,
				"update_command":         "cbi registry update",
				"embedded_registry":      !info.Overridden,
				"runtime_mutability":     true,
			}, outfmt.Options{Format: globals.format, Machine: globals.machine}, nil)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "update",
		Short: "Install the latest compatible generated registry metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := fetchBytes(cmd.Context(), registryMetadataSource, 32<<20)
			if err != nil {
				return typedH("network", "registry update failed: "+err.Error(), "check network or proxy settings, then rerun cbi registry update")
			}
			info, err := registry.InstallOverride(cmd.Context(), data)
			if err != nil {
				return typedH("business", "registry update was rejected: "+err.Error(), "keep the embedded registry and update the CLI if the metadata is incompatible")
			}
			return writeControlValue(cmd, map[string]any{
				"ok":                     true,
				"source":                 registryMetadataSource,
				"registry_version":       info.EffectiveVersion,
				"embedded_version":       info.EmbeddedVersion,
				"registry_override_path": info.OverridePath,
				"restart_required":       true,
				"next_command":           "cbi registry check --machine",
			}, outfmt.Options{Format: globals.format, Machine: globals.machine}, nil)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "reset",
		Short: "Remove the managed override and restore the embedded registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := registry.RemoveOverride(cmd.Context())
			if err != nil {
				return err
			}
			return writeControlValue(cmd, map[string]any{
				"ok":               true,
				"removed_path":     path,
				"registry_version": reg.Version,
			}, outfmt.Options{Format: globals.format, Machine: globals.machine}, nil)
		},
	})
	return cmd
}

func fetchOpenAPIMethodCount(ctx context.Context) (int, error) {
	b, err := fetchBytes(ctx, openAPISource, 32<<20)
	if err != nil {
		return 0, err
	}
	return countOpenAPIMethods(b)
}

func countOpenAPIMethods(b []byte) (int, error) {
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return 0, err
	}
	count := 0
	for _, operations := range doc.Paths {
		for method := range operations {
			switch strings.ToUpper(method) {
			case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
				count++
			}
		}
	}
	return count, nil
}

func fetchBytes(ctx context.Context, source string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, err
	}
	resp, err := registryHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s returned HTTP %d", source, resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, maxBytes+1)
	b, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > maxBytes {
		return nil, fmt.Errorf("response exceeds %d bytes", maxBytes)
	}
	return b, nil
}
