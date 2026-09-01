package app

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"sync"

	"github.com/user-for-download/go-dumper/internal/chunker"
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
	payload    string
	processErr error
}

func RunConcurrent(files []sniffedFile, root string, ch *chunker.Chunker, fmtr format.Formatter, st *stats.Stats, rep *progress.Reporter, workers int) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tempDir, err := os.MkdirTemp("", "dumper-render-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

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
					rel := ToRel(root, j.sf.path)

					tmp, terr := os.CreateTemp(tempDir, "render-*")
					if terr != nil {
						f.Close()
						r.processErr = terr
						select {
						case results <- r:
						case <-ctx.Done():
							return
						}
						continue
					}
					_, terr = tmp.WriteString(fmtr.FileHeader(rel))

					bytes, runes, cerr := renderFile(f, func(line string) error {
						_, werr := tmp.WriteString(line)
						return werr
					})
					f.Close()
					if terr == nil && cerr == nil {
						if footer := fmtr.FileFooter(rel); footer != "" {
							_, terr = tmp.WriteString(footer)
						}
					}
					if closeErr := tmp.Close(); terr == nil {
						terr = closeErr
					}
					if terr != nil || cerr != nil {
						os.Remove(tmp.Name())
						if terr != nil {
							r.processErr = terr
						} else {
							r.processErr = cerr
						}
					} else {
						r.render.bytes = bytes
						r.render.runes = runes
						r.payload = tmp.Name()
					}
				}

				select {
				case results <- r:
				case <-ctx.Done():
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
					st.AddSkipped(cur.path, stats.ReasonError, cur.processErr)
				}
				rep.FinishFile()
				finishedCount++
				continue
			}
			payload, err := os.Open(cur.payload)
			if err == nil {
				err = copyPayload(payload, ch.WriteString)
			}
			if payload != nil {
				if closeErr := payload.Close(); err == nil {
					err = closeErr
				}
			}
			os.Remove(cur.payload)
			if err != nil {
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
		for i := finishedCount; i < len(files); i++ {
			rep.FinishFile()
		}
		return writeErr
	}
	return nil
}

// copyPayload copies a rendered temp payload into the chunker line by line,
// mirroring renderFile: complete lines keep chunk rotation at line
// boundaries, so chunks stay rune-correct even in concurrent mode.
func copyPayload(f *os.File, write func(string) error) error {
	r := bufio.NewReaderSize(f, 64*1024)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			if werr := write(string(line)); werr != nil {
				return werr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}
