package tree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "cmd", "dumper"), 0o755))
	must(os.MkdirAll(filepath.Join(root, "internal"), 0o755))
	must(os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644))
	must(os.WriteFile(filepath.Join(root, "cmd", "dumper", "main.go"), []byte("package main\n"), 0o644))
	must(os.WriteFile(filepath.Join(root, "internal", "lib.go"), []byte("package internal\n"), 0o644))
	must(os.WriteFile(filepath.Join(root, ".hidden"), []byte("x"), 0o644))
	return root
}

func TestGenerate_Basic(t *testing.T) {
	root := setupTree(t)
	out, err := Generate(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"cmd/", "dumper/", "main.go", "internal/", "lib.go", "go.mod"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected tree to contain %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, ".hidden") {
		t.Errorf("hidden file should be skipped by default")
	}
}

func TestGenerate_IncludeHidden(t *testing.T) {
	root := setupTree(t)
	out, err := Generate(Options{Root: root, IncludeHidden: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, ".hidden") {
		t.Errorf("expected hidden files to be included")
	}
}

func TestGenerate_MaxDepth(t *testing.T) {
	root := setupTree(t)
	out, err := Generate(Options{Root: root, MaxDepth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "main.go") {
		t.Errorf("with MaxDepth=1 nested files must not appear:\n%s", out)
	}
	if !strings.Contains(out, "cmd/") {
		t.Errorf("first-level dirs must appear")
	}
}

func TestGenerate_Sizes(t *testing.T) {
	root := setupTree(t)
	out, err := Generate(Options{Root: root, IncludeSizes: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, " B)") && !strings.Contains(out, " KB)") {
		t.Errorf("expected size annotations, got:\n%s", out)
	}
}

func TestGenerate_NotADirectory(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file.txt")
	_ = os.WriteFile(file, []byte("content"), 0o644)

	_, err := Generate(Options{Root: file})
	if err == nil {
		t.Error("expected error for non-directory")
	}
}

func TestGenerate_EmptyDir(t *testing.T) {
	root := t.TempDir()
	out, err := Generate(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	// Should just show the root dir name
	if !strings.Contains(out, filepath.Base(root)) {
		t.Errorf("expected root dir name in output")
	}
}

func TestGenerate_DeepHierarchy(t *testing.T) {
	root := t.TempDir()
	deep := root
	for i := 0; i < 5; i++ {
		deep = filepath.Join(deep, "level"+string(rune('0'+i)))
	}
	_ = os.MkdirAll(deep, 0o755)
	_ = os.WriteFile(filepath.Join(deep, "deep.go"), []byte("package deep\n"), 0o644)

	out, err := Generate(Options{Root: root, MaxDepth: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "level0/") {
		t.Errorf("expected level0 in output")
	}
	if strings.Contains(out, "deep.go") {
		t.Errorf("deep.go should not appear with MaxDepth=3")
	}
}

func TestGenerate_Sorting(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "z_dir"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, "a_dir"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "z_file.txt"), []byte("z"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "a_file.txt"), []byte("a"), 0o644)

	out, err := Generate(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	aIdx := strings.Index(out, "a_")
	zIdx := strings.Index(out, "z_")
	if aIdx < 0 || zIdx < 0 {
		t.Errorf("missing files in output")
	}
	aDirIdx := strings.Index(out, "a_dir/")
	zDirIdx := strings.Index(out, "z_dir/")
	if aDirIdx > zDirIdx {
		t.Errorf("directories should be sorted: got\n%s", out)
	}
}

func setupFilteredTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "cmd", "dumper"), 0o755))
	must(os.MkdirAll(filepath.Join(root, "internal", "tree"), 0o755))
	must(os.MkdirAll(filepath.Join(root, "vendor", "lib"), 0o755))
	must(os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644))
	must(os.WriteFile(filepath.Join(root, "cmd", "dumper", "main.go"), []byte("package main\n"), 0o644))
	must(os.WriteFile(filepath.Join(root, "internal", "tree", "tree.go"), []byte("package tree\n"), 0o644))
	must(os.WriteFile(filepath.Join(root, "vendor", "lib", "vendor.go"), []byte("package lib\n"), 0o644))
	return root
}

