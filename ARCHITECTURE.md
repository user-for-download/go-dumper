# Architecture

This document describes the internal structure of `dumper`.

## Package Overview

```
cmd/dumper/            CLI entry point
internal/app/          Orchestration: pipeline wiring
internal/config/        Config loading, defaults, CLI overrides, validation
internal/walker/       Filesystem traversal with glob filtering
internal/chunker/      Rune-aware output file splitting
internal/cleaner/      Language-aware comment stripping
internal/format/       Output formatters (plain, markdown, xml)
internal/tree/         ASCII project tree generation
internal/util/         Binary file detection
internal/stats/        Thread-safe counters + JSON export
internal/progress/     mpb-based progress bar
```

## Data Flow

```
CLI (main.go)
  └─> config.Load() + MergeCLI() + Validate()
       └─> app.Run(cfg)
            ├─> walker.Collect()     — filesystem traversal
            │   ├─> walker.ExpandPatterns() — @file expansion
            │   ├─> autoExcludeOutput()   — exclude output dir
            │   └─> autoExcludeSelf()      — exclude dumper binary
            │
            ├─> binary detection (util.SniffBinary)
            │   └─> skip binary files → stats
            │
            ├─> format.New()        — select output formatter
            │
            ├─> tree.Generate()     — optional project tree
            │   └─> uses same includes/excludes for ModeInclude filtering
            │
            ├─> for each text file:
            │   ├─> renderFile()     — header + content + footer
            │   ├─> cleaner.Stream() — optional comment stripping
            │   └─> chunker.WriteString/WriteBytes — split into chunks
            │
            └─> stats.WriteJSON()  — optional stats export
```

## Key Design Decisions

### Rune-Correct Chunking

`MaxSymbols` counts Unicode codepoints (runes), not bytes. This is correct for LLM context windows which count tokens by characters/codepoints. The chunker converts to `[]rune` before slicing:

```go
runes := []rune(s)
piece := string(runes[i:end])
```

### Output Order Preservation in Concurrent Mode

`RunConcurrent` uses a reorder buffer (`pending map[int]result`) keyed by input index. Results are drained in order once their turn comes, guaranteeing output file order matches input order regardless of read completion order.

### Absolute Path Handling (`toRel`)

When `--include` specifies an absolute path outside the scan root (e.g., `--include /home/user/other/.env`), the walker still resolves it via `doublestar.FilepathGlob`. The `app.toRel()` helper bridges the absolute path back to the output formatter:

```go
func toRel(root, path string) string {
    absRoot, _ := filepath.Abs(root)
    absPath, _ := filepath.Abs(path)
    rel, err := filepath.Rel(absRoot, absPath)
    if err != nil {
        return filepath.ToSlash(path)  // fallback to original
    }
    return filepath.ToSlash(rel)
}
```

This is used in both `ProcessFile` (serial) and `RunConcurrent` (parallel) to ensure file headers show a sensible relative path even when the file lives outside the project root.

### Explicit Config Error

`config.Load(path, explicit bool)` takes an `explicit` parameter. When `true` and the config file does not exist, it returns a hard error (`"config file %q not found"`) instead of silently falling back to defaults. This is triggered in `main.go` via `c.Flags().Changed("config")`.

### Hidden File Glob Phase

The walker's `Collect()` has two phases:

1. **WalkDir phase** — respects `IncludeHidden` flag; skips dotfiles/dirs when false
2. **Glob phase** — explicitly named hidden files (e.g., `/path/to/.env`) are allowed regardless of `IncludeHidden`, so an explicit `--include` of a hidden file always works

### Auto-Exclusions

Three automatic exclusions are applied:

1. **Output directory** — `autoExcludeOutput()` uses `filepath.Rel` to compute the output path relative to the root. If it resolves to a child path (not `..` or absolute), it's added as `<out>/**`.
2. **Dumper binary** — `autoExcludeSelf()` uses `os.Executable()` to find the running binary and adds it to excludes if it's under the scan root.
3. **Effective excludes** — Both are combined in `app.EffectiveExcludes()` which is used by both `app.Run()` and `--dry-run`.

### Include Priority Over Exclude

When both `--include` and `--exclude` patterns match a file, the include pattern may win if it's more specific. The walker uses pattern specificity scoring:

- Explicit file paths (no wildcards) always win over any exclude pattern
- Otherwise, patterns with more literal characters (non-wildcard) win over patterns with more wildcards

This allows `include: ["deploy/.env.example"]` to work even when `exclude: ["deploy/**"]` is present.

### Type Filter

The `--type` flag filters files by extension. When set (e.g., `--type go`), only files with that exact extension are included. The filter is applied in both the walker and tree generation for consistency.

### Clean Output Folder

The `--clean` flag removes the output directory before writing. This is useful for ensuring a fresh start without manual cleanup.

### Config Validation

`Config.Validate()` enforces invariants after loading and merging:

