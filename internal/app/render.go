package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/user-for-download/go-dumper/internal/cleaner"
	"github.com/user-for-download/go-dumper/internal/format"
	"github.com/user-for-download/go-dumper/internal/util"
)

var ErrBinaryFile = errors.New("binary file")

type EffectiveExcludesFunc func(root, output string, excludes []string, excludeSelf bool) []string

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
	header string
	footer string
	bytes  int64
	runes  int64
}

// renderFile streams pre-opened file content through the cleaner into
// writeLine, applying the formatter's EscapeBody to each line before counting
// and writing. The caller is responsible for sniffing binary and writing
// header/footer. Returns byte and rune counts of the escaped output.
func renderFile(f *os.File, ext string, mode cleaner.Mode, fmtr format.Formatter, writeLine func(string) error) (bytes int64, runes int64, err error) {
	if err := cleaner.Stream(f, ext, mode, func(line string) error {
		escaped := fmtr.EscapeBody(line)
		bytes += int64(len(escaped))
		runes += int64(util.RuneCount(escaped))
		return writeLine(escaped)
	}); err != nil {
		return 0, 0, fmt.Errorf("clean: %w", err)
	}
	return bytes, runes, nil
}

func autoExcludeOutput(root string, output string, excludes []string) []string {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return excludes
	}
	absOut, err := filepath.Abs(output)
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

func autoExcludeSelf(cfgPath string, excludes []string, excludeSelf bool) []string {
	if !excludeSelf {
		return excludes
	}
	exe, err := os.Executable()
	if err != nil {
		return excludes
	}
	absExe, err := filepath.Abs(exe)
	if err != nil {
		return excludes
	}
	absRoot, err := filepath.Abs(cfgPath)
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
