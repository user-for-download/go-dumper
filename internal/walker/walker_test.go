package walker

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWalker_Basic(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.go"), "package a")
	writeFile(t, filepath.Join(root, "b.txt"), "hello")

	w, err := New(Options{
		Root:     root,
		Includes: []string{"**/*"},
		Excludes: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := w.Collect()
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, e := range entries {
		rel, _ := filepath.Rel(root, e.Path)
		names = append(names, filepath.ToSlash(rel))
	}
	sort.Strings(names)

	want := []string{"a.go", "b.txt"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
}

func TestWalker_IncludeExclude(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "src", "main.go"), "package main")
	writeFile(t, filepath.Join(root, "src", "util.go"), "package util")
	writeFile(t, filepath.Join(root, "docs", "readme.md"), "# readme")
	writeFile(t, filepath.Join(root, "vendor", "lib.go"), "package lib")

	w, err := New(Options{
		Root:     root,
		Includes: []string{"src/**/*.go"},
		Excludes: []string{"**/util.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := w.Collect()
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, e := range entries {
		rel, _ := filepath.Rel(root, e.Path)
		names = append(names, filepath.ToSlash(rel))
	}
	sort.Strings(names)

	want := []string{"src/main.go"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
}

func TestWalker_DefaultInclude(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), "a")
	writeFile(t, filepath.Join(root, "b.go"), "package b")

	w, err := New(Options{
		Root:     root,
		Includes: nil,
		Excludes: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := w.Collect()
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Errorf("expected 2 files, got %d", len(entries))
	}
}

func TestWalker_IncludeHidden(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".hidden"), "hidden")
	writeFile(t, filepath.Join(root, "visible.txt"), "visible")

	w, err := New(Options{
		Root:          root,
		Includes:      []string{"**/*"},
		IncludeHidden: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	files, err := w.Collect()
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, e := range files {
		rel, _ := filepath.Rel(root, e.Path)
		names = append(names, filepath.ToSlash(rel))
	}

	if len(names) != 1 || names[0] != "visible.txt" {
		t.Errorf("hidden file should be excluded: got %v", names)
	}
}

func TestWalker_IncludeHidden_True(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".hidden"), "hidden")
	writeFile(t, filepath.Join(root, "visible.txt"), "visible")

	w, err := New(Options{
		Root:          root,
		Includes:      []string{"**/*"},
		IncludeHidden: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := w.Collect()
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Errorf("expected 2 files, got %d", len(entries))
	}
}

func TestIsInsideHiddenDir(t *testing.T) {
	tests := []struct {
		path, root string
		want       bool
	}{
		{"/project/.hidden/file.go", "/project", true},
		{"/project/.git/config", "/project", true},
		{"/project/src/main.go", "/project", false},
		{"/project/.hidden/.nested/file.go", "/project", true},
	}
	for _, tt := range tests {
		got := isInsideHiddenDir(tt.path, tt.root)
		if got != tt.want {
			t.Errorf("isInsideHiddenDir(%q, %q) = %v, want %v", tt.path, tt.root, got, tt.want)
		}
	}
}

func TestWalker_FilesInsideHiddenDirExcluded(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".hidden", "secret.go"), "package secret")
	writeFile(t, filepath.Join(root, "visible.go"), "package visible")

	w, err := New(Options{
		Root:          root,
		Includes:      []string{"**/*"},
		IncludeHidden: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := w.Collect()
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, e := range entries {
		rel, _ := filepath.Rel(root, e.Path)
		names = append(names, filepath.ToSlash(rel))
	}

	if len(names) != 1 || names[0] != "visible.go" {
		t.Errorf("files inside hidden dirs should be excluded: got %v", names)
	}
}

func TestWalker_Errors(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), "a")

	w, err := New(Options{
		Root:     root,
		Includes: []string{"**/*"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Collect()
	if err != nil {
		t.Fatal(err)
	}

	errs := w.Errors()
	if len(errs) > 0 {
		t.Logf("collected %d errors: %v", len(errs), errs)
	}
}

func TestExpandAtFiles_Mixed(t *testing.T) {
	dir := t.TempDir()
	listPath := filepath.Join(dir, "patterns.txt")
	writeFile(t, listPath, "**/*.go\n**/*.md\n")

	in := []string{"src/**", "@" + listPath, "docs/**"}
	out, err := expandAtFiles(in)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"src/**", "**/*.go", "**/*.md", "docs/**"}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("got %v, want %v", out, want)
	}
}

func TestExpandAtFiles_Missing(t *testing.T) {
	_, err := expandAtFiles([]string{"@/no/such/file_xyz"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestExpandAtFiles_EmptyAndComments(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "p.txt")
	writeFile(t, p, "# header\n\n   \n# more\n")
	out, err := expandAtFiles([]string{"@" + p})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty list, got %v", out)
	}
}

func TestWalker_WithAtFile(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "src"), 0o755)
	writeFile(t, filepath.Join(root, "src", "a.go"), "package a")
	writeFile(t, filepath.Join(root, "src", "b.txt"), "x")
	writeFile(t, filepath.Join(root, "README.md"), "hi")

	listPath := filepath.Join(root, "inc.txt")
	writeFile(t, listPath, "**/*.go\n**/*.md\n")

	w, err := New(Options{
		Root:     root,
		Includes: []string{"@" + listPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := w.Collect()
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, e := range entries {
		rel, _ := filepath.Rel(root, e.Path)
		names = append(names, filepath.ToSlash(rel))
	}
	sort.Strings(names)

	want := []string{"README.md", "src/a.go"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
}

func TestWalker_AbsoluteInclude(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "main.go")
	writeFile(t, target, "package main")
	w, err := New(Options{Root: root, Includes: []string{target}})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := w.Collect()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != target {
		t.Fatalf("got %#v, want %s", entries, target)
	}
}

func TestWalker_SkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	writeFile(t, outside, "secret")
	if err := os.Symlink(outside, filepath.Join(root, "secret.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	w, err := New(Options{Root: root, Includes: []string{"**/*"}})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := w.Collect()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("symlink should be skipped: %#v", entries)
	}
}

func TestWalker_PrunesExcludedDirectories(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "node_modules", "dep", "index.js"), "dependency")
	writeFile(t, filepath.Join(root, "src", "main.go"), "package main")
	w, err := New(Options{
		Root:     root,
		Includes: []string{"**/*"},
		Excludes: []string{"node_modules/**"},
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := w.Collect()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || filepath.Base(entries[0].Path) != "main.go" {
		t.Fatalf("excluded directory was traversed or included: %#v", entries)
	}
}

func TestWalker_RejectsInvalidPattern(t *testing.T) {
	root := t.TempDir()
	_, err := New(Options{Root: root, Includes: []string{"[invalid"}})
	if err == nil {
		t.Fatal("invalid glob pattern must be rejected, not silently ignored")
	}
}

func TestWalker_NegationInIncludes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.go"), "package a")
	writeFile(t, filepath.Join(root, "vendor", "b.go"), "package b")

	w, err := New(Options{
		Root:     root,
		Includes: []string{"**/*.go", "!**/vendor/**"},
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := w.Collect()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		rel, _ := filepath.Rel(root, e.Path)
		names = append(names, filepath.ToSlash(rel))
	}
	if len(names) != 1 || names[0] != "a.go" {
		t.Errorf("negated include should exclude vendor: got %v", names)
	}
}

func TestWalker_ExplicitIncludeInsideHiddenDir(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".config", "credentials.env"), "KEY=1")
	writeFile(t, filepath.Join(root, ".other", "junk.env"), "X=1")

	w, err := New(Options{
		Root:     root,
		Includes: []string{".config/credentials.env"},
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := w.Collect()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || filepath.Base(entries[0].Path) != "credentials.env" {
		t.Fatalf("explicit include of a hidden-dir file must work: %#v", entries)
	}
}

func TestResolveNegations(t *testing.T) {
	inc, exc := ResolveNegations(
		[]string{"**/*.go", "!vendor/**", "!x.txt"},
		[]string{"build/**", "!keep.txt"},
	)
	wantInc := []string{"**/*.go", "keep.txt"}
	wantExc := []string{"vendor/**", "x.txt", "build/**"}
	if !reflect.DeepEqual(inc, wantInc) || !reflect.DeepEqual(exc, wantExc) {
		t.Errorf("got inc=%v exc=%v, want inc=%v exc=%v", inc, exc, wantInc, wantExc)
	}
}