- `path` and `output` must not be empty
- `output` must not be the same as `path` (using `filepath.Abs` comparison)
- `max_symbols` must be > 0
- `concurrency` must be >= 1
- `format` must be one of: `plain`, `markdown`, `xml`
- `clear.mode` must be one of: `line`, `line_and_block`
- `tree.mode` must be one of: `full`, `include`
- `tree.max_depth` must be >= 0
- `tree.format` must be `ascii`

### Tree Include Mode

When `tree.mode: "include"`, the tree generation uses the same include/exclude patterns and Type filter as the walker:

1. `walker.ExpandPatterns()` expands `@file` patterns
2. A `collectAllowed()` pass walks the filesystem and builds a `map[string]struct{}` of slash-relative paths that pass the filters, including the Type filter
3. `writeChildren()` skips any file not in the allowed set, and skips any directory that has no allowed descendants (`hasAllowedDescendant()`)
4. Result: the tree shows exactly the files that will be dumped

### Pattern Expansion

`@file` syntax is expanded in one place: `walker.ExpandPatterns()`. This is called once in `app.Run()` and once in `main.go` (dry-run), producing expanded pattern lists that are passed to both the walker and the tree generator. This ensures tree include mode sees the same effective patterns as the walker.

### Comment Stripping

The cleaner (`stripLine()`) is a single-pass state machine per line. It handles:
- Shebang lines (`#!`) — preserved
- Line comments (`//`, `#`, `--`, etc.) — truncated
- Block comments (`/* */`, `<!-- -->`) — removed, with mid-line close + code-after handling
- Multi-line block comment spans
- Blank lines inside block comments — suppressed (not leaked to output)

The cleaner is **token-unaware** — it does not track whether `//` or `/*` appears inside a string literal. This is documented and acceptable for a dumper tool. Future work: integrate language-specific parsers (e.g., `go/scanner` for Go) for `--clear-mode=ast`.

### Format Plug

`format.Formatter` is an interface with three implementations:
- `plainFmt` — `===== FILE: path =====` markers
- `markdownFmt` — fenced code blocks with language hints from `langFromPath()`
- `xmlFmt` — `<file path="..."><![CDATA[...]]></file>`

`format.New()` validates the format name and returns an error for unknown values.

### Stats Handling

Stats are initialized with `stats.New()` at the start of `app.Run()`. A deferred function ensures `st.Finish(0)` is called on any early error path, preventing garbage values from `DurationSec()`. On successful completion, `st.Finish(ch.ChunkCount())` is called with a boolean flag to prevent the defer from overwriting the duration.

### Progress Bar Handling

In concurrent mode, when a write error occurs, the worker pool is cancelled. The remaining unprocessed files may never send results (they exit via `ctx.Done()`). After the error, `RunConcurrent` drains the remaining file count by calling `rep.FinishFile()` for each unprocessed file, ensuring the progress bar completes correctly.

## Concurrency Model

- `Concurrency: 1` (default) — serial file reads
- `Concurrency: N` — N goroutines read and render files concurrently
- The reorder buffer ensures output files are written in the same order as the input list
- Worker goroutines are cancelled via `context.WithCancel` if a write error occurs
- The chunker (`Chunker`) has its own mutex — writes are serialized at the chunker level

## File Handle Management

1. `util.SniffBinary()` briefly opens the file, reads up to 8KB to detect binary content or null bytes, and immediately closes it to prevent file descriptor leaks.
2. If text, the file path is added to a processing queue.
3. Later, `renderFile()` opens the file from scratch, streams it through the cleaner (comment stripper), and defers the close.
4. This ensures only a controlled number of files (bounded by the `--concurrency` setting) are ever held open simultaneously.

## Testing Strategy

- **Unit tests** for: chunker (rune counting, rotation, splitting), cleaner (comment removal, edge cases), walker (glob matching, symlink cycle detection), tree (ModeFull/ModeInclude), config (validation)
- **E2E tests** (`app_e2e_test.go`): full pipeline including reassembly fidelity (chunked output joined must equal canonical single-chunk output), binary skipping, concurrency ordering, tree content, output directory auto-exclusion
- Re-assembly invariant: two runs with `MaxSymbols=50` (split) and `MaxSymbols=1_000_000` (canonical) must produce byte-identical joined output

## Known Limitations

1. **Cleaner is token-unaware** — may corrupt strings containing `//` or `/*`
2. **No deduplication** — identical files (by path) could appear twice if the filesystem presents them twice
3. **Tree generation is not concurrency-safe with walker errors** — tree independently re-walks the filesystem; if files change between walks, tree and content may disagree
4. **`Concurrency > 1` is not byte-for-byte reproducible** — parallel reads have non-deterministic timing; use `--concurrency 1` for reproducible output
5. **No symlink cycle detection** — `filepath.WalkDir` does not follow symlinks; cycles via symlinks are not detected