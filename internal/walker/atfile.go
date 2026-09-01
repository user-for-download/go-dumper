package walker

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ResolveNegations moves patterns prefixed with "!" between the include and
// exclude lists: "!pattern" among the includes becomes an exclude, and
// "!pattern" among the excludes becomes an include. This implements the
// negation syntax documented for pattern files. The operation is idempotent.
func ResolveNegations(includes, excludes []string) ([]string, []string) {
	outInc := make([]string, 0, len(includes)+len(excludes))
	outExc := make([]string, 0, len(includes)+len(excludes))
	for _, p := range includes {
		if n, ok := negate(p); ok {
			outExc = append(outExc, n)
		} else {
			outInc = append(outInc, p)
		}
	}
	for _, p := range excludes {
		if n, ok := negate(p); ok {
			outInc = append(outInc, n)
		} else {
			outExc = append(outExc, p)
		}
	}
	return outInc, outExc
}

func negate(p string) (string, bool) {
	if strings.HasPrefix(p, "!") && len(p) > 1 {
		return p[1:], true
	}
	return p, false
}

func ExpandPatterns(patterns []string) ([]string, error) {
	return expandAtFiles(patterns)
}

func expandAtFiles(patterns []string) ([]string, error) {
	var out []string
	for _, p := range patterns {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if !strings.HasPrefix(p, "@") {
			out = append(out, p)
			continue
		}
		path := strings.TrimPrefix(p, "@")
		lines, err := readPatternFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", path, err)
		}
		out = append(out, lines...)
	}
	return out, nil
}

func readPatternFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(strings.TrimLeft(line, " \t"), "#") {
			continue
		}
		lines = append(lines, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}
