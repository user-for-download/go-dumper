package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/user-for-download/go-dumper/internal/walker"
)

// Temporary diagnostic: dump what the walker returns for a temp dir on Windows.
func TestWinDiagWalkerProbe(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "text.txt"), []byte("hello\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "bin.bin"), []byte{0x7F, 'E', 'L', 'F', 0x00}, 0o644)
	_ = os.WriteFile(filepath.Join(root, "nested", "deep.txt"), []byte("deep\n"), 0o644)

	w, err := walker.New(walker.Options{Root: root, Includes: []string{"**/*"}})
	if err != nil {
		t.Fatal(err)
	}
	entries, cerr := w.Collect()
	_ = cerr
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Path)
	}
	empties := w.Errors()
	fmt.Printf("::notice::WINDIAG walker root=%q entries=%v walkerr=%v\n", root, names, empties)
	t.Fatalf("WINDIAG entries=%v", names)
}
