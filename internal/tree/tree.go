package tree

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/user-for-download/go-dumper/internal/glob"
)

type Mode string

const (
	ModeFull    Mode = "full"
	ModeInclude Mode = "include"
)

type Options struct {
	Root          string
	MaxDepth      int
	IncludeSizes  bool
	IncludeHidden bool

	Mode     Mode
	Includes []string
	Excludes []string
	Type     []string

	// AllowedFiles is an optional list of pre-collected file paths (from the
	// walker) for ModeInclude. When set, the tree is built from these paths
	// instead of re-walking the filesystem.
	AllowedFiles    []string
	AllowedFilesSet bool
}

func (o *Options) mode() Mode {
	switch o.Mode {
	case ModeInclude:
		return ModeInclude
	default:
		return ModeFull
	}
}

func Generate(opts Options) (string, error) {
	info, err := os.Stat(opts.Root)
	if err != nil {
		return "", fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("root is not a directory: %s", opts.Root)
	}
	if err := glob.Validate(opts.Includes); err != nil {
		return "", err
	}
	if err := glob.Validate(opts.Excludes); err != nil {
		return "", err
	}

	var allowed map[string]struct{}
	if opts.mode() == ModeInclude {
		if opts.AllowedFilesSet || len(opts.AllowedFiles) > 0 {
			allowed = make(map[string]struct{}, len(opts.AllowedFiles))
			absRoot, _ := filepath.Abs(opts.Root)
			for _, p := range opts.AllowedFiles {
				absPath, _ := filepath.Abs(p)
				rel, rerr := filepath.Rel(absRoot, absPath)
				if rerr != nil {
					// Fallback: strip leading "./" and normalize so the key
					// matches the format writeChildren builds (e.g. "foo/bar.go").
					rel = filepath.Clean(p)
				}
				allowed[filepath.ToSlash(rel)] = struct{}{}
			}
		} else {
			allowed, err = collectAllowed(opts)
			if err != nil {
				return "", err
			}
		}
	}

	var sb strings.Builder
	sb.WriteString(rootDisplayName(opts.Root))
	sb.WriteString("/\n")
	if err := writeChildren(&sb, opts.Root, "", 1, opts, allowed); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// rootDisplayName returns a sensible label for the tree root. filepath.Base
// of "." is ".", which would render the tree header as "./"; resolve to the
// real directory name in that case.
func rootDisplayName(root string) string {
	name := filepath.Base(root)
	if name == "." || name == string(filepath.Separator) || name == "/" || name == "" {
		if abs, err := filepath.Abs(root); err == nil {
			if base := filepath.Base(abs); base != "/" && base != "." && base != "" {
				return base
			}
		}
	}
	return name
}

func collectAllowed(opts Options) (map[string]struct{}, error) {
	set := make(map[string]struct{})
	err := filepath.WalkDir(opts.Root, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}

		isRoot := path == opts.Root || path == filepath.Clean(opts.Root)
		name := d.Name()
		if !isRoot && !opts.IncludeHidden && strings.HasPrefix(name, ".") && name != "." {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(opts.Root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		matchedInclude := glob.MatchAny(opts.Includes, rel)
		matchedExclude := glob.MatchAny(opts.Excludes, rel)

		if !matchedInclude {
			return nil
		}
		if matchedExclude && !glob.IncludeMoreSpecific(opts.Includes, opts.Excludes, rel) {
			return nil
		}
		if len(opts.Type) > 0 {
			ext := filepath.Ext(name)
			if ext == "" {
				return nil
			}
			cleanExt := strings.TrimPrefix(ext, ".")
			matched := false
			for _, t := range opts.Type {
				if cleanExt == strings.TrimPrefix(t, ".") {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}
		}
		set[rel] = struct{}{}
		return nil
	})
	return set, err
}

func hasAllowedDescendant(allowed map[string]struct{}, dirRel string) bool {
	prefix := dirRel + "/"
	for p := range allowed {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

func writeChildren(sb *strings.Builder, dir, prefix string, depth int, opts Options, allowed map[string]struct{}) error {
	if opts.MaxDepth > 0 && depth > opts.MaxDepth {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	dirRel, _ := filepath.Rel(opts.Root, dir)
	dirRel = filepath.ToSlash(dirRel)
	if dirRel == "." {
		dirRel = ""
	}

	filtered := make([]os.DirEntry, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !opts.IncludeHidden && strings.HasPrefix(name, ".") {
			continue
		}
		childRel := name
		if dirRel != "" {
			childRel = dirRel + "/" + name
		}
		if opts.mode() == ModeFull {
			// Full mode shows every file, but still honors excludes (vendor/,
			// build dirs, the auto-excluded output directory, ...) so the tree
			// never advertises files that will never be dumped.
			if e.IsDir() {
				if glob.MatchAny(opts.Excludes, childRel) || glob.MatchAny(opts.Excludes, childRel+"/**") {
					continue
				}
			} else if glob.MatchAny(opts.Excludes, childRel) {
				continue
			}
		}
		if allowed != nil {
			if e.IsDir() {
				if !hasAllowedDescendant(allowed, childRel) {
					continue
				}
			} else {
				if _, ok := allowed[childRel]; !ok {
					continue
				}
			}
		}
		filtered = append(filtered, e)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		di, dj := filtered[i].IsDir(), filtered[j].IsDir()
		if di != dj {
			return di
		}
		return filtered[i].Name() < filtered[j].Name()
	})

	for i, e := range filtered {
		isLast := i == len(filtered)-1
		branch := "├── "
		nextPrefix := prefix + "│   "
		if isLast {
			branch = "└── "
			nextPrefix = prefix + "    "
		}
		name := e.Name()
		if e.IsDir() {
			sb.WriteString(prefix + branch + name + "/\n")
			if err := writeChildren(sb, filepath.Join(dir, name), nextPrefix, depth+1, opts, allowed); err != nil {
				return err
			}
			continue
		}
		line := prefix + branch + name
		if opts.IncludeSizes {
			if info, err := e.Info(); err == nil {
				line += " (" + humanSize(info.Size()) + ")"
			}
		}
		sb.WriteString(line + "\n")
	}
	return nil
}

func humanSize(n int64) string {
	const (
		KB = 1 << 10
		MB = 1 << 20
		GB = 1 << 30
	)
	switch {
	case n >= GB:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(GB))
	case n >= MB:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(MB))
	case n >= KB:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(KB))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
