package cleaner

import (
	"strings"
	"testing"
)

func collect(t *testing.T, src, ext string, mode Mode) string {
	t.Helper()
	var sb strings.Builder
	err := Stream(strings.NewReader(src), ext, mode, func(line string) error {
		sb.WriteString(line)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

func TestCleaner_OffKeepsAll(t *testing.T) {
	src := "// hi\nfoo\n"
	got := collect(t, src, ".go", ModeOff)
	if got != src {
		t.Errorf("ModeOff must preserve content; got %q", got)
	}
}

func TestCleaner_LineMode_Go(t *testing.T) {
	src := "// header\npackage main\n  // indented\nfunc f(){}\n"
	got := collect(t, src, ".go", ModeLine)
	want := "package main\nfunc f(){}\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCleaner_LineMode_Python(t *testing.T) {
	src := "# top\nimport os\n   # mid\nx = 1\n"
	got := collect(t, src, ".py", ModeLine)
	want := "import os\nx = 1\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCleaner_LineMode_DoesNotRemoveBlocks(t *testing.T) {
	src := "/* block */\nfoo\n"
	got := collect(t, src, ".go", ModeLine)
	if !strings.Contains(got, "/* block */") {
		t.Errorf("ModeLine must not strip block comments; got %q", got)
	}
}

func TestCleaner_BlockMode_SingleLine(t *testing.T) {
	src := "/* single */\nfoo\n"
	got := collect(t, src, ".go", ModeLineAndBlock)
	want := "foo\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCleaner_BlockMode_MultiLine(t *testing.T) {
	src := "/*\n license\n line\n*/\npackage main\n"
	got := collect(t, src, ".go", ModeLineAndBlock)
	want := "package main\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCleaner_BlockMode_HTML(t *testing.T) {
	src := "<!--\n hidden\n-->\n<p>visible</p>\n"
	got := collect(t, src, ".html", ModeLineAndBlock)
	want := "<p>visible</p>\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCleaner_UnknownExtension_PassThrough(t *testing.T) {
	src := "// looks like a comment\nfoo\n"
	got := collect(t, src, ".unknownext", ModeLineAndBlock)
	if got != src {
		t.Errorf("unknown ext must pass through; got %q", got)
	}
}

func TestCleaner_BlockMode_JavaScript(t *testing.T) {
	src := "// line comment\nconst x = 1;\n/* block\ncomment */\nconst y = 2;\n"
	got := collect(t, src, ".js", ModeLineAndBlock)
	want := "const x = 1;\nconst y = 2;\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCleaner_BlockMode_CSS(t *testing.T) {
	src := "/* comment */ .class { color: red; }\n"
	got := collect(t, src, ".css", ModeLineAndBlock)
	want := " .class { color: red; }\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCleaner_BlockMode_SQL(t *testing.T) {
	src := "-- single line\nSELECT * FROM t;\n/* block\ncomment */\nINSERT INTO t VALUES(1);\n"
	got := collect(t, src, ".sql", ModeLineAndBlock)
	want := "SELECT * FROM t;\nINSERT INTO t VALUES(1);\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCleaner_BlockMode_Rust(t *testing.T) {
	src := "// line\nfn main() {}\n/* block\ncomment */\nconst X = 1;\n"
	got := collect(t, src, ".rs", ModeLineAndBlock)
	want := "fn main() {}\nconst X = 1;\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCleaner_ShellComments(t *testing.T) {
	src := "#!/bin/bash\n# comment\necho hello\n"
	got := collect(t, src, ".sh", ModeLine)
	want := "#!/bin/bash\necho hello\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCleaner_IniComments(t *testing.T) {
	src := "; comment\n[section]\nkey=value\n"
	got := collect(t, src, ".ini", ModeLine)
	want := "[section]\nkey=value\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCleaner_YamlComments(t *testing.T) {
	src := "# comment\nkey: value\n# another\n"
	got := collect(t, src, ".yaml", ModeLine)
	want := "key: value\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCleaner_BlockMode_ClosesMidLineWithCodeAfter(t *testing.T) {
	src := "foo /* block end */ bar\nbaz\n"
	got := collect(t, src, ".go", ModeLineAndBlock)
	want := "foo  bar\nbaz\n"
	if got != want {
		t.Errorf("block closing mid-line with code after: got %q, want %q", got, want)
	}
}

func TestCleaner_BlockMode_MidLineCloseNoTrailingContent(t *testing.T) {
	src := "a /* comment */ b\n"
	got := collect(t, src, ".go", ModeLineAndBlock)
	want := "a  b\n"
	if got != want {
		t.Errorf("single-line block with code after: got %q, want %q", got, want)
	}
}

func TestCleaner_StripsRealInlineComment(t *testing.T) {
	src := "package main\nvar x = 1 // remove this\n"
	got := collect(t, src, ".go", ModeLine)
	if strings.Contains(got, "remove this") {
		t.Errorf("real comment was not removed: got %q", got)
	}
	if !strings.Contains(got, "var x = 1") {
		t.Errorf("code was removed: got %q", got)
	}
}
