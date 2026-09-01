package walker

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/user-for-download/go-dumper/internal/glob"
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
	inc, exc = ResolveNegations(inc, exc)
	if len(inc) == 0 {
		inc = []string{"**/*"}
	}
	opts.Includes = normalizePatterns(inc)
	opts.Excludes = normalizePatterns(exc)
	if err := glob.Validate(opts.Includes); err != nil {
		return nil, err
	}
	if err := glob.Validate(opts.Excludes); err != nil {
		return nil, err
	}
	return &Walker{opts: opts}, nil
}

func normalizePatterns(patterns []string) []string {
	normalized := make([]string, 0, len(patterns))
	for _, p := range patterns {
		p = filepath.ToSlash(p)
		if p != "" {
			normalized = append(normalized, p)
		}
	}
	return normalized
}

func (w *Walker) Collect() ([]Entry, error) {
	w.errors = nil
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
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}

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
			dirRel, dirErr := filepath.Rel(absRoot, absPath)
			if dirErr == nil {
				dirRel = filepath.ToSlash(dirRel)
				for _, exclude := range w.opts.Excludes {
					if glob.MatchPattern(exclude, dirRel) || glob.MatchPattern(exclude, dirRel+"/") || glob.MatchPattern(exclude, dirRel+"/**") {
						return filepath.SkipDir
					}
				}
			}
			return nil
		}

		rel, relErr := filepath.Rel(absRoot, absPath)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)

		if !glob.MatchAny(w.opts.Includes, rel) {
			return nil
		}
		if glob.Excluded(w.opts.Includes, w.opts.Excludes, rel) {
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
		matches, gerr := doublestar.FilepathGlob(pat)
		if gerr != nil {
			w.errors = append(w.errors, fmt.Errorf("glob %q: %w", pat, gerr))
			continue
		}
		for _, match := range matches {
			if info, err := os.Lstat(match); err != nil || info.Mode()&os.ModeSymlink != 0 {
				continue
			}
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

			// Skip hidden files when IncludeHidden is false, unless the pattern
			// is an explicit (non-wildcard) reference like "@file.txt" that
			// intentionally names a hidden file.
			if !w.opts.IncludeHidden && strings.HasPrefix(info.Name(), ".") {
				if glob.HasWildcard(inc) {
					continue
				}
			}

			// Also skip files whose parent directory is hidden. The glob phase
			// doesn't prune hidden directories like WalkDir does, so we need
			// to check here to stay consistent. Explicit (non-wildcard)
			// includes are exempt so that naming a hidden file directly —
			// even inside a hidden directory — always works.
			if !w.opts.IncludeHidden && glob.HasWildcard(inc) && isInsideHiddenDir(match, root) {
				continue
			}

			rel, relErr := filepath.Rel(absRoot, absMatch)
			if relErr != nil {
				rel = match
			}
			rel = filepath.ToSlash(rel)

			matchTarget := rel
			if filepath.IsAbs(inc) {
				matchTarget = filepath.ToSlash(absMatch)
			}
			if !glob.MatchAny([]string{inc}, matchTarget) {
				continue
			}
			excluded := glob.Excluded(w.opts.Includes, w.opts.Excludes, rel)
			if matchTarget != rel {
				// Absolute include: also honor exclude patterns (including
				// absolute ones) against the real filesystem path.
				if glob.MatchAny(w.opts.Excludes, matchTarget) && !glob.IncludeMoreSpecific(w.opts.Includes, w.opts.Excludes, matchTarget) {
					excluded = true
				}
			}
			if filepath.IsAbs(inc) && !glob.HasWildcard(inc) {
				excluded = false
			}
			if excluded {
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
	return result, errors.Join(w.errors...)
}

func (w *Walker) Errors() []error {
	return w.errors
}

// isInsideHiddenDir reports whether path contains a hidden directory component
// (a segment starting with '.') between root and the file itself.
func isInsideHiddenDir(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	for _, part := range strings.Split(filepath.Dir(rel), string(filepath.Separator)) {
		if strings.HasPrefix(part, ".") && part != "." {
			return true
		}
	}
	return false
}
