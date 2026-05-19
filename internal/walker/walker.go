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
	Type          string
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
		if w.matchIncludePriority(w.opts.Includes, w.opts.Excludes, rel) {
			return nil
		}
		if w.opts.Type != "" {
			ext := filepath.Ext(name)
			if ext == "" || strings.TrimPrefix(ext, ".") != w.opts.Type {
				return nil
			}
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

func (w *Walker) matchIncludePriority(includes, excludes []string, rel string) bool {
	matchedInclude := w.matchAny(includes, rel)
	matchedExclude := w.matchAny(excludes, rel)

	if !matchedInclude {
		return matchedExclude
	}

	if !matchedExclude {
		return false
	}

	return !w.anyIncludeMoreSpecificThanExclude(includes, excludes, rel)
}

func (w *Walker) anyIncludeMoreSpecificThanExclude(includes, excludes []string, rel string) bool {
	for _, inc := range includes {
		if inc == "" {
			continue
		}
		ok, err := doublestar.PathMatch(inc, rel)
		if err != nil || !ok {
			continue
		}
		if !hasWildcard(inc) {
			return true
		}
		for _, exc := range excludes {
			if exc == "" {
				continue
			}
			ok2, err2 := doublestar.PathMatch(exc, rel)
			if err2 != nil || !ok2 {
				continue
			}
			if patternSpecificity(inc) > patternSpecificity(exc) {
				return true
			}
		}
	}
	return false
}

func hasWildcard(p string) bool {
	return strings.Contains(p, "*") || strings.Contains(p, "?")
}

func patternSpecificity(p string) int {
	literals := 0
	for _, c := range p {
		if c != '*' && c != '?' {
			literals++
		}
	}
	return literals
}
