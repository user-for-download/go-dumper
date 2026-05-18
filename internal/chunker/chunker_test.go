package chunker

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/yourname/dumper/internal/util"
)

func newTestChunker(t *testing.T, max int) (*Chunker, string) {
	t.Helper()
	dir := t.TempDir()
	c, err := New(Options{
		OutputDir:  dir,
		Prefix:     "dump",
		MaxSymbols: max,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c, dir
}

func readChunks(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var contents []string
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		contents = append(contents, string(b))
	}
	return contents
}

func TestChunker_SingleChunk(t *testing.T) {
	c, dir := newTestChunker(t, 100)
	for _, s := range []string{"hello\n", "world\n"} {
		if err := c.WriteString(s); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	chunks := readChunks(t, dir)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != "hello\nworld\n" {
		t.Errorf("unexpected content: %q", chunks[0])
	}
	if c.ChunkCount() != 1 {
		t.Errorf("ChunkCount = %d, want 1", c.ChunkCount())
	}
}

func TestChunker_RotateOnOverflow(t *testing.T) {
	c, dir := newTestChunker(t, 10)

	for i := 0; i < 3; i++ {
		if err := c.WriteString("abcde\n"); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	chunks := readChunks(t, dir)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d (%v)", len(chunks), chunks)
	}
	for i, c := range chunks {
		if c != "abcde\n" {
			t.Errorf("chunk %d = %q, want %q", i, c, "abcde\n")
		}
	}
}

func TestChunker_OversizedLineIsSplit(t *testing.T) {
	dir := t.TempDir()
	c, err := New(Options{
		OutputDir:      dir,
		Prefix:         "dump",
		MaxSymbols:     5,
		SplitLongLines: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := c.WriteString("short\n"); err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("x", 50) + "\n"
	if err := c.WriteString(big); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteString("tail\n"); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	chunks := readChunks(t, dir)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}

	for _, c := range chunks {
		if util.RuneCount(c) > 5 {
			t.Errorf("no chunk may exceed MaxSymbols; got %d runes in %q", util.RuneCount(c), c)
		}
	}

	foundShort := false
	foundTail := false
	for _, c := range chunks {
		if strings.Contains(c, "short") {
			foundShort = true
		}
		if strings.Contains(c, "tail") {
			foundTail = true
		}
	}
	if !foundShort || !foundTail {
		t.Errorf("short and tail must be preserved; got %v", chunks)
	}
}

func TestChunker_UnicodeRuneCounting(t *testing.T) {
	c, dir := newTestChunker(t, 14)

	s := "Привет\n"
	if utf8.RuneCountInString(s) != 7 {
		t.Fatalf("test precondition broken: rune count = %d", utf8.RuneCountInString(s))
	}
	for i := 0; i < 2; i++ {
		if err := c.WriteString(s); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	chunks := readChunks(t, dir)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for 14 runes, got %d", len(chunks))
	}
	if chunks[0] != s+s {
		t.Errorf("unexpected content: %q", chunks[0])
	}
}

func TestChunker_FileNamingFormat(t *testing.T) {
	dir := t.TempDir()
	c, err := New(Options{
		OutputDir:      dir,
		Prefix:         "dump",
		MaxSymbols:     3,
		SplitLongLines: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err := c.WriteString("aaaa"); err != nil {
			t.Fatal(err)
		}
	}
	_ = c.Close()

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "dump_") || !strings.HasSuffix(name, ".txt") {
			t.Errorf("bad filename: %s", name)
		}
		if len(name) != len("dump_00001.txt") {
			t.Errorf("expected zero-padded name, got: %s", name)
		}
	}
}

func TestChunker_EmptyWriteIsNoop(t *testing.T) {
	c, dir := newTestChunker(t, 10)
	if err := c.WriteString(""); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	chunks := readChunks(t, dir)
	if len(chunks) != 0 {
		t.Errorf("expected no chunks for empty write, got %v", chunks)
	}
}

func TestChunker_SplitLongLines(t *testing.T) {
	dir := t.TempDir()
	c, err := New(Options{
		OutputDir:      dir,
		Prefix:         "dump",
		MaxSymbols:     5,
		SplitLongLines: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := c.WriteString(strings.Repeat("a", 13)); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	chunks := readChunks(t, dir)
	want := []string{"aaaaa", "aaaaa", "aaa"}
	if len(chunks) != len(want) {
		t.Fatalf("want %d chunks, got %d (%v)", len(want), len(chunks), chunks)
	}
	for i := range want {
		if chunks[i] != want[i] {
			t.Errorf("chunk %d: got %q want %q", i, chunks[i], want[i])
		}
	}
}

func TestChunker_SplitLongLines_Unicode(t *testing.T) {
	dir := t.TempDir()
	c, err := New(Options{
		OutputDir:      dir,
		Prefix:         "dump",
		MaxSymbols:     3,
		SplitLongLines: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	s := "Привет!"
	if utf8.RuneCountInString(s) != 7 {
		t.Fatalf("precondition: rune count = %d", utf8.RuneCountInString(s))
	}
	if err := c.WriteString(s); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	chunks := readChunks(t, dir)
	want := []string{"При", "вет", "!"}
	if len(chunks) != len(want) {
		t.Fatalf("want %d chunks, got %d (%v)", len(want), len(chunks), chunks)
	}
	for i := range want {
		if chunks[i] != want[i] {
			t.Errorf("chunk %d: got %q want %q", i, chunks[i], want[i])
		}
		if utf8.RuneCountInString(chunks[i]) > 3 {
			t.Errorf("chunk %d exceeds MaxSymbols", i)
		}
	}
}

func TestChunker_OversizedContentIsRejectedWhenSplitDisabled(t *testing.T) {
	dir := t.TempDir()
	c, err := New(Options{
		OutputDir:      dir,
		Prefix:         "dump",
		MaxSymbols:     5,
		SplitLongLines: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("x", 20)
	err = c.WriteString(big)
	if err == nil {
		t.Fatal("expected error when line exceeds max_symbols and split_long_lines is false")
	}
	_ = c.Close()
}

func TestChunker_WriteBytes(t *testing.T) {
	c, dir := newTestChunker(t, 100)
	if err := c.WriteBytes([]byte("hello\n"), 6); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	chunks := readChunks(t, dir)
	if len(chunks) != 1 || chunks[0] != "hello\n" {
		t.Errorf("WriteBytes failed: %v", chunks)
	}
}

func TestChunker_WriteBytes_RuneCount(t *testing.T) {
	c, dir := newTestChunker(t, 10)
	if err := c.WriteBytes([]byte("привет\n"), 7); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	chunks := readChunks(t, dir)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != "привет\n" {
		t.Errorf("content mismatch: %q", chunks[0])
	}
	if util.RuneCount(chunks[0]) != 7 {
		t.Errorf("rune count should be 7")
	}
}
