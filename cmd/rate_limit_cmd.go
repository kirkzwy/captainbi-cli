package cmd

import (
	"github.com/spf13/cobra"

	"github.com/kirkzwy/captainbi-cli/internal/client"
	outfmt "github.com/kirkzwy/captainbi-cli/internal/output"
)

func newRateLimitCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "rate-limit", Short: "Inspect local rate limit state"}
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show local rate limit wait state",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			status, err := client.RateLimitStatus(cfg)
			if err != nil {
				return err
			}
			status["ok"] = true
			return writeControlValue(cmd, status, outfmt.Options{Format: globals.format, Machine: globals.machine}, nil)
		},
	})
	return cmd
}
