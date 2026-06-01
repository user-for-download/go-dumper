package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/user-for-download/go-dumper/internal/app"
	"github.com/user-for-download/go-dumper/internal/config"
	"github.com/user-for-download/go-dumper/internal/walker"
)

const longHelp = `dumper concatenates a project's text files into one or more chunked output
files, suitable for feeding into LLMs, code review, or archival.

Pattern files (@file syntax)
  Any --include or --exclude value that starts with "@" is treated as a path
  to a text file containing one pattern per line. Blank lines and lines
  starting with "#" are ignored.

Output format
  --format plain      Original "===== FILE: path =====" markers (default).
  --format markdown   Each file rendered as a fenced code block with a heading.
  --format xml        XML-ish <file path="..."><![CDATA[ ... ]]></file>.

Tree mode
  --tree-mode full     Show every file in the project tree (default).
  --tree-mode include  Show only files that match the active --include/--exclude
                       patterns — the tree mirrors exactly what was dumped.

Safety
  --exclude-self (on by default) auto-excludes the running binary if it is
  located inside the scan root, so dumper never tries to embed itself.

Comment stripping (--clear)
  The comment stripper is token-unaware and may corrupt strings that contain
  comment markers (e.g., "https://example.com"). For accurate parsing, use a
  language-specific parser.

Concurrency
  When --concurrency > 1, file reads run in parallel. Output order is
  preserved, but byte-for-byte reproducibility across runs is not guaranteed.
  Use --concurrency 1 for deterministic output.`

