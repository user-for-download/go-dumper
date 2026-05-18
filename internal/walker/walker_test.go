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
