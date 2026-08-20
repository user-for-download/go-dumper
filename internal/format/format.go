package format

import (
	"fmt"
	"strings"
)

type Formatter interface {
	Preamble() string
	Postamble() string
	TreeBlock(tree string) string
	FileHeader(relPath string) string
	FileFooter(relPath string) string
}

func New(name string) (Formatter, error) {
	switch strings.ToLower(name) {
	case "", "plain":
		return plainFmt{}, nil
	case "markdown", "md":
		return markdownFmt{}, nil
	default:
		return nil, fmt.Errorf("invalid format %q: use plain or markdown", name)
	}
}

type plainFmt struct{}

func (plainFmt) Preamble() string             { return "" }
func (plainFmt) Postamble() string            { return "" }
func (plainFmt) TreeBlock(t string) string    { return "===== PROJECT TREE =====\n" + t + "\n" }
func (plainFmt) FileHeader(rel string) string { return "\n===== FILE: " + rel + " =====\n" }
func (plainFmt) FileFooter(string) string     { return "" }

type markdownFmt struct{}

func (markdownFmt) Preamble() string  { return "# Project Dump\n\n" }
func (markdownFmt) Postamble() string { return "" }
func (markdownFmt) TreeBlock(t string) string {
	return "## Project Tree\n\n```\n" + t + "```\n\n"
}
func (markdownFmt) FileHeader(rel string) string {
	return "\n## `" + rel + "`\n\n```\n"
}
func (markdownFmt) FileFooter(string) string { return "```\n" }
