package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/user-for-download/go-dumper/internal/chunker"
	"github.com/user-for-download/go-dumper/internal/config"
	"github.com/user-for-download/go-dumper/internal/format"
	"github.com/user-for-download/go-dumper/internal/progress"
	"github.com/user-for-download/go-dumper/internal/stats"
	"github.com/user-for-download/go-dumper/internal/tree"
	"github.com/user-for-download/go-dumper/internal/util"
	"github.com/user-for-download/go-dumper/internal/walker"
)

// ToRel renders path relative to root (slash-separated). Files outside the
// root fall back to their original path.
func ToRel(root, path string) string {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func Run(cfg *config.Config) (*stats.Stats, error) {
	st := stats.New()
	finished := false
	defer func() {
		if !finished {
			st.Finish(0)
		}
	}()

	if cfg.Clean {
		if outputContainsRoot(cfg.Path, cfg.Output) {
			return st, errors.New("refusing to clean an output directory that contains path")
		}
		if err := os.RemoveAll(cfg.Output); err != nil {
			return st, fmt.Errorf("clean output: %w", err)
		}
	}

	includes, err := walker.ExpandPatterns(cfg.Include)
	if err != nil {
		return st, fmt.Errorf("include patterns: %w", err)
	}

	excludes := EffectiveExcludes(cfg.Path, cfg.Output, cfg.Exclude, cfg.ExcludeSelf)

	excludes, err = walker.ExpandPatterns(excludes)
	if err != nil {
		return st, fmt.Errorf("exclude patterns: %w", err)
	}

	// Resolve "!pattern" negations once so the walker and the tree generator
	// see identical effective include/exclude lists.
	includes, excludes = walker.ResolveNegations(includes, excludes)

	w, err := walker.New(walker.Options{
		Root:          cfg.Path,
		Includes:      includes,
		Excludes:      excludes,
		Type:          cfg.Type,
		IncludeHidden: cfg.IncludeHidden,
	})
	if err != nil {
		return st, err
	}
	entries, err := w.Collect()
	if err != nil {
		return st, err
	}
	for _, e := range w.Errors() {
		st.AddError("walk: " + e.Error())
	}

	st.SetTotalFiles(len(entries))

	// Sniff binary files up front: skipped files never reach the processing
	// queue or the include-mode tree, so the tree mirrors exactly what will
	// be dumped and the progress bar tracks only real work.
	files := make([]sniffedFile, 0, len(entries))
	for _, e := range entries {
		isBin, serr := util.SniffBinary(e.Path)
		if serr != nil {
			st.AddSkipped(e.Path, stats.ReasonError, serr)
			continue
		}
		if isBin {
			st.AddSkipped(e.Path, stats.ReasonBinary, nil)
			continue
		}
		files = append(files, sniffedFile{path: e.Path, size: e.Size})
	}

	// Validate the stats file target before anything is written: a stats file
	// that would overwrite one of the chunk files (dump_00001.txt, ...) is a
	// configuration error and must fail early, not after the dump completes.
	if cfg.StatsFile != "" && statsFileCollidesWithChunk(cfg) {
		return st, errors.New("stats_file must not overwrite a chunk file")
	}

	// Generate the tree before the output directory or any chunk file exists,
	// so the tree can never list the dump output itself.
	var treeStr string
	if cfg.Tree.Enabled {
		treeMode := tree.ModeFull
		if cfg.Tree.Mode == "include" {
			treeMode = tree.ModeInclude
		}
		treeOpts := tree.Options{
			Root:            cfg.Path,
			MaxDepth:        cfg.Tree.MaxDepth,
			IncludeSizes:    cfg.Tree.IncludeSizes,
			IncludeHidden:   cfg.IncludeHidden,
			Mode:            treeMode,
			Includes:        includes,
			Excludes:        excludes,
			Type:            cfg.Type,
			AllowedFilesSet: treeMode == tree.ModeInclude,
		}
		if treeMode == tree.ModeInclude {
			paths := make([]string, len(files))
			for i, f := range files {
				paths[i] = f.path
			}
			treeOpts.AllowedFiles = paths
		}
		ts, terr := tree.Generate(treeOpts)
		if terr != nil {
			st.AddError("tree: " + terr.Error())
		} else {
			treeStr = ts
		}
	}

	if err := os.MkdirAll(cfg.Output, 0o755); err != nil {
		return st, err
	}

	fmtr, err := format.New(cfg.Format)
	if err != nil {
		return st, err
	}

	ext := ".txt"
	switch cfg.Format {
	case "markdown":
		ext = ".md"
	}

	ch, err := chunker.New(chunker.Options{
		OutputDir:      cfg.Output,
		Prefix:         cfg.ChunkPrefix,
		MaxSymbols:     cfg.MaxSymbols,
		SplitLongLines: cfg.SplitLongLines,
		Extension:      ext,
	})
	if err != nil {
		return st, err
	}

	// On any failure the partial chunk files are removed: a failed run must
	// not leave an incomplete dump behind that looks like a complete one.
	dumpComplete := false
	defer func() {
		if !dumpComplete {
			if aerr := ch.Abandon(); aerr != nil {
				st.AddError("cleanup chunks: " + aerr.Error())
			}
		}
	}()

	if pre := fmtr.Preamble(); pre != "" {
		if err := ch.WriteString(pre); err != nil {
			return st, err
		}
	}

	if treeStr != "" {
		if err := ch.WriteString(fmtr.TreeBlock(treeStr)); err != nil {
			return st, err
		}
	}

	rep := progress.New(cfg.Progress, len(files))
	defer rep.Done()

	if cfg.Concurrency > 1 {
		if err := RunConcurrent(files, cfg.Path, ch, fmtr, st, rep, cfg.Concurrency); err != nil {
			return st, err
		}
	} else {
		for _, sf := range files {
			err := ProcessFile(sf, cfg.Path, ch, fmtr, st)
			if errors.Is(err, ErrBinaryFile) {
				st.AddSkipped(sf.path, stats.ReasonBinary, nil)
			} else if err != nil {
				var outputErr *outputError
				if errors.As(err, &outputErr) {
					return st, fmt.Errorf("process %s: %w", sf.path, err)
				}
				st.AddSkipped(sf.path, stats.ReasonError, err)
			}
			rep.FinishFile()
		}
	}

	if post := fmtr.Postamble(); post != "" {
		if err := ch.WriteString(post); err != nil {
			return st, err
		}
	}
	if err := ch.Close(); err != nil {
		return st, fmt.Errorf("close chunks: %w", err)
	}
	dumpComplete = true

	st.Finish(ch.ChunkCount())
	finished = true
	if cfg.StatsFile != "" {
		if err := st.WriteJSON(cfg.StatsFile); err != nil {
			return st, fmt.Errorf("stats: %w", err)
		}
	}
	return st, nil
}

// statsFileCollidesWithChunk reports whether cfg.StatsFile names a file the
// chunker would create (e.g. <output>/dump_00001.txt).
func statsFileCollidesWithChunk(cfg *config.Config) bool {
	absStats, err := canonicalPath(cfg.StatsFile)
	if err != nil {
		return false
	}
	absOut, err := canonicalPath(cfg.Output)
	if err != nil {
		return false
	}
	if filepath.Dir(absStats) != filepath.Clean(absOut) {
		return false
	}
	ext := ".txt"
	if cfg.Format == "markdown" {
		ext = ".md"
	}
	name := filepath.Base(absStats)
	prefix := cfg.ChunkPrefix + "_"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ext) {
		return false
	}
	mid := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ext)
	if mid == "" {
		return false
	}
	for _, r := range mid {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func ProcessFile(sf sniffedFile, root string, ch *chunker.Chunker, fmtr format.Formatter, st *stats.Stats) error {
	f, isBin, err := util.SniffAndRewind(sf.path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if isBin {
		return ErrBinaryFile
	}

	rel := ToRel(root, sf.path)

	if err := ch.WriteString(fmtr.FileHeader(rel)); err != nil {
		return &outputError{err: err}
	}

	var writeErr error
	bytes, runes, err := renderFile(f, func(line string) error {
		writeErr = ch.WriteString(line)
		return writeErr
	})
	if err != nil {
		if writeErr != nil {
			return &outputError{err: writeErr}
		}
		return err
	}

	if footer := fmtr.FileFooter(rel); footer != "" {
		if err := ch.WriteString(footer); err != nil {
			return &outputError{err: err}
		}
	}

	st.IncProcessed(bytes, runes)
	return nil
}
