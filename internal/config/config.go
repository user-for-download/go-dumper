package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type TreeConfig struct {
	Enabled      bool   `json:"enabled"`
	Format       string `json:"format"`
	MaxDepth     int    `json:"max_depth"`
	IncludeSizes bool   `json:"include_sizes"`
	Mode         string `json:"mode"`
}

type Config struct {
	Path           string     `json:"path"`
	Output         string     `json:"output"`
	Include        []string   `json:"include"`
	Exclude        []string   `json:"exclude"`
	Type           []string   `json:"type"`
	Clean          bool       `json:"clean"`
	MaxSymbols     int        `json:"max_symbols"`
	ChunkPrefix    string     `json:"chunk_prefix"`
	SplitLongLines bool       `json:"split_long_lines"`
	Progress       bool       `json:"progress"`
	StatsFile      string     `json:"stats_file"`
	IncludeHidden  bool       `json:"include_hidden"`
	Concurrency    int        `json:"concurrency"`
	ExcludeSelf    bool       `json:"exclude_self"`
	Format         string     `json:"format"`
	Tree           TreeConfig `json:"tree"`
}

func defaults() *Config {
	return &Config{
		Path:    ".",
		Output:  "./dump_out",
		Include: []string{"**/*"},
		Exclude: []string{
			".git/**", "**/.git/**",
			"node_modules/**", "**/node_modules/**",
			"vendor/**", "**/vendor/**",
			"dist/**", "**/dist/**",
			"build/**", "**/build/**",
			"target/**", "**/target/**",
			"out/**", "**/out/**",
			"coverage/**", "**/coverage/**",
			".next/**", "**/.next/**",
			".cache/**", "**/.cache/**",
		},
		MaxSymbols:  1_000_000,
		ChunkPrefix: "dump",
		Tree:        TreeConfig{Enabled: false, Format: "ascii", MaxDepth: 0, IncludeSizes: true, Mode: "full"},
		Concurrency: 1,
		ExcludeSelf: true,
		Format:      "plain",
	}
}

// InitDefaults returns defaults suitable for dumper init — same as defaults()
// but with progress and tree enabled (friendlier for interactive use).
func InitDefaults() *Config {
	cfg := defaults()
	cfg.Progress = true
	cfg.Tree.Enabled = true
	return cfg
}

func Load(path string, explicit bool) (*Config, error) {
	cfg := defaults()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if explicit {
			return nil, fmt.Errorf("config file %q not found", path)
		}
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config %q (check for JSON syntax errors like trailing commas or incorrect types): %w", path, err)
	}
	cfg.normalize()
	return cfg, nil
}

func (c *Config) normalize() {
	if len(c.Include) == 0 {
		c.Include = []string{"**/*"}
	}
	if c.ChunkPrefix == "" {
		c.ChunkPrefix = "dump"
	}
	if c.Tree.Format == "" {
		c.Tree.Format = "ascii"
	}
	if c.Tree.Mode == "" {
		c.Tree.Mode = "full"
	}
	if c.Format == "" {
		c.Format = "plain"
	}
	if c.Format == "md" {
		c.Format = "markdown"
	}
}

func samePath(a, b string) bool {
	absA, errA := canonicalPath(a)
	absB, errB := canonicalPath(b)
	if errA != nil || errB != nil {
		return false
	}
	return filepath.Clean(absA) == filepath.Clean(absB)
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := filepath.Clean(abs)
	var suffix []string
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(abs), nil
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func pathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != "." && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (c *Config) Validate() error {
	if c.Path == "" {
		return errors.New("path must not be empty")
	}
	if c.Output == "" {
		return errors.New("output must not be empty")
	}
	absPath, pathErr := canonicalPath(c.Path)
	absOutput, outputErr := canonicalPath(c.Output)
	if pathErr != nil || outputErr != nil {
		return errors.New("path and output must be valid filesystem paths")
	}
	if samePath(c.Path, c.Output) {
		return errors.New("output directory must not be the same as path")
	}
	if pathContains(absOutput, absPath) {
		return errors.New("output directory must not contain path")
	}
	if filepath.Clean(absOutput) == string(filepath.Separator) {
		return errors.New("output directory must not be the filesystem root")
	}
	if c.MaxSymbols <= 0 {
		return errors.New("max_symbols must be > 0")
	}
	if c.Concurrency < 1 {
		return errors.New("concurrency must be >= 1")
	}
	switch c.Format {
	case "plain", "markdown":
	default:
		return errors.New("format must be one of: plain, markdown")
	}
	switch c.Tree.Mode {
	case "", "full", "include":
	default:
		return errors.New("tree.mode must be one of: full, include")
	}
	if c.Tree.MaxDepth < 0 {
		return errors.New("tree.max_depth must be >= 0")
	}
	switch c.Tree.Format {
	case "", "ascii":
	default:
		return errors.New("tree.format must be ascii")
	}
	return nil
}

type CLIOverrides struct {
	Path           *string
	Output         *string
	Include        []string
	Exclude        []string
	IncludeSet     bool
	ExcludeSet     bool
	Type           []string
	TypeSet        bool
	Clean          *bool
	MaxSymbols     *int
	ChunkPrefix    *string
	SplitLongLines *bool
	Progress       *bool
	StatsFile      *string
	TreeEnabled    *bool
	TreeDepth      *int
	TreeMode       *string
	Concurrency    *int
	ExcludeSelf    *bool
	Format         *string
	IncludeHidden  *bool
}

func (c *Config) MergeCLI(o CLIOverrides) error {
	if o.Path != nil {
		c.Path = *o.Path
	}
	if o.Output != nil {
		c.Output = *o.Output
	}
	if o.IncludeSet {
		c.Include = o.Include
	}
	if o.ExcludeSet {
		c.Exclude = o.Exclude
	}
	if o.TypeSet {
		c.Type = o.Type
	}
	if o.Clean != nil {
		c.Clean = *o.Clean
	}
	if o.MaxSymbols != nil {
		c.MaxSymbols = *o.MaxSymbols
	}
	if o.ChunkPrefix != nil {
		c.ChunkPrefix = *o.ChunkPrefix
	}
	if o.SplitLongLines != nil {
		c.SplitLongLines = *o.SplitLongLines
	}
	if o.Progress != nil {
		c.Progress = *o.Progress
	}
	if o.StatsFile != nil {
		c.StatsFile = *o.StatsFile
	}
	if o.TreeEnabled != nil {
		c.Tree.Enabled = *o.TreeEnabled
	}
	if o.TreeDepth != nil {
		c.Tree.MaxDepth = *o.TreeDepth
	}
	if o.TreeMode != nil {
		c.Tree.Mode = *o.TreeMode
	}
	if o.IncludeHidden != nil {
		c.IncludeHidden = *o.IncludeHidden
	}
	if o.Concurrency != nil {
		c.Concurrency = *o.Concurrency
	}
	if o.ExcludeSelf != nil {
		c.ExcludeSelf = *o.ExcludeSelf
	}
	if o.Format != nil {
		c.Format = *o.Format
	}
	c.normalize()
	return nil
}
