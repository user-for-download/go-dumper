package walker

import (
	"io/fs"
	"os"
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
	Type          []string
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
	seen := make(map[string]struct{})

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
		absPath, absErr := filepath.Abs(path)
		if absErr == nil {
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

		rel, relErr := filepath.Rel(absRoot, absPath)
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
		if len(w.opts.Type) > 0 {
			ext := filepath.Ext(name)
			if ext == "" {
				return nil
			}
			cleanExt := strings.TrimPrefix(ext, ".")
			matched := false
			for _, t := range w.opts.Type {
				if cleanExt == strings.TrimPrefix(t, ".") {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}
		}
		info, err := d.Info()
		size := int64(0)
		if err == nil {
			size = info.Size()
		}
		result = append(result, Entry{Path: path, Size: size})
		if absErr == nil {
			seen[absPath] = struct{}{}
		}
		return nil
	})
	if err != nil {
		w.errors = append(w.errors, err)
	}

	for _, inc := range w.opts.Includes {
		pat := inc
		if !filepath.IsAbs(pat) {
			pat = filepath.Join(root, pat)
		}
		matches, _ := doublestar.FilepathGlob(pat)
		for _, match := range matches {
			absMatch, absErr := filepath.Abs(match)
			if absErr != nil {
				absMatch = match
			}
			if _, ok := seen[absMatch]; ok {
				continue
			}
			info, err := os.Stat(match)
			if err != nil || info.IsDir() {
				continue
			}

			// Skip IncludeHidden check for explicit (non-wildcard) patterns so that
			// explicitly named hidden files like "/path/to/.env" are always allowed.
			if !w.opts.IncludeHidden && strings.HasPrefix(info.Name(), ".") {
				if hasWildcard(inc) {
					continue
				}
			}

			rel, relErr := filepath.Rel(absRoot, absMatch)
			if relErr != nil {
				rel = match
			}
			rel = filepath.ToSlash(rel)

			if !w.matchAny(w.opts.Includes, rel) {
				continue
			}
			if w.matchIncludePriority(w.opts.Includes, w.opts.Excludes, rel) {
				continue
			}
			if len(w.opts.Type) > 0 {
				ext := filepath.Ext(info.Name())
				if ext == "" {
					continue
				}
				cleanExt := strings.TrimPrefix(ext, ".")
				matched := false
				for _, t := range w.opts.Type {
					if cleanExt == strings.TrimPrefix(t, ".") {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			result = append(result, Entry{Path: match, Size: info.Size()})
			seen[absMatch] = struct{}{}
		}
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

func (w *Walker) Errors() []error {
	return w.errors
}

// matchPattern checks whether pattern matches rel. For wildcard patterns it
// uses doublestar.PathMatchUnvalidated. For plain (non-wildcard) patterns it
// also matches as a directory prefix — e.g. pattern "cmd" matches "cmd/main.go".
func matchPattern(pattern, rel string) bool {
	if doublestar.PathMatchUnvalidated(pattern, rel) {
		return true
	}
	if !hasWildcard(pattern) {
		dirPrefix := strings.TrimSuffix(pattern, "/") + "/"
		return strings.HasPrefix(rel, dirPrefix)
	}
	return false
}

func (w *Walker) matchAny(patterns []string, rel string) bool {
	for _, p := range patterns {
		if p == "" {
			continue
		}
		if matchPattern(p, rel) {
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
		if !matchPattern(inc, rel) {
			continue
		}
		// Exact file match (no wildcards, PathMatch succeeded) always wins.
		if !hasWildcard(inc) && doublestar.PathMatchUnvalidated(inc, rel) {
			return true
		}
		for _, exc := range excludes {
			if exc == "" {
				continue
			}
			if !matchPattern(exc, rel) {
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
	return strings.ContainsAny(p, "*?")
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
