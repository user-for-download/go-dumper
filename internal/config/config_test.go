package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidate_ValidDefault(t *testing.T) {
	cfg := defaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
}

func TestValidate_InvalidFormat(t *testing.T) {
	cfg := defaults()
	cfg.Format = "bad"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid format error")
	}
}

func TestValidate_InvalidConcurrency(t *testing.T) {
	cfg := defaults()
	cfg.Concurrency = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid concurrency error")
	}
}

func TestValidate_InvalidMaxSymbols(t *testing.T) {
	cfg := defaults()
	cfg.MaxSymbols = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid max_symbols error")
	}
}

func TestValidate_InvalidTreeMode(t *testing.T) {
	cfg := defaults()
	cfg.Tree.Mode = "bad"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid tree.mode error")
	}
}

func TestValidate_InvalidTreeMaxDepth(t *testing.T) {
	cfg := defaults()
	cfg.Tree.MaxDepth = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid tree.max_depth error")
	}
}

func TestValidate_ValidTreeModes(t *testing.T) {
	for _, mode := range []string{"", "full", "include"} {
		cfg := defaults()
		cfg.Tree.Mode = mode
		if err := cfg.Validate(); err != nil {
			t.Errorf("tree.mode=%q should be valid: %v", mode, err)
		}
	}
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load("/nonexistent/config.json", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
	if cfg.Format != "plain" {
		t.Errorf("default format should be plain, got %q", cfg.Format)
	}
	if cfg.Tree.Mode != "full" {
		t.Errorf("default tree.mode should be full, got %q", cfg.Tree.Mode)
	}
}

func TestNormalize_MdToMarkdown(t *testing.T) {
	cfg := defaults()
	cfg.Format = "md"
	cfg.normalize()
	if cfg.Format != "markdown" {
		t.Errorf("md should normalize to markdown, got %q", cfg.Format)
	}
}

func TestNormalize_EmptyTreeModeToFull(t *testing.T) {
	cfg := defaults()
	cfg.Tree.Mode = ""
	cfg.normalize()
	if cfg.Tree.Mode != "full" {
		t.Errorf("empty tree.mode should normalize to full, got %q", cfg.Tree.Mode)
	}
}

func TestMergeCLI_AppliesIncludeHidden(t *testing.T) {
	cfg := defaults()
	hide := true
	_ = cfg.MergeCLI(CLIOverrides{IncludeHidden: &hide})
	if !cfg.IncludeHidden {
		t.Error("IncludeHidden should be true after merge")
	}
}

func TestValidate_EmptyPath(t *testing.T) {
	cfg := defaults()
	cfg.Path = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected empty path error")
	}
}

func TestValidate_EmptyOutput(t *testing.T) {
	cfg := defaults()
	cfg.Output = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected empty output error")
	}
}

func TestValidate_ValidFormats(t *testing.T) {
	for _, f := range []string{"plain", "markdown"} {
		cfg := defaults()
		cfg.Format = f
		if err := cfg.Validate(); err != nil {
			t.Errorf("format=%q should be valid: %v", f, err)
		}
	}
}

func TestValidate_NegativeMaxSymbols(t *testing.T) {
	cfg := defaults()
	cfg.MaxSymbols = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected negative max_symbols error")
	}
}

func TestDefaults(t *testing.T) {
	cfg := defaults()
	if cfg.Path != "." {
		t.Errorf("default path should be ., got %q", cfg.Path)
	}
	if cfg.Output != "./dump_out" {
		t.Errorf("default output should be ./dump_out, got %q", cfg.Output)
	}
	if cfg.MaxSymbols != 1_000_000 {
		t.Errorf("default max_symbols should be 1000000, got %d", cfg.MaxSymbols)
	}
	if cfg.Concurrency != 1 {
		t.Errorf("default concurrency should be 1, got %d", cfg.Concurrency)
	}
	if !cfg.ExcludeSelf {
		t.Error("default exclude_self should be true")
	}
	if cfg.Tree.Enabled {
		t.Error("default tree.enabled should be false")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	cfg, err := Load("/tmp/nonexistent_path_12345.json", false)
	if err != nil {
		t.Fatalf("Load should not error on missing file: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("defaults should be valid: %v", err)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	tmp, err := os.CreateTemp("", "bad_config_*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.WriteString("invalid json"); err != nil {
		t.Fatal(err)
	}
	_ = tmp.Close()

	_, err = Load(tmp.Name(), false)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoad_ExplicitMissingFile(t *testing.T) {
	_, err := Load("/tmp/nonexistent_path_12345.json", true)
	if err == nil {
		t.Fatal("expected error for explicit missing file")
	}
}

func TestValidate_RejectsOutputAncestor(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := defaults()
	cfg.Path = root
	cfg.Output = parent
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected output ancestor to be rejected")
	}
}

func TestValidate_AllowsOutputDescendant(t *testing.T) {
	root := t.TempDir()
	cfg := defaults()
	cfg.Path = root
	cfg.Output = filepath.Join(root, "dump_out")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("output descendant should be valid: %v", err)
	}
}
