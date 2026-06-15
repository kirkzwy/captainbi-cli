package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/spf13/cobra"

	outfmt "github.com/kirkzwy/captainbi-cli/internal/output"
	"github.com/kirkzwy/captainbi-cli/internal/registry"
)

const openAPISource = "https://doc.captainbi.com/openapi.json"

func newRegistryCmd(reg *registry.Registry, regErr error) *cobra.Command {
	cmd := &cobra.Command{Use: "registry", Short: "Inspect embedded endpoint registry"}
	cmd.AddCommand(&cobra.Command{
		Use:   "check",
		Short: "Compare embedded registry with CaptainBI OpenAPI path count",
		RunE: func(cmd *cobra.Command, args []string) error {
			if regErr != nil {
				return regErr
			}
			count, err := fetchOpenAPIPathCount(cmd.Context())
			if err != nil {
				return err
			}
			methods := len(reg.AllMethods())
			return outfmt.Write(cmd.OutOrStdout(), map[string]any{
				"ok":                 count == methods,
				"source":             openAPISource,
				"openapi_paths":      count,
				"registry_methods":   methods,
				"registry_version":   reg.Version,
				"registry_is_stale":  count != methods,
				"update_command":     "go run ./tools/gen-registry",
				"embedded_registry":  true,
				"runtime_mutability": false,
			}, outfmt.Options{Format: globals.format, Machine: globals.machine}, nil)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "update",
		Short: "Show the development command for updating embedded registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			return outfmt.Write(cmd.OutOrStdout(), map[string]any{
				"ok":             false,
				"reason":         "registry is embedded in the binary",
				"update_command": "go run ./tools/gen-registry",
			}, outfmt.Options{Format: globals.format, Machine: globals.machine}, nil)
		},
	})
	return cmd
}

func fetchOpenAPIPathCount(ctx context.Context) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openAPISource, nil)
	if err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	var doc struct {
		Paths map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return 0, err
	}
	return len(doc.Paths), nil
}
