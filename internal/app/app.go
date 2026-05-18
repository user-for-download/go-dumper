package app

import (
	"fmt"
	"os"

	"github.com/yourname/dumper/internal/chunker"
	"github.com/yourname/dumper/internal/cleaner"
	"github.com/yourname/dumper/internal/config"
	"github.com/yourname/dumper/internal/format"
	"github.com/yourname/dumper/internal/progress"
	"github.com/yourname/dumper/internal/stats"
	"github.com/yourname/dumper/internal/tree"
	"github.com/yourname/dumper/internal/util"
	"github.com/yourname/dumper/internal/walker"
)

func Run(cfg *config.Config) (*stats.Stats, error) {
	st := stats.New()

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

	textFiles := make([]sniffedFile, 0, len(entries))
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
		textFiles = append(textFiles, sniffedFile{
			path: e.Path, size: e.Size,
		})
	}

	st.SetTotalFiles(len(entries))

	if err := os.MkdirAll(cfg.Output, 0o755); err != nil {
		return st, err
	}

	fmtr, err := format.New(cfg.Format)
	if err != nil {
		return st, err
	}

	ch, err := chunker.New(chunker.Options{
		OutputDir:      cfg.Output,
		Prefix:         cfg.ChunkPrefix,
		MaxSymbols:     cfg.MaxSymbols,
		SplitLongLines: cfg.SplitLongLines,
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
		treeStr, terr := tree.Generate(tree.Options{
			Root:          cfg.Path,
			MaxDepth:      cfg.Tree.MaxDepth,
			IncludeSizes:  cfg.Tree.IncludeSizes,
			IncludeHidden: cfg.IncludeHidden,
			Mode:          treeMode,
			Includes:      includes,
			Excludes:      excludes,
		})
		if terr != nil {
			st.AddError("tree: " + terr.Error())
		}
		if treeStr != "" {
			if err := ch.WriteString(fmtr.TreeBlock(treeStr)); err != nil {
				return st, err
			}
		}
	}

	mode := cleaner.ModeOff
	if cfg.Clear.Enabled {
		if cfg.Clear.Mode == "line_and_block" {
			mode = cleaner.ModeLineAndBlock
		} else {
			mode = cleaner.ModeLine
		}
	}

	rep := progress.New(cfg.Progress, len(textFiles))
	defer rep.Done()

	if cfg.Concurrency > 1 {
		if err := RunConcurrent(textFiles, cfg.Path, ch, mode, fmtr, st, rep, cfg.Concurrency); err != nil {
			return st, err
		}
	} else {
		for _, sf := range textFiles {
			if err := ProcessFile(sf, cfg.Path, ch, mode, fmtr, st, rep); err != nil {
				st.AddError(sf.path + ": " + err.Error())
			}
			rep.FinishFile()
		}
	}

	if post := fmtr.Postamble(); post != "" {
		if err := ch.WriteString(post); err != nil {
			return st, err
		}
	}

	st.Finish(ch.ChunkCount())
	if cfg.StatsFile != "" {
		if err := st.WriteJSON(cfg.StatsFile); err != nil {
			return st, fmt.Errorf("stats: %w", err)
		}
	}
	return st, nil
}

func ProcessFile(sf sniffedFile, root string, ch *chunker.Chunker, mode cleaner.Mode, fmtr format.Formatter, st *stats.Stats, rep *progress.Reporter) error {
	r, err := renderFile(sf, root, mode, fmtr)
	if err != nil {
		return err
	}
	if err := ch.WriteString(r.header); err != nil {
		return err
	}
	if err := ch.WriteBytes(r.payload, int(r.runes)); err != nil {
		return err
	}
	if r.footer != "" {
		if err := ch.WriteString(r.footer); err != nil {
			return err
		}
	}
	st.IncProcessed(r.bytes, r.runes)
	return nil
}