func TestGenerate_ModeFull_ShowsAll(t *testing.T) {
	root := setupFilteredTree(t)
	out, err := Generate(Options{
		Root:     root,
		Mode:     ModeFull,
		Includes: []string{"**/*.md"},
		Excludes: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "go.mod") {
		t.Errorf("mode=full should show all files regardless of patterns; got:\n%s", out)
	}
}

func TestGenerate_ModeInclude_FiltersFiles(t *testing.T) {
	root := setupFilteredTree(t)
	out, err := Generate(Options{
		Root:     root,
		Mode:     ModeInclude,
		Includes: []string{"**/*"},
		Excludes: []string{"vendor/**"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"go.mod", "cmd/", "dumper/", "main.go", "internal/", "tree/", "tree.go"} {
		if !strings.Contains(out, want) {
			t.Errorf("mode=include should show %q, got:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"vendor/", "vendor.go"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("mode=include should NOT show %q (excluded by vendor/**), got:\n%s", unwanted, out)
		}
	}
}

func TestGenerate_ModeInclude_ExcludePattern(t *testing.T) {
	root := setupFilteredTree(t)
	out, err := Generate(Options{
		Root:     root,
		Mode:     ModeInclude,
		Includes: []string{"**/*"},
		Excludes: []string{"vendor/**", "cmd/**"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "vendor/") || strings.Contains(out, "cmd/") {
		t.Errorf("vendor/ and cmd/ should be excluded, got:\n%s", out)
	}
	if !strings.Contains(out, "internal/") {
		t.Errorf("internal/ should appear (has allowed file), got:\n%s", out)
	}
}

func TestGenerate_ModeInclude_EmptyResultShowsOnlyRoot(t *testing.T) {
	root := setupFilteredTree(t)
	out, err := Generate(Options{
		Root:     root,
		Mode:     ModeInclude,
		Includes: []string{"**/*.nonexistent"},
		Excludes: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, filepath.Base(root)+"/") {
		t.Errorf("should show only root when nothing matches, got:\n%s", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("expected only root line, got %d lines:\n%s", len(lines), out)
	}
}

func TestGenerate_ModeInclude_DirectoryPrunedWhenAllChildrenExcluded(t *testing.T) {
	root := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "only_excluded", "sub"), 0o755))
	must(os.WriteFile(filepath.Join(root, "only_excluded", "file.go"), []byte("package x\n"), 0o644))
	must(os.WriteFile(filepath.Join(root, "allowed.go"), []byte("package x\n"), 0o644))

	out, err := Generate(Options{
		Root:     root,
		Mode:     ModeInclude,
		Includes: []string{"**/*.go"},
		Excludes: []string{"only_excluded/**"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "only_excluded/") {
		t.Errorf("only_excluded/ should be pruned (all children excluded), got:\n%s", out)
	}
	if !strings.Contains(out, "allowed.go") {
		t.Errorf("allowed.go should appear, got:\n%s", out)
	}
}

func TestGenerate_ModeDefault_IsFull(t *testing.T) {
	root := setupFilteredTree(t)
	out, err := Generate(Options{
		Root:     root,
		Includes: []string{"**/*.md"},
		Excludes: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "go.mod") {
		t.Errorf("empty Mode should default to full (all files shown), got:\n%s", out)
	}
}

func TestGenerate_ModeInclude_WithAllowedFiles(t *testing.T) {
	root := setupFilteredTree(t)
	out, err := Generate(Options{
		Root: root,
		Mode: ModeInclude,
		AllowedFiles: []string{
			filepath.Join(root, "go.mod"),
			filepath.Join(root, "cmd", "dumper", "main.go"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "go.mod") {
		t.Errorf("go.mod should appear via AllowedFiles, got:\n%s", out)
	}
	if !strings.Contains(out, "main.go") {
		t.Errorf("main.go should appear via AllowedFiles, got:\n%s", out)
	}
	if strings.Contains(out, "vendor/") {
		t.Errorf("vendor/ should not appear (not in AllowedFiles), got:\n%s", out)
	}
}

// Regression: with Root "." (the default) the tree header used to render as
// "./" instead of the project directory name.
func TestRootDisplayName(t *testing.T) {
	tmp := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(old) }()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	if got := rootDisplayName("."); got != filepath.Base(tmp) {
		t.Errorf("rootDisplayName dot = %q, want %q", got, filepath.Base(tmp))
	}
	if got := rootDisplayName(filepath.Join(tmp, "sub")); got != "sub" {
		t.Errorf("rootDisplayName(sub) = %q, want sub", got)
	}
}

// Regression: full-mode tree now honors excludes so it never advertises
// files that will never be dumped.
func TestGenerate_ModeFull_RespectsExcludes(t *testing.T) {
	root := setupFilteredTree(t)
	out, err := Generate(Options{
		Root:     root,
		Mode:     ModeFull,
		Excludes: []string{"vendor/**"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "vendor/") || strings.Contains(out, "vendor.go") {
		t.Errorf("full mode must honor excludes, got:\n%s", out)
	}
	if !strings.Contains(out, "go.mod") {
		t.Errorf("non-excluded files must still appear, got:\n%s", out)
	}
}
