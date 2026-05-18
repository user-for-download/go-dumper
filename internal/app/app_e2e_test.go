package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/user-for-download/go-dumper/internal/config"
)

func readAllChunks(t *testing.T, dir string) (names []string, all string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "dump_") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var sb strings.Builder
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(b)
	}
	return names, sb.String()
}

func TestE2E_FullPipeline_NoLimits(t *testing.T) {
	root := filepath.Join("testdata", "sample")
	out := t.TempDir()

	cfg := &config.Config{
		Path:        root,
		Output:      out,
		Include:     []string{"**/*.go", "**/*.md", "**/*.sh"},
		Exclude:     []string{"vendor/**"},
		MaxSymbols:  1_000_000,
		ChunkPrefix: "dump",
		StatsFile:   filepath.Join(out, "stats.json"),
		Format:      "plain",
		Clear: config.ClearConfig{
			Enabled: true,
			Mode:    "line_and_block",
		},
		Tree: config.TreeConfig{
			Enabled: true,
		},
	}

	st, err := Run(cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	names, content := readAllChunks(t, out)
	if len(names) != 1 {
		t.Fatalf("expected 1 chunk, got %d (%v)", len(names), names)
	}

	if !strings.Contains(content, "===== PROJECT TREE =====") {
		t.Errorf("tree header missing")
	}

	for _, want := range []string{
		"FILE: README.md",
		"FILE: src/main.go",
		"FILE: pkg/util.go",
		"FILE: scripts/run.sh",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in output", want)
		}
	}

	if strings.Contains(content, "vendor/lib.go") || strings.Contains(content, "must be excluded") {
		t.Errorf("vendor must be excluded")
	}

	if strings.Contains(content, "// header comment") {
		t.Errorf("line comment must be stripped")
	}
	if strings.Contains(content, "block\n   comment") {
		t.Errorf("block comment must be stripped")
	}
	if strings.Contains(content, "# launcher") {
		t.Errorf("shell comment must be stripped")
	}

	for _, want := range []string{
		`package main`,
		`fmt.Println("hi")`,
		`func Add(a, b int) int`,
		`echo run`,
		`Hello world`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected code/text %q to be preserved", want)
		}
	}

	if st.SkippedFiles() != 0 {
		t.Errorf("expected 0 skipped, got %d", st.SkippedFiles())
	}
	if st.ProcessedFiles() != 4 {
		t.Errorf("expected 4 processed, got %d", st.ProcessedFiles())
	}

	raw, err := os.ReadFile(cfg.StatsFile)
	if err != nil {
		t.Fatalf("stats.json: %v", err)
	}
	var parsed struct {
		TotalFiles     int `json:"total_files"`
		ProcessedFiles int `json:"processed_files"`
		SkippedFiles   int `json:"skipped_files"`
		ChunksCreated  int `json:"chunks_created"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse stats.json: %v", err)
	}
	if parsed.TotalFiles != 4 || parsed.ProcessedFiles != 4 || parsed.SkippedFiles != 0 {
		t.Errorf("bad stats counters: %+v", parsed)
	}
}

func TestE2E_Chunking_RespectsMaxSymbols(t *testing.T) {
	root := filepath.Join("testdata", "sample")
	out := t.TempDir()

	const max = 80
	cfg := &config.Config{
		Path:           root,
		Output:         out,
		Include:        []string{"**/*"},
		Exclude:        []string{"vendor/**"},
		MaxSymbols:     max,
		ChunkPrefix:    "dump",
		SplitLongLines: true,
		Format:         "plain",
		Clear: config.ClearConfig{
			Enabled: false,
		},
	}

	_, err := Run(cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	names, _ := readAllChunks(t, out)
	if len(names) < 2 {
		t.Fatalf("expected multiple chunks for max=%d, got %d", max, len(names))
	}

	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(out, n))
		if err != nil {
			t.Fatal(err)
		}
		if rc := utf8.RuneCountInString(string(b)); rc > max {
			t.Errorf("chunk %s has %d runes > max %d", n, rc, max)
		}
	}
}

func TestE2E_Reassembly_PreservesContent(t *testing.T) {
	root := filepath.Join("testdata", "sample")
	out := t.TempDir()

	cfg := &config.Config{
		Path:           root,
		Output:         out,
		Include:        []string{"**/*"},
		Exclude:        []string{"vendor/**"},
		MaxSymbols:     50,
		ChunkPrefix:    "dump",
		SplitLongLines: true,
		Format:         "plain",
		Clear: config.ClearConfig{
			Enabled: false,
		},
	}

	_, err := Run(cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	_, joined := readAllChunks(t, out)

	out2 := t.TempDir()
	cfg2 := &config.Config{
		Path:           root,
		Output:         out2,
		Include:        []string{"**/*"},
		Exclude:        []string{"vendor/**"},
		MaxSymbols:     1_000_000,
		ChunkPrefix:    "dump",
		SplitLongLines: false,
		Format:         "plain",
		Clear: config.ClearConfig{
			Enabled: false,
		},
	}

	_, err = Run(cfg2)
	if err != nil {
		t.Fatalf("Run canonical: %v", err)
	}

	_, canonical := readAllChunks(t, out2)

	if joined != canonical {
		t.Errorf("re-assembled chunks (split mode) must equal canonical output\n--- joined ---\n%s\n--- canonical ---\n%s",
			joined, canonical)
	}
}

func TestE2E_BinaryFilesSkipped(t *testing.T) {
	root := t.TempDir()
	out := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "text.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "binary.bin"), []byte{0x7F, 'E', 'L', 'F', 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Path:        root,
		Output:      out,
		Include:     []string{"**/*"},
		MaxSymbols:  1_000_000,
		ChunkPrefix: "dump",
		StatsFile:   filepath.Join(out, "stats.json"),
		Format:      "plain",
	}

	st, err := Run(cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	_, content := readAllChunks(t, out)

	if strings.Contains(content, "binary.bin") {
		t.Errorf("binary file should be skipped")
	}
	if !strings.Contains(content, "text.txt") {
		t.Errorf("text file should be included")
	}

	if st.SkippedFiles() != 1 {
		t.Errorf("expected 1 skipped, got %d", st.SkippedFiles())
	}
	if st.ProcessedFiles() != 1 {
		t.Errorf("expected 1 processed, got %d", st.ProcessedFiles())
	}
}

func TestE2E_WithConcurrency(t *testing.T) {
	root := filepath.Join("testdata", "sample")
	out := t.TempDir()

	cfg := &config.Config{
		Path:        root,
		Output:      out,
		Include:     []string{"**/*.go", "**/*.md"},
		Exclude:     []string{"vendor/**"},
		MaxSymbols:  1_000_000,
		ChunkPrefix: "dump",
		Concurrency: 4,
		Format:      "plain",
	}

	st, err := Run(cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	_, content := readAllChunks(t, out)

	for _, want := range []string{"src/main.go", "pkg/util.go", "README.md"} {
		if !strings.Contains(content, "FILE: "+want) {
			t.Errorf("missing %s", want)
		}
	}

	if st.ProcessedFiles() != 3 {
		t.Errorf("expected 3 processed, got %d", st.ProcessedFiles())
	}

	names, _ := readAllChunks(t, out)
	if len(names) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(names))
	}
}

func TestE2E_TreeContent(t *testing.T) {
	root := filepath.Join("testdata", "sample")
	out := t.TempDir()

	cfg := &config.Config{
		Path:        root,
		Output:      out,
		Include:     []string{"**/*"},
		Exclude:     []string{"vendor/**"},
		MaxSymbols:  1_000_000,
		ChunkPrefix: "dump",
		Format:      "plain",
		Tree: config.TreeConfig{
			Enabled:      true,
			IncludeSizes: true,
			MaxDepth:     2,
		},
	}

	_, err := Run(cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	_, content := readAllChunks(t, out)

	if !strings.Contains(content, "===== PROJECT TREE =====") {
		t.Errorf("tree header missing")
	}
	if !strings.Contains(content, "src/") {
		t.Errorf("tree should contain src/")
	}
	if !strings.Contains(content, "main.go") {
		t.Errorf("tree with max depth 2 should show main.go (depth=2)")
	}
}

func TestE2E_OutputDirectoryAutoExcluded(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "dump_out")

	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "old_dump.txt"), []byte("SHOULD_NOT_APPEAR\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Path:        root,
		Output:      out,
		Include:     []string{"**/*"},
		MaxSymbols:  1_000_000,
		ChunkPrefix: "dump",
		Format:      "plain",
	}

	_, err := Run(cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	_, content := readAllChunks(t, out)

	if strings.Contains(content, "SHOULD_NOT_APPEAR") {
		t.Errorf("output directory content should be auto-excluded")
	}
	if !strings.Contains(content, "main.go") {
		t.Errorf("main.go should be included")
	}
}
