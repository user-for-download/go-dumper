package walker

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

type Entry struct {
	Path string
	Size int64
}

type Options struct {
	Root          string
	Includes      []string
	Excludes      []string
	IncludeHidden bool
}

type Walker struct {
	opts   Options
	errors []error
}

func New(opts Options) (*Walker, error) {
	inc, err := expandAtFiles(opts.Includes)
	if err != nil {
		return nil, err
	}
	exc, err := expandAtFiles(opts.Excludes)
	if err != nil {
		return nil, err
	}
	if len(inc) == 0 {
		inc = []string{"**/*"}
	}
	opts.Includes = normalizePatterns(inc)
	opts.Excludes = normalizePatterns(exc)
	return &Walker{opts: opts}, nil
}

func normalizePatterns(patterns []string) []string {
	normalized := make([]string, len(patterns))
	for i, p := range patterns {
		normalized[i] = filepath.ToSlash(strings.TrimSpace(p))
	}
	return normalized
}

func (w *Walker) Collect() ([]Entry, error) {
	var result []Entry
	root := w.opts.Root

	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}

	visitedDirs := make(map[string]bool)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			w.errors = append(w.errors, walkErr)
			return nil
		}
		name := d.Name()

		isRoot := false
		if absPath, err := filepath.Abs(path); err == nil {
			isRoot = absPath == absRoot
		}
		if !isRoot && !w.opts.IncludeHidden && strings.HasPrefix(name, ".") && name != "." {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			absPath, _ := filepath.Abs(path)
			if visitedDirs[absPath] {
				return filepath.SkipDir
			}
			visitedDirs[absPath] = true
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)

		if !w.matchAny(w.opts.Includes, rel) {
			return nil
		}
		if w.matchAny(w.opts.Excludes, rel) {
			return nil
		}
		info, err := d.Info()
		size := int64(0)
		if err == nil {
			size = info.Size()
		}
		result = append(result, Entry{Path: path, Size: size})
		return nil
	})
	if err != nil {
		w.errors = append(w.errors, err)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

func (w *Walker) Errors() []error {
	return w.errors
}

func (w *Walker) matchAny(patterns []string, rel string) bool {
	for _, p := range patterns {
		if p == "" {
			continue
		}
		ok, err := doublestar.PathMatch(p, rel)
		if err == nil && ok {
			return true
		}
	}
	return false
}
