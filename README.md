# dumper

A Go CLI tool that concatenates a project's text files into chunked output files, suitable for feeding into LLMs, code review, or archival.

```
$ dumper --path ./myproject --output ./dump_out
```

## Features

- **Glob-based filtering** — include/exclude files using doublestar patterns (`**/*.go`, `vendor/**`, etc.)
- **`@file` syntax** — load patterns from a file, one per line
- **Rune-aware chunking** — respects Unicode codepoints (not bytes) for LLM context windows
- **Comment stripping** — removes line and block comments (token-unaware; use with caution)
- **Output formats** — `plain`, `markdown`, or `xml`
- **Project tree** — optionally prepend an ASCII tree of your project, either full (`--tree-mode full`) or filtered to match what was dumped (`--tree-mode include`)
- **Auto-exclusions** — the output directory and the dumper binary itself are always excluded
- **Concurrent reads** — optional parallelism for large projects
- **Stats export** — JSON summary of processed/skipped files, timing, and chunk counts

## Installation

```bash
git clone https://github.com/user-for-download/go-dumper.git
cd dumper
go build -o dumper ./cmd/dumper
```

Or install globally:

```bash
go install ./cmd/dumper
```

## Quick Start

```bash
# Create a default config
./dumper init

# Run with defaults (current directory, ./dump_out)
./dumper

# Dry run — list files without writing
./dumper --dry-run

# Verbose output with stats
./dumper --verbose

# Custom include/exclude
./dumper --path ./src --include "**/*.go" --exclude "vendor/**"

# Markdown output, include project tree (filtered to match dumped files)
./dumper --format markdown --tree --tree-mode include

# Strip comments (line and block)
./dumper --clear --clear-mode line_and_block

# Parallel reading
./dumper --concurrency 4
```

## Config File

`prc.json` (or any JSON config via `--config`):

```json
{
  "path": ".",
  "output": "./dump_out",
  "include": ["**/*"],
  "exclude": [".git/**", "dump_out/**", "node_modules/**", "vendor/**"],
  "max_symbols": 1000000,
  "chunk_prefix": "dump",
  "split_long_lines": true,
  "progress": true,
  "stats_file": "dump_out/stats.json",
  "include_hidden": false,
  "concurrency": 1,
  "exclude_self": true,
  "format": "markdown",
  "clear": {
    "enabled": false,
    "mode": "line"
  },
  "tree": {
    "enabled": true,
    "format": "ascii",
    "max_depth": 0,
    "include_sizes": true,
    "mode": "include"
  }
}
```

### Config Precedence

JSON defaults → config file → CLI flags. Each level overrides the previous.

## CLI Flags

| Flag | Description |
|---|---|
| `-p, --path` | Project root (default: `.`) |
| `-o, --output` | Output directory (default: `./dump_out`) |
| `-i, --include` | Include patterns (doublestar globs) |
| `-x, --exclude` | Exclude patterns |
| `-m, --max-symbols` | Max runes per chunk (default: 1,000,000) |
| `--chunk-prefix` | Output file prefix (default: `dump`) |
| `--split-long-lines` | Split lines that exceed `max-symbols` |
| `-c, --config` | Config file path (default: `prc.json`) |
| `--tree` | Prepend project tree to output |
| `--tree-mode full\|include` | `full` = all files; `include` = only matched files |
| `--tree-depth` | Max tree depth (0 = unlimited) |
| `--format plain\|markdown\|xml` | Output format |
| `--clear` | Strip comments |
| `--clear-mode line\|line_and_block` | Comment stripping mode |
| `--concurrency N` | Parallel readers (default: 1) |
| `--exclude-self` | Auto-skip the dumper binary (default: true) |
| `--include-hidden` | Include dotfiles/dirs (default: false) |
| `--progress` | Show progress bar |
| `--stats-file` | Write stats JSON |
| `--dry-run` | List files without writing |
| `-v, --verbose` | Verbose logging |

## Pattern Files

Any `--include` or `--exclude` value that starts with `@` is treated as a path to a text file containing patterns, one per line:

```
# patterns.txt
**/*.go
**/*.md
!**/vendor/**
```

```bash
./dumper --include "@patterns.txt"
```

Lines starting with `#` and blank lines are ignored.

## Comment Stripping

The comment stripper is token-unaware. It may corrupt strings that contain comment markers (e.g., `"https://example.com"`). For accurate parsing, use language-specific parsers. **Keep `clear.enabled: false` for sensitive code.**

## Output Format Examples

### Plain (default)

```
===== FILE: src/main.go =====

package main

func main() {}

===== FILE: README.md =====

# My Project
```

### Markdown

```markdown
# Project Dump

## `src/main.go`

```go
package main
func main() {}
```

## `README.md`

# My Project
```

### XML

```xml
<dump>
<file path="src/main.go"><![CDATA[
package main
func main() {}
]]></file>
<file path="README.md"><![CDATA[
# My Project
]]></file>
</dump>
```

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed design documentation.

## Testing

```bash
go test ./...
go vet ./...
go build -o dumper ./cmd/dumper
```

## License

MIT