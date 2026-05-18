package app

import (
	"context"
	"sync"

	"github.com/user-for-download/go-dumper/internal/chunker"
	"github.com/user-for-download/go-dumper/internal/cleaner"
	"github.com/user-for-download/go-dumper/internal/format"
	"github.com/user-for-download/go-dumper/internal/progress"
	"github.com/user-for-download/go-dumper/internal/stats"
)

type job struct {
	index int
	sf    sniffedFile
}

type result struct {
	index      int
	path       string
	render     renderResult
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
				rendered, err := renderFile(j.sf, root, mode, fmtr)
				if err != nil {
					r.processErr = err
				} else {
					r.render = rendered
				}
				select {
				case <-ctx.Done():
					return
				case results <- r:
				}
			}
		}()
	}
	go func() { wg.Wait(); close(results) }()

	pending := make(map[int]result)
	next := 0
	var writeErr error
	for r := range results {
		if writeErr != nil {
			rep.FinishFile()
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
				st.AddError(cur.path + ": " + cur.processErr.Error())
				rep.FinishFile()
				continue
			}
			if err := ch.WriteString(cur.render.header); err != nil {
				writeErr = err
				cancel()
				rep.FinishFile()
				break
			}
			if err := ch.WriteBytes(cur.render.payload, int(cur.render.runes)); err != nil {
				writeErr = err
				cancel()
				rep.FinishFile()
				break
			}
			if cur.render.footer != "" {
				if err := ch.WriteString(cur.render.footer); err != nil {
					writeErr = err
					cancel()
					rep.FinishFile()
					break
				}
			}
			st.IncProcessed(cur.render.bytes, cur.render.runes)
			rep.FinishFile()
		}
	}
	if writeErr != nil {
		cancel()
		return writeErr
	}
	return nil
}
