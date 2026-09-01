// Package glob centralizes doublestar pattern handling shared by the walker
// and the tree generator: matching, include-priority resolution, specificity
// scoring, and validation.
package glob

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// HasWildcard reports whether the pattern uses any doublestar special syntax
// (globs, character classes, or brace alternation). Patterns without special
// syntax are treated as literals that also match as directory prefixes.
func HasWildcard(p string) bool {
	return strings.ContainsAny(p, "*?[]{}")
}

// MatchPattern checks whether pattern matches rel. For wildcard patterns it
// uses doublestar.PathMatchUnvalidated. For plain (non-wildcard) patterns it
// also matches as a directory prefix — e.g. pattern "cmd" matches "cmd/main.go".
func MatchPattern(pattern, rel string) bool {
	// doublestar only treats "**" as a real globstar when it is followed by the
	// platform separator ("/" on Unix, "\\" on Windows). Our patterns and rels
	// are always slash-normalized, so on Windows we convert both sides to
	// backslash form before asking doublestar to match, otherwise "**" degrades
	// to a plain "*" ("bash-like") and globstar patterns silently stop matching.
	p, n := pattern, rel
	if filepath.Separator == '\\' {
		p = strings.ReplaceAll(p, "/", "\\")
		n = strings.ReplaceAll(n, "/", "\\")
	}
	if doublestar.PathMatchUnvalidated(p, n) {
		return true
	}
	if !HasWildcard(pattern) {
		dirPrefix := strings.TrimSuffix(pattern, "/") + "/"
		return strings.HasPrefix(rel, dirPrefix)
	}
	return false
}

// MatchAny reports whether any of the patterns matches rel.
func MatchAny(patterns []string, rel string) bool {
	for _, p := range patterns {
		if p == "" {
			continue
		}
		if MatchPattern(p, rel) {
			return true
		}
	}
	return false
}

// Specificity scores a pattern by its number of literal (non-wildcard)
// characters. More literals == more specific.
func Specificity(p string) int {
	literals := 0
	for _, c := range p {
		if c != '*' && c != '?' {
			literals++
		}
	}
	return literals
}

// IncludeMoreSpecific reports whether any include pattern matching rel is
// more specific than every exclude pattern matching rel. An exact include
// (no wildcards) that matched rel always wins.
func IncludeMoreSpecific(includes, excludes []string, rel string) bool {
	for _, inc := range includes {
		if inc == "" {
			continue
		}
		if !MatchPattern(inc, rel) {
			continue
		}
		// Exact file match (no wildcards, PathMatch succeeded) always wins.
		if !HasWildcard(inc) && doublestar.PathMatchUnvalidated(inc, rel) {
			return true
		}
		for _, exc := range excludes {
			if exc == "" {
				continue
			}
			if !MatchPattern(exc, rel) {
				continue
			}
			if Specificity(inc) > Specificity(exc) {
				return true
			}
		}
	}
	return false
}

// Excluded applies include-priority rules and reports whether rel should be
// skipped: files not matched by any include are skipped when excluded, and
// files matched by both an include and an exclude are kept only when some
// include pattern is more specific than the matching exclude(s).
func Excluded(includes, excludes []string, rel string) bool {
	matchedInclude := MatchAny(includes, rel)
	matchedExclude := MatchAny(excludes, rel)

	if !matchedInclude {
		return matchedExclude
	}
	if !matchedExclude {
		return false
	}
	return !IncludeMoreSpecific(includes, excludes, rel)
}

// Validate checks every pattern for doublestar syntax errors. Invalid
// patterns are rejected up front instead of silently matching nothing.
func Validate(patterns []string) error {
	for _, p := range patterns {
		if p == "" {
			continue
		}
		if !doublestar.ValidatePattern(p) {
			return fmt.Errorf("invalid glob pattern %q", p)
		}
	}
	return nil
}
