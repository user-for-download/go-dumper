package walker

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func ExpandPatterns(patterns []string) ([]string, error) {
	return expandAtFiles(patterns)
}

func expandAtFiles(patterns []string) ([]string, error) {
	var out []string
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
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
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}
