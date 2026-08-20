package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/user-for-download/go-dumper/internal/chunker"
	"github.com/user-for-download/go-dumper/internal/config"
	"github.com/user-for-download/go-dumper/internal/format"
	"github.com/user-for-download/go-dumper/internal/progress"
	"github.com/user-for-download/go-dumper/internal/stats"
	"github.com/user-for-download/go-dumper/internal/tree"
	"github.com/user-for-download/go-dumper/internal/util"
	"github.com/user-for-download/go-dumper/internal/walker"
)

func toRel(root, path string) string {
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

	files := make([]sniffedFile, len(entries))
	for i, e := range entries {
		files[i] = sniffedFile{path: e.Path, size: e.Size}
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
	defer ch.Close()
	if pre := fmtr.Preamble(); pre != "" {
		if err := ch.WriteString(pre); err != nil {
			return st, err
		}
	}

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
		treeStr, terr := tree.Generate(treeOpts)
		if terr != nil {
			st.AddError("tree: " + terr.Error())
		}
		if treeStr != "" {
			if err := ch.WriteString(fmtr.TreeBlock(treeStr)); err != nil {
				return st, err
			}
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

	st.Finish(ch.ChunkCount())
	finished = true
	if cfg.StatsFile != "" {
		for i := 1; i <= ch.ChunkCount(); i++ {
			chunkPath := filepath.Join(cfg.Output, fmt.Sprintf("%s_%05d%s", cfg.ChunkPrefix, i, ext))
			if sameFilePath(cfg.StatsFile, chunkPath) {
				return st, errors.New("stats_file must not overwrite a chunk file")
			}
		}
		if err := st.WriteJSON(cfg.StatsFile); err != nil {
			return st, fmt.Errorf("stats: %w", err)
		}
	}
	return st, nil
}

func sameFilePath(a, b string) bool {
	absA, errA := canonicalPath(a)
	absB, errB := canonicalPath(b)
	return errA == nil && errB == nil && filepath.Clean(absA) == filepath.Clean(absB)
}

func ProcessFile(sf sniffedFile, root string, ch *chunker.Chunker, fmtr format.Formatter, st *stats.Stats) error {
	f, isBin, err := util.SniffAndRewind(sf.path)
	if err != nil {
		return err
	}
	defer f.Close()
	if isBin {
		return ErrBinaryFile
	}

	rel := toRel(root, sf.path)

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
