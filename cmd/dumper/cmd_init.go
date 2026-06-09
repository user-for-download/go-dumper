package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/user-for-download/go-dumper/internal/config"
)

func newInitCmd() *cobra.Command {
	var (
		out   string
		force bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a default config file (prc.json)",
		Long: `Creates a prc.json config file with sensible defaults.
Tree and progress display are enabled by default.
Edit the file to customise include/exclude patterns, chunk size, and more.

Tree mode
  "full"    – the tree shows every file in the project (default).
  "include" – the tree shows only the files that were actually dumped,
              i.e. those matching your include/exclude patterns.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if _, err := os.Stat(out); err == nil && !force {
				return fmt.Errorf(
					"config file %q already exists; use --force to overwrite", out,
				)
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("stat %q: %w", out, err)
			}

			data, err := json.MarshalIndent(config.InitDefaults(), "", "  ")
			if err != nil {
				return fmt.Errorf("marshal config: %w", err)
			}
			data = append(data, '\n')

			if err := os.WriteFile(out, data, 0o644); err != nil {
				return fmt.Errorf("write %q: %w", out, err)
			}

			fmt.Printf("created %s\n", out)
			return nil
		},
	}

	cmd.Flags().StringVarP(&out, "output", "o", "prc.json", "path for the generated config file")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing config file")
	return cmd
}
