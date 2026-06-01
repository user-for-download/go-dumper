package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type initConfig struct {
	Path           string   `json:"path"`
	Output         string   `json:"output"`
	Include        []string `json:"include"`
	Exclude        []string `json:"exclude"`
	Type           []string `json:"type"`
	MaxSymbols     int      `json:"max_symbols"`
	ChunkPrefix    string   `json:"chunk_prefix"`
	SplitLongLines bool     `json:"split_long_lines"`
	Progress       bool     `json:"progress"`
	StatsFile      string   `json:"stats_file"`
	IncludeHidden  bool     `json:"include_hidden"`
	Concurrency    int      `json:"concurrency"`
	ExcludeSelf    bool     `json:"exclude_self"`
	Format         string   `json:"format"`
	Clear          clearCfg `json:"clear"`
	Tree           treeCfg  `json:"tree"`
}

type clearCfg struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"`
}

type treeCfg struct {
	Enabled      bool   `json:"enabled"`
	Format       string `json:"format"`
	MaxDepth     int    `json:"max_depth"`
	IncludeSizes bool   `json:"include_sizes"`
	Mode         string `json:"mode"`
}

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

			cfg := initConfig{
				Path:           ".",
				Output:         "./dump_out",
				Include:        []string{"**/*"},
				Exclude:        []string{},
				Type:           []string{},
				MaxSymbols:     1_000_000,
				ChunkPrefix:    "dump",
				SplitLongLines: false,
				Progress:       true,
				StatsFile:      "",
				IncludeHidden:  false,
				Concurrency:    1,
				ExcludeSelf:    true,
				Format:         "plain",
				Clear: clearCfg{
					Enabled: false,
					Mode:    "line",
				},
				Tree: treeCfg{
					Enabled:      true,
					Format:       "ascii",
					MaxDepth:     0,
					IncludeSizes: true,
					Mode:         "full",
				},
			}

			data, err := json.MarshalIndent(cfg, "", "  ")
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
