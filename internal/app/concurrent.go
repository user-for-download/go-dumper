package app

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"sync"

	"github.com/user-for-download/go-dumper/internal/chunker"
	"github.com/user-for-download/go-dumper/internal/cleaner"
	"github.com/user-for-download/go-dumper/internal/format"
	"github.com/user-for-download/go-dumper/internal/progress"
	"github.com/user-for-download/go-dumper/internal/stats"
	"github.com/user-for-download/go-dumper/internal/util"
)

type job struct {
	index int
	sf    sniffedFile
}

type result struct {
	index      int
	path       string
	render     renderResult
	payload    []byte
	processErr error
}

func RunConcurrent(files []sniffedFile, root string, ch *chunker.Chunker, mode cleaner.Mode, fmtr format.Formatter, st *stats.Stats, rep *progress.Reporter, workers int) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobs := make(chan job, workers*2)
	results := make(chan result, workers*2)

	go func() {
		defer close(jobs)
		for i, sf := range files {
			select {
			case <-ctx.Done():
				return
			case jobs <- job{index: i, sf: sf}:
			}
		}
	}()

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				r := result{index: j.index, path: j.sf.path}

				f, isBin, err := util.SniffAndRewind(j.sf.path)
				if err != nil {
					r.processErr = err
				} else if isBin {
					r.processErr = ErrBinaryFile
					f.Close()
				} else {
					rel := toRel(root, j.sf.path)

					var buf bytes.Buffer
					buf.WriteString(fmtr.FileHeader(rel))

					bytes, runes, cerr := renderFile(f, filepath.Ext(j.sf.path), mode, func(line string) error {
						_, werr := buf.WriteString(line)
						return werr
					})
					f.Close()
					if cerr != nil {
						r.processErr = cerr
					} else {
						if footer := fmtr.FileFooter(rel); footer != "" {
							buf.WriteString(footer)
						}
						r.render.bytes = bytes
						r.render.runes = runes
						r.payload = buf.Bytes()
					}
				}

				select {
				case results <- r:
				case <-ctx.Done():
					select {
					case results <- r:
					default:
					}
					return
				}
			}
		}()
	}
	go func() { wg.Wait(); close(results) }()

	pending := make(map[int]result)
	next := 0
	var writeErr error
	finishedCount := 0
	for r := range results {
		if writeErr != nil {
			rep.FinishFile()
			finishedCount++
			continue
		}
		pending[r.index] = r
		for {
			cur, ok := pending[next]
			if !ok {
				break
			}
			delete(pending, next)
			next++

			if cur.processErr != nil {
				if errors.Is(cur.processErr, ErrBinaryFile) {
					st.AddSkipped(cur.path, stats.ReasonBinary, nil)
				} else {
					st.AddError(cur.path + ": " + cur.processErr.Error())
				}
				rep.FinishFile()
				finishedCount++
				continue
			}
			if err := ch.WriteBytes(cur.payload, int(cur.render.runes)); err != nil {
				writeErr = err
				cancel()
				rep.FinishFile()
				finishedCount++
				break
			}
			st.IncProcessed(cur.render.bytes, cur.render.runes)
			rep.FinishFile()
			finishedCount++
		}
	}
	if writeErr != nil {
		cancel()
		for i := finishedCount; i < len(files); i++ {
			rep.FinishFile()
		}
		return writeErr
	}
	return nil
}
