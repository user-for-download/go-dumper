package progress

import (
	"os"
	"time"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

type Reporter struct {
	enabled bool
	p       *mpb.Progress
	files   *mpb.Bar
}

func New(enabled bool, totalFiles int) *Reporter {
	if !enabled {
		return &Reporter{}
	}
	if totalFiles == 0 {
		return &Reporter{}
	}
	p := mpb.New(mpb.WithOutput(os.Stderr), mpb.WithRefreshRate(120*time.Millisecond))
	files := p.AddBar(int64(totalFiles),
		mpb.PrependDecorators(decor.Name("files "), decor.CountersNoUnit("%d/%d ")),
		mpb.AppendDecorators(decor.Percentage(), decor.Name(" "), decor.AverageETA(decor.ET_STYLE_GO)),
	)
	return &Reporter{enabled: true, p: p, files: files}
}

func (r *Reporter) FinishFile() {
	if r.enabled {
		r.files.Increment()
	}
}

func (r *Reporter) Done() {
	if r.enabled {
		r.p.Wait()
	}
}
