package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yourname/dumper/internal/cleaner"
	"github.com/yourname/dumper/internal/format"
	"github.com/yourname/dumper/internal/util"
)

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
	header  string
	footer  string
	payload []byte
	bytes   int64
	runes   int64
}

// renderFile processes a single file and returns its formatted content.
func renderFile(sf sniffedFile, root string, mode cleaner.Mode, fmtr format.Formatter) (renderResult, error) {
	var r renderResult

	f, err := os.Open(sf.path)
	if err != nil {
		return r, err
	}
	defer f.Close()

	rel, _ := filepath.Rel(root, sf.path)
	rel = filepath.ToSlash(rel)
	r.header = fmtr.FileHeader(rel)
	r.footer = fmtr.FileFooter(rel)

	ext := filepath.Ext(sf.path)

	var buf bytes.Buffer
	emit := func(line string) error {
		r.bytes += int64(len(line))
		r.runes += int64(util.RuneCount(line))
		_, err := buf.WriteString(line)
		return err
	}
	if err := cleaner.Stream(f, ext, mode, emit); err != nil {
		return r, fmt.Errorf("clean: %w", err)
	}
	r.payload = buf.Bytes()
	return r, nil
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
