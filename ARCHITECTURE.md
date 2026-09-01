# Architecture

This document describes the internal structure of `dumper`.

## Data Flow

```
CLI (main.go)
  └─> config.Load() + MergeCLI() + Validate()
       └─> app.Run(cfg)
            ├─> walker.ExpandPatterns()  — @file expansion
            ├─> walker.ResolveNegations() — "!pattern" include/exclude flipping
            ├─> EffectiveExcludes()      — output dir + dumper binary auto-excludes
            │
            ├─> walker.New() + Collect() — filesystem traversal, pattern validation
            │
            ├─> binary detection (util.SniffBinary, pre-pass)
            │   └─> binary/read-error files → stats; never reach the pipeline
            │
            ├─> tree.Generate()          — generated BEFORE the output dir or any
            │   │                          chunk file exists, so it never lists them
            │   └─> include mode uses only the (text) files that will be dumped
            │
            ├─> chunker.New()            — partial output is abandoned (deleted)
            │                              if the run fails
            ├─> for each text file:
            │   ├─> renderFile()         — streams complete lines
            │   └─> chunker.WriteString  — rotates chunks at line boundaries
            │
            └─> stats.WriteJSON()        — optional stats export (validated early)
```

## Package Overview

```
cmd/dumper/            CLI entry point
internal/app/          Orchestration: pipeline wiring
internal/config/       Config loading, defaults, CLI overrides, validation
internal/glob/         Shared doublestar matching, priority, validation
internal/walker/       Filesystem traversal with glob filtering
internal/chunker/      Rune-aware output file splitting
internal/format/       Output formatters (plain, markdown)
internal/tree/         ASCII project tree generation
internal/util/         Binary file detection
internal/stats/        Thread-safe counters + JSON export
internal/progress/     mpb-based progress bar
```

## Key Design Decisions

### Line-Based, Rune-Correct Chunking

`renderFile` streams files line by line, and `MaxSymbols` counts Unicode
codepoints (runes), not bytes. Because the chunker only ever rotates chunks
at line boundaries, every chunk is valid UTF-8 and its rune count is exact.
An oversized single line is split on rune boundaries when
`split_long_lines` is enabled, and rejected with a clear error when it is
not. A missing trailing newline at EOF is added so file content never runs
into the next file header or markdown fence.

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
2. **Glob phase** — explicitly named hidden files (e.g., `/path/to/.env`, even inside a hidden directory) are allowed regardless of `IncludeHidden`, so an explicit `--include` of a hidden file always works. Wildcard patterns still respect the `IncludeHidden` flag.

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
- `format` must be one of: `plain`, `markdown`
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

### Format Plug

`format.Formatter` is an interface with two implementations:
- `plainFmt` — `===== FILE: path =====` markers
- `markdownFmt` — raw fenced code blocks without language detection

`format.New()` validates the format name and returns an error for unknown values.

### Stats Handling

Stats are initialized with `stats.New()` at the start of `app.Run()`. A deferred function ensures `st.Finish(0)` is called on any early error path, preventing garbage values from `DurationSec()`. On successful completion, `st.Finish(ch.ChunkCount())` is called with a boolean flag to prevent the defer from overwriting the duration. A `stats_file` that would overwrite one of the chunk files (e.g. `dump_00001.txt`) is rejected before any output is written.

### Failure Cleanup

If a run fails after the chunker was created (write errors, oversized content with splitting disabled, close errors), `chunker.Abandon()` removes every chunk file the run created, so a broken dump is never left on disk looking like a complete one.

### Tree Full Mode

`tree.mode: "full"` shows every file in the project **except excluded ones** — configured excludes plus the auto-excluded output directory and dumper binary are always honored, so the tree never advertises files that will never be dumped. The tree is also generated before the output directory or any chunk file exists.

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
3. Later, `renderFile()` opens the file from scratch, copies raw bytes to the formatter/chunker, and defers the close.
4. This ensures only a controlled number of files (bounded by the `--concurrency` setting) are ever held open simultaneously.

## Testing Strategy

- **Unit tests** for: chunker (rune counting, rotation, splitting, abandon), walker (glob matching and directory pruning, invalid patterns, negation, hidden dirs), tree (ModeFull/ModeInclude, root naming, excludes), glob (validation, include priority), config (validation)
- **E2E tests** (`app_e2e_test.go`): full pipeline including reassembly fidelity (chunked output joined must equal canonical single-chunk output), binary skipping, concurrency ordering, tree content, output directory auto-exclusion
- **Regression tests** cover the harder failure modes: multi-block files with small `max_symbols`, multi-byte files spanning chunk boundaries (all chunks must be valid UTF-8, serial and concurrent), markdown fence termination, failed-run cleanup, stats-file collisions
- Re-assembly invariant: two runs with `MaxSymbols=50` (split) and `MaxSymbols=1_000_000` (canonical) must produce byte-identical joined output

## Known Limitations

1. **No deduplication** — identical files (by path) could appear twice if the filesystem presents them twice
2. **Tree generation is not concurrency-safe with walker errors** — tree independently re-walks the filesystem; if files change between walks, tree and content may disagree
3. **`Concurrency > 1` is not byte-for-byte reproducible** — parallel reads have non-deterministic timing; use `--concurrency 1` for reproducible output
