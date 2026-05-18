package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type ClearConfig struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"`
}

type TreeConfig struct {
	Enabled      bool   `json:"enabled"`
	Format       string `json:"format"`
	MaxDepth     int    `json:"max_depth"`
	IncludeSizes bool   `json:"include_sizes"`
	Mode         string `json:"mode"`
}

type Config struct {
	Path           string      `json:"path"`
	Output         string      `json:"output"`
	Include        []string    `json:"include"`
	Exclude        []string    `json:"exclude"`
	MaxSymbols     int         `json:"max_symbols"`
	ChunkPrefix    string      `json:"chunk_prefix"`
	SplitLongLines bool        `json:"split_long_lines"`
	Progress       bool        `json:"progress"`
	StatsFile      string      `json:"stats_file"`
	IncludeHidden  bool        `json:"include_hidden"`
	Concurrency    int         `json:"concurrency"`
	ExcludeSelf    bool        `json:"exclude_self"`
	Format         string      `json:"format"`
	Clear          ClearConfig `json:"clear"`
	Tree           TreeConfig  `json:"tree"`
}

func defaults() *Config {
	return &Config{
		Path:        ".",
		Output:      "./dump_out",
		Include:     []string{"**/*"},
		MaxSymbols:  1_000_000,
		ChunkPrefix: "dump",
		Clear:       ClearConfig{Enabled: false, Mode: "line"},
		Tree:        TreeConfig{Enabled: false, Format: "ascii", MaxDepth: 0, IncludeSizes: true, Mode: "full"},
		Concurrency: 1,
		ExcludeSelf: true,
		Format:      "plain",
	}
}

func Load(path string) (*Config, error) {
	cfg := defaults()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
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
	if c.Clear.Mode == "" {
		c.Clear.Mode = "line"
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
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return false
	}
	return filepath.Clean(absA) == filepath.Clean(absB)
}

func (c *Config) Validate() error {
	if c.Path == "" {
		return errors.New("path must not be empty")
	}
	if c.Output == "" {
		return errors.New("output must not be empty")
	}
	if samePath(c.Path, c.Output) {
		return errors.New("output directory must not be the same as path")
	}
	if c.MaxSymbols <= 0 {
		return errors.New("max_symbols must be > 0")
	}
	if c.Concurrency < 1 {
		return errors.New("concurrency must be >= 1")
	}
	switch c.Format {
	case "plain", "markdown", "xml":
	default:
		return errors.New("format must be one of: plain, markdown, xml")
	}
	switch c.Clear.Mode {
	case "", "line", "line_and_block":
	default:
		return errors.New("clear.mode must be one of: line, line_and_block")
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
	MaxSymbols     *int
	ChunkPrefix    *string
	SplitLongLines *bool
	Progress       *bool
	StatsFile      *string
	ClearEnabled   *bool
	ClearMode      *string
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
	if o.ClearEnabled != nil {
		c.Clear.Enabled = *o.ClearEnabled
	}
	if o.ClearMode != nil {
		c.Clear.Mode = *o.ClearMode
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
