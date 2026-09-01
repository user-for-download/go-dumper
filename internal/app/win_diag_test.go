package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/user-for-download/go-dumper/internal/glob"
)

func TestWinDiagWalkerProbe(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "text.txt"), []byte("hello\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "bm.bin"), []byte{0x7F, 'E', 'L', 'F', 0x00}, 0o644)
	_ = os.WriteFile(filepath.Join(root, "nested", "deep.txt"), []byte("deep\n"), 0o644)

	absRoot, _ := filepath.Abs(root)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		rel, rerr := filepath.Rel(absRoot, path)
		relSlash := filepath.ToSlash(rel)
		if rerr != nil {
			relSlash = path
		}
		fmt.Printf("::notice::WINDIAG walk path=%q absRoot=%q dir=%v name=%q rel=%q match=%v\n", path, absRoot, d.IsDir(), d.Name(), relSlash, glob.MatchAny([]string{"**/*"}, relSlash))
		return nil
	})
	if err != nil {
		fmt.Printf("::notice::WINDIAG walkerr=%v\n", err)
	}
	t.Fatalf("WINDIAG done")
}
