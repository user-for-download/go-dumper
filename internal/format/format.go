package format

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Formatter interface {
	Preamble() string
	Postamble() string
	TreeBlock(tree string) string
	FileHeader(relPath string) string
	FileFooter(relPath string) string
	EscapeBody(s string) string
}

func New(name string) (Formatter, error) {
	switch strings.ToLower(name) {
	case "", "plain":
		return plainFmt{}, nil
	case "markdown", "md":
		return markdownFmt{}, nil
	case "xml":
		return xmlFmt{}, nil
	default:
		return nil, fmt.Errorf("invalid format %q: use plain, markdown, or xml", name)
	}
}

type plainFmt struct{}

func (plainFmt) Preamble() string             { return "" }
func (plainFmt) Postamble() string            { return "" }
func (plainFmt) TreeBlock(t string) string    { return "===== PROJECT TREE =====\n" + t + "\n" }
func (plainFmt) FileHeader(rel string) string { return "\n===== FILE: " + rel + " =====\n" }
func (plainFmt) FileFooter(string) string     { return "" }
func (plainFmt) EscapeBody(s string) string   { return s }

type markdownFmt struct{}

func (markdownFmt) Preamble() string  { return "# Project Dump\n\n" }
func (markdownFmt) Postamble() string { return "" }
func (markdownFmt) TreeBlock(t string) string {
	return "## Project Tree\n\n```\n" + t + "```\n\n"
}
func (markdownFmt) FileHeader(rel string) string {
	return "\n## `" + rel + "`\n\n```" + langFromPath(rel) + "\n"
}
func (markdownFmt) FileFooter(string) string   { return "```\n" }
func (markdownFmt) EscapeBody(s string) string { return s }

type xmlFmt struct{}

func (xmlFmt) Preamble() string          { return "<dump>\n" }
func (xmlFmt) Postamble() string         { return "</dump>\n" }
func (xmlFmt) TreeBlock(t string) string { return "<tree><![CDATA[\n" + t + "]]></tree>\n" }
func (xmlFmt) FileHeader(rel string) string {
	return "<file path=\"" + xmlEscape(rel) + "\"><![CDATA[\n"
}
func (xmlFmt) FileFooter(string) string { return "]]></file>\n" }
func (xmlFmt) EscapeBody(s string) string {
	// Escape ]]> inside CDATA sections using the standard CDATA escaping trick:
	// split the CDATA at each ]]> so the content round-trips safely.
	return strings.ReplaceAll(s, "]]>", "]]]]><![CDATA[")
}

func xmlEscape(s string) string {
	r := strings.NewReplacer(`&`, `&amp;`, `<`, `&lt;`, `>`, `&gt;`, `"`, `&quot;`)
	return r.Replace(s)
}

func langFromPath(rel string) string {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".sh":
		return "bash"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".c", ".h":
		return "c"
	case ".cpp", ".hpp":
		return "cpp"
	case ".rb":
		return "ruby"
	case ".sql":
		return "sql"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	case ".xml":
		return "xml"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".json":
		return "json"
	case ".md":
		return "markdown"
	default:
		return ""
	}
}
