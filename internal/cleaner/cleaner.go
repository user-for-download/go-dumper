package cleaner

import (
	"bufio"
	"io"
	"strings"
)

type Mode int

const (
	ModeOff Mode = iota
	ModeLine
	ModeLineAndBlock
)

type langSpec struct {
	line   []string
	blocks [][2]string
}

type langRules struct {
	lineComment string
	blockOpen   string
	blockClose  string
}

var langs = map[string]langSpec{
	".go":   {line: []string{"//"}, blocks: [][2]string{{"/*", "*/"}}},
	".c":    {line: []string{"//"}, blocks: [][2]string{{"/*", "*/"}}},
	".h":    {line: []string{"//"}, blocks: [][2]string{{"/*", "*/"}}},
	".cpp":  {line: []string{"//"}, blocks: [][2]string{{"/*", "*/"}}},
	".java": {line: []string{"//"}, blocks: [][2]string{{"/*", "*/"}}},
	".js":   {line: []string{"//"}, blocks: [][2]string{{"/*", "*/"}}},
	".ts":   {line: []string{"//"}, blocks: [][2]string{{"/*", "*/"}}},
	".rs":   {line: []string{"//"}, blocks: [][2]string{{"/*", "*/"}}},
	".css":  {blocks: [][2]string{{"/*", "*/"}}},
	".html": {blocks: [][2]string{{"<!--", "-->"}}},
	".xml":  {blocks: [][2]string{{"<!--", "-->"}}},
	".htm":  {blocks: [][2]string{{"<!--", "-->"}}},
	".py":   {line: []string{"#"}},
	".sh":   {line: []string{"#"}},
	".rb":   {line: []string{"#"}},
	".sql":  {line: []string{"--"}, blocks: [][2]string{{"/*", "*/"}}},
	".ini":  {line: []string{";"}},
	".yaml": {line: []string{"#"}},
	".yml":  {line: []string{"#"}},
	".toml": {line: []string{"#"}},
	".mk":   {line: []string{"#"}},
}

func rulesFor(spec langSpec, mode Mode) langRules {
	r := langRules{}
	if mode == ModeLine || mode == ModeLineAndBlock {
		if len(spec.line) > 0 {
			r.lineComment = spec.line[0]
		}
	}
	if mode == ModeLineAndBlock && len(spec.blocks) > 0 {
		r.blockOpen = spec.blocks[0][0]
		r.blockClose = spec.blocks[0][1]
	}
	return r
}

func stripLine(line string, r langRules, inBlock bool) (string, bool) {
	nl := ""
	if strings.HasSuffix(line, "\n") {
		nl = "\n"
		line = line[:len(line)-1]
	}

	if inBlock && strings.TrimSpace(line) == "" {
		return "", true
	}

	trimmed := strings.TrimSpace(line)

	var out strings.Builder
	out.Grow(len(line))
	i := 0

	for i < len(line) {
		if inBlock {
			if r.blockClose == "" {
				i = len(line)
				break
			}
			idx := strings.Index(line[i:], r.blockClose)
			if idx < 0 {
				i = len(line)
				break
			}
			i += idx + len(r.blockClose)
			inBlock = false
			continue
		}

		if strings.HasPrefix(trimmed, "#!") && i == 0 {
			out.WriteString(line)
			i = len(line)
			continue
		}

		lineIdx := -1
		if r.lineComment != "" {
			if j := strings.Index(line[i:], r.lineComment); j >= 0 {
				lineIdx = i + j
			}
		}
		blockIdx := -1
		if r.blockOpen != "" {
			if j := strings.Index(line[i:], r.blockOpen); j >= 0 {
				blockIdx = i + j
			}
		}

		switch {
		case lineIdx == -1 && blockIdx == -1:
			out.WriteString(line[i:])
			i = len(line)

		case lineIdx != -1 && (blockIdx == -1 || lineIdx < blockIdx):
			out.WriteString(line[i:lineIdx])
			i = len(line)

		default:
			out.WriteString(line[i:blockIdx])
			i = blockIdx + len(r.blockOpen)
			inBlock = true
		}
	}

	cleaned := out.String()
	if len(cleaned) < len(line) {
		cleaned = strings.TrimRight(cleaned, " \t")
	}

	if strings.TrimSpace(cleaned) == "" && strings.TrimSpace(line) != "" {
		return "", inBlock
	}

	return cleaned + nl, inBlock
}

func Stream(r io.Reader, ext string, mode Mode, emit func(string) error) error {
	if mode == ModeOff {
		return copyLines(r, emit)
	}
	spec, hasSpec := langs[strings.ToLower(ext)]
	if !hasSpec {
		return copyLines(r, emit)
	}
	rule := rulesFor(spec, mode)

	br := bufio.NewReaderSize(r, 64*1024)
	inBlock := false

	for {
		line, rerr := br.ReadString('\n')
		if len(line) > 0 {
			cleaned, nb := stripLine(line, rule, inBlock)
			inBlock = nb
			if cleaned != "" {
				if err := emit(cleaned); err != nil {
					return err
				}
			}
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

func copyLines(r io.Reader, emit func(string) error) error {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			if err := emit(line); err != nil {
				return err
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
