package app

import (
	"encoding/json"
	"fmt"
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

	for _, want := range []string{
		`package main`,
		`fmt.Println("hi")`,
		`// header comment`,
		`block`,
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

// Regression: files larger than one 64KB read block with short lines used to
// fail with "content exceeds max symbols" when split_long_lines was false,
// because the chunker received raw read blocks instead of lines.
func TestE2E_LargeFile_ShortLines_SmallMaxSymbols(t *testing.T) {
	root := t.TempDir()
	out := t.TempDir()

	var sb strings.Builder
	for i := 0; i < 20000; i++ {
		fmt.Fprintf(&sb, "line %d of an ordinary text file\n", i)
	}
	big := filepath.Join(root, "big.txt")
	if err := os.WriteFile(big, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if len(sb.String()) < 128*1024 {
		t.Fatalf("precondition: test file must span multiple 64KB read blocks")
	}

	cfg := &config.Config{
		Path:        root,
		Output:      out,
		Include:     []string{"**/big.txt"},
		MaxSymbols:  1000,
		ChunkPrefix: "dump",
		Format:      "plain",
	}
	if _, err := Run(cfg); err != nil {
		t.Fatalf("Run with short lines and small max_symbols must succeed: %v", err)
	}

	names, joined := readAllChunks(t, out)
	for _, n := range names {
		b, _ := os.ReadFile(filepath.Join(out, n))
		if utf8.RuneCountInString(string(b)) > 1000 {
			t.Errorf("chunk %s exceeds max_symbols", n)
		}
	}
	if !strings.Contains(joined, "line 19999 of an ordinary text file") {
		t.Error("tail of large file must be preserved")
	}
}

// Regression: multi-byte runes used to be split across chunk boundaries at
// 64KB read-block edges, producing invalid UTF-8 chunks.
func TestE2E_MultibyteChunksAreValidUTF8(t *testing.T) {
	root := t.TempDir()

	line := strings.Repeat("\u3042", 50000) // 150KB, 3 bytes/rune, no newline
	if err := os.WriteFile(filepath.Join(root, "uni.txt"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, concurrency := range []int{1, 2} {
		out := t.TempDir()
		cfg := &config.Config{
			Path:           root,
			Output:         out,
			Include:        []string{"**/uni.txt"},
			MaxSymbols:     40000,
			ChunkPrefix:    "dump",
			SplitLongLines: true,
			Concurrency:    concurrency,
			Format:         "plain",
		}
		if _, err := Run(cfg); err != nil {
			t.Fatalf("Run (concurrency=%d): %v", concurrency, err)
		}
		names, joinedAll := readAllChunks(t, out)
		for _, n := range names {
			b, _ := os.ReadFile(filepath.Join(out, n))
			if !utf8.Valid(b) {
				t.Errorf("chunk %s (concurrency=%d) is not valid UTF-8", n, concurrency)
			}
		}
		joined := strings.TrimPrefix(joinedAll, "\n===== FILE: uni.txt =====\n")
		joined = strings.TrimSuffix(joined, "\n")
		if joined != line {
			t.Errorf("(concurrency=%d) content mismatch: joined length %d, want %d", concurrency, len(joined), len(line))
		}
	}
}

// Regression: markdown files without a trailing newline used to glue the
// closing code fence onto the last content line.
func TestE2E_MarkdownTrailingNewline(t *testing.T) {
	root := t.TempDir()
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "readme.md"), []byte("no trailing newline"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Path:        root,
		Output:      out,
		Include:     []string{"**/readme.md"},
		MaxSymbols:  1_000_000,
		ChunkPrefix: "dump",
		Format:      "markdown",
	}
	if _, err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	_, content := readAllChunks(t, out)
	if !strings.Contains(content, "no trailing newline\n```") {
		t.Errorf("closing fence must be on its own line, got:\n%s", content)
	}
}

// Regression: tree-mode include used to list binary files even though they
// are never dumped.
func TestE2E_TreeIncludeMode_ExcludesBinary(t *testing.T) {
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
		Format:      "plain",
		Tree: config.TreeConfig{
			Enabled: true,
			Mode:    "include",
		},
	}
	if _, err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	_, content := readAllChunks(t, out)
	if strings.Contains(content, "binary.bin") {
		t.Errorf("include-mode tree must not list binary files, got:\n%s", content)
	}
}

// Regression: invalid glob patterns used to be silently ignored (0 files
// dumped, exit 0).
func TestE2E_InvalidGlobFails(t *testing.T) {
	root := t.TempDir()
	out := t.TempDir()
	cfg := &config.Config{
		Path:        root,
		Output:      out,
		Include:     []string{"[invalid"},
		MaxSymbols:  1_000_000,
		ChunkPrefix: "dump",
		Format:      "plain",
	}
	if _, err := Run(cfg); err == nil {
		t.Fatal("invalid glob pattern must produce an error")
	}
}

// Regression: the tree used to list the output directory and the chunk file
// being written into it.
func TestE2E_TreeDoesNotListOutputDir(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "dump_out")
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Stale output from a previous run must not appear either.
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "dump_00001.txt"), []byte("STALE"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Path:        root,
		Output:      out,
		Include:     []string{"**/*.go"},
		MaxSymbols:  1_000_000,
		ChunkPrefix: "dump",
		Format:      "markdown",
		Tree:        config.TreeConfig{Enabled: true},
	}
	if _, err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	_, content := readAllChunks(t, out)
	if strings.Contains(content, "dump_out") {
		t.Errorf("tree must not list the output directory, got:\n%s", content)
	}
}

// Regression: failed runs used to leave partial chunk files behind.
func TestE2E_FailedRunLeavesNoChunks(t *testing.T) {
	root := t.TempDir()
	out := t.TempDir()
	// One single line larger than max_symbols with splitting disabled:
	// processing fails, and the partial dump must be cleaned up.
	if err := os.WriteFile(filepath.Join(root, "oneline.txt"), []byte(strings.Repeat("x", 5000)), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Path:        root,
		Output:      out,
		Include:     []string{"**/oneline.txt"},
		MaxSymbols:  1000,
		ChunkPrefix: "dump",
		Format:      "plain",
	}
	if _, err := Run(cfg); err == nil {
		t.Fatal("expected run to fail (oversized single line, split disabled)")
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "dump_") {
			t.Errorf("failed run must not leave chunk files behind, found %s", e.Name())
		}
	}
}

// Regression: a stats_file that collides with a chunk file must be rejected
// before any output is written.
func TestE2E_StatsFileCollisionFailsEarly(t *testing.T) {
	root := t.TempDir()
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Path:        root,
		Output:      out,
		Include:     []string{"**/a.txt"},
		MaxSymbols:  1_000_000,
		ChunkPrefix: "dump",
		Format:      "plain",
		StatsFile:   filepath.Join(out, "dump_00001.txt"),
	}
	if _, err := Run(cfg); err == nil {
		t.Fatal("stats_file colliding with a chunk file must fail")
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("no output must be written when the stats file collides, got %v", entries)
	}
}
