package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/user-for-download/go-dumper/internal/util"
)

var ErrBinaryFile = errors.New("binary file")

func EffectiveExcludes(root, output string, excludes []string, excludeSelf bool) []string {
	out := excludes
	out = autoExcludeOutput(root, output, out)
	out = autoExcludeSelf(root, out, excludeSelf)
	return out
}

type sniffedFile struct {
	path string
	size int64
}

type renderResult struct {
	bytes int64
	runes int64
}

type outputError struct {
	err error
}

func (e *outputError) Error() string { return e.err.Error() }
func (e *outputError) Unwrap() error { return e.err }

// renderFile streams the file line by line. Passing complete lines (instead
// of raw read blocks) to writeLine guarantees that chunk rotation happens at
// line boundaries: rune-safe chunking, correct per-chunk rune accounting,
// and sane semantics for split_long_lines (which now really means "split
// oversized lines"). The final line is newline-terminated, so file content
// in the output never runs into the next header/footer.
func renderFile(f *os.File, writeLine func(string) error) (bytes int64, runes int64, err error) {
	r := bufio.NewReaderSize(f, 64*1024)
	for {
		line, readErr := r.ReadBytes('\n')
		if len(line) > 0 {
			if line[len(line)-1] != '\n' {
				line = append(line, '\n')
			}
			runes += int64(util.RuneCount(string(line)))
			if err := writeLine(string(line)); err != nil {
				return 0, 0, &outputError{err: err}
			}
			bytes += int64(len(line))
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return bytes, runes, nil
			}
			return 0, 0, fmt.Errorf("read: %w", readErr)
		}
	}
}

func autoExcludeOutput(root string, output string, excludes []string) []string {
	absRoot, err := canonicalPath(root)
	if err != nil {
		return excludes
	}
	absOut, err := canonicalPath(output)
	if err != nil {
		return excludes
	}
	rel, err := filepath.Rel(absRoot, absOut)
	if err != nil {
		return excludes
	}
	rel = filepath.ToSlash(rel)

	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return excludes
	}

	return append(excludes, rel+"/**")
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := filepath.Clean(abs)
	var suffix []string
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(abs), nil
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func outputContainsRoot(root, output string) bool {
	absRoot, rootErr := canonicalPath(root)
	absOutput, outputErr := canonicalPath(output)
	if rootErr != nil || outputErr != nil {
		return true
	}
	rel, err := filepath.Rel(absOutput, absRoot)
	return err == nil && (rel == "." || (rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func autoExcludeSelf(cfgPath string, excludes []string, excludeSelf bool) []string {
	if !excludeSelf {
		return excludes
	}
	exe, err := os.Executable()
	if err != nil {
		return excludes
	}
	// Use symlink-resolving canonical paths for BOTH sides so the binary is
	// still detected when the scan root or the executable path goes through
	// a symlink (e.g. /tmp -> /private/tmp on macOS).
	absExe, err := canonicalPath(exe)
	if err != nil {
		return excludes
	}
	absRoot, err := canonicalPath(cfgPath)
	if err != nil {
		return excludes
	}
	rel, err := filepath.Rel(absRoot, absExe)
	if err != nil {
		return excludes
	}
	if filepath.IsAbs(rel) {
		return excludes
	}
	rel = filepath.ToSlash(rel)
	if rel == "" || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return excludes
	}
	return append(excludes, rel)
}