func main() {
	var (
		cfgPath string
		dryRun  bool
		verbose bool

		fPath, fOutput, fChunkPrefix, fClearMode, fStatsFile, fFormat, fTreeMode string
		fInclude, fExclude, fType                                                []string
		fMaxSymbols, fTreeDepth, fConcurrency                                    int
		fSplit, fProgress, fClear, fTree, fExcludeSelf, fIncludeHidden, fClean   bool
	)

	rootCmd := &cobra.Command{
		Use:   "dumper",
		Short: "Project text dumper",
		Long:  longHelp,
		RunE: func(c *cobra.Command, _ []string) error {
			cfg, err := config.Load(cfgPath, c.Flags().Changed("config"))
			if err != nil {
				return err
			}

			o := config.CLIOverrides{}
			if c.Flags().Changed("path") {
				o.Path = &fPath
			}
			if c.Flags().Changed("output") {
				o.Output = &fOutput
			}
			if c.Flags().Changed("include") {
				o.Include = fInclude
				o.IncludeSet = true
			}
			if c.Flags().Changed("exclude") {
				o.Exclude = fExclude
				o.ExcludeSet = true
			}
			if c.Flags().Changed("type") {
				o.Type = fType
				o.TypeSet = true
			}
			if c.Flags().Changed("clean") {
				o.Clean = &fClean
			}
			if c.Flags().Changed("max-symbols") {
				o.MaxSymbols = &fMaxSymbols
			}
			if c.Flags().Changed("chunk-prefix") {
				o.ChunkPrefix = &fChunkPrefix
			}
			if c.Flags().Changed("split-long-lines") {
				o.SplitLongLines = &fSplit
			}
			if c.Flags().Changed("progress") {
				o.Progress = &fProgress
			}
			if c.Flags().Changed("stats-file") {
				o.StatsFile = &fStatsFile
			}
			if c.Flags().Changed("clear") {
				o.ClearEnabled = &fClear
			}
			if c.Flags().Changed("clear-mode") {
				o.ClearMode = &fClearMode
			}
			if c.Flags().Changed("tree") {
				o.TreeEnabled = &fTree
			}
			if c.Flags().Changed("tree-depth") {
				o.TreeDepth = &fTreeDepth
			}
			if c.Flags().Changed("tree-mode") {
				o.TreeMode = &fTreeMode
			}
			if c.Flags().Changed("concurrency") {
				o.Concurrency = &fConcurrency
			}
			if c.Flags().Changed("exclude-self") {
				o.ExcludeSelf = &fExcludeSelf
			}
			if c.Flags().Changed("include-hidden") {
				o.IncludeHidden = &fIncludeHidden
			}
			if c.Flags().Changed("format") {
				o.Format = &fFormat
			}
			if err := cfg.MergeCLI(o); err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}

			if dryRun {
				includes, err := walker.ExpandPatterns(cfg.Include)
				if err != nil {
					return fmt.Errorf("include patterns: %w", err)
				}
				excludes := app.EffectiveExcludes(cfg.Path, cfg.Output, cfg.Exclude, cfg.ExcludeSelf)
				excludes, err = walker.ExpandPatterns(excludes)
				if err != nil {
					return fmt.Errorf("exclude patterns: %w", err)
				}
				w, err := walker.New(walker.Options{
					Root:          cfg.Path,
					Includes:      includes,
					Excludes:      excludes,
					Type:          cfg.Type,
					IncludeHidden: cfg.IncludeHidden,
				})
				if err != nil {
					return err
				}
				entries, err := w.Collect()
				if err != nil {
					return err
				}
				for _, e := range entries {
					fmt.Println(e.Path)
				}
				return nil
			}

			st, err := app.Run(cfg)
			if err != nil {
				return err
			}
			if verbose {
				fmt.Printf("processed: %d/%d files, %d chunks, %.2f sec\n",
					st.ProcessedFiles(), st.TotalFiles(), st.ChunksCreated(), st.DurationSec())
			}
			return nil
		},
	}

	rootCmd.Flags().StringVarP(&cfgPath, "config", "c", "prc.json", "config file")
	rootCmd.Flags().StringVarP(&fPath, "path", "p", ".", "project root")
	rootCmd.Flags().StringVarP(&fOutput, "output", "o", "./dump_out", "output dir")
	rootCmd.Flags().StringSliceVarP(&fInclude, "include", "i", nil,
		`include patterns (doublestar globs); use @file to load patterns from a file`)
	rootCmd.Flags().StringSliceVarP(&fExclude, "exclude", "x", nil,
		`exclude patterns (doublestar globs); use @file to load patterns from a file`)
	rootCmd.Flags().StringSliceVar(&fType, "type", nil, "filter by file extension (e.g., go, js)")
	rootCmd.Flags().BoolVar(&fClean, "clean", false, "clean output folder before writing")
	rootCmd.Flags().IntVarP(&fMaxSymbols, "max-symbols", "m", 1_000_000, "max runes per chunk")
	rootCmd.Flags().StringVar(&fChunkPrefix, "chunk-prefix", "dump", "chunk file prefix")
	rootCmd.Flags().BoolVar(&fSplit, "split-long-lines", false, "split oversized lines")
	rootCmd.Flags().BoolVar(&fClear, "clear", false, "strip comments")
	rootCmd.Flags().StringVar(&fClearMode, "clear-mode", "line", "line|line_and_block")
	rootCmd.Flags().BoolVar(&fTree, "tree", false, "prepend project tree")
	rootCmd.Flags().IntVar(&fTreeDepth, "tree-depth", 0, "max tree depth")
	rootCmd.Flags().StringVar(&fTreeMode, "tree-mode", "full",
		"tree content: full (all files) | include (only files matching --include/--exclude)")
	rootCmd.Flags().BoolVar(&fProgress, "progress", false, "show progress bar")
	rootCmd.Flags().StringVar(&fStatsFile, "stats-file", "", "write stats JSON")
	rootCmd.Flags().IntVar(&fConcurrency, "concurrency", 1, "parallel readers")
	rootCmd.Flags().BoolVar(&fExcludeSelf, "exclude-self", true,
		"auto-skip the dumper binary if it lives inside the scan root")
	rootCmd.Flags().BoolVar(&fIncludeHidden, "include-hidden", false,
		"include files and directories starting with .")
	rootCmd.Flags().StringVar(&fFormat, "format", "plain",
		"output format: plain | markdown | xml")
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "list files only")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "verbose logging")

	rootCmd.AddCommand(newInitCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
