package chunker

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/user-for-download/go-dumper/internal/util"
)

type Options struct {
	OutputDir      string
	Prefix         string
	MaxSymbols     int
	SplitLongLines bool
	Extension      string
}

type Chunker struct {
	mu         sync.Mutex
	opts       Options
	current    *os.File
	bw         *bufio.Writer
	chunkIdx   int
	curSymbols int
}

func New(opts Options) (*Chunker, error) {
	if opts.MaxSymbols <= 0 {
		return nil, fmt.Errorf("MaxSymbols must be > 0")
	}
	if opts.Prefix == "" {
		opts.Prefix = "dump"
	}
	return &Chunker{opts: opts}, nil
}

func (c *Chunker) ensureOpen() error {
	if c.bw != nil {
		return nil
	}
	return c.openNew()
}

func (c *Chunker) openNew() error {
	if c.bw != nil {
		_ = c.bw.Flush()
	}
	if c.current != nil {
		_ = c.current.Close()
	}
	c.chunkIdx++
	ext := c.opts.Extension
	if ext == "" {
		ext = ".txt"
	} else if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	name := fmt.Sprintf("%s_%05d%s", c.opts.Prefix, c.chunkIdx, ext)
	path := filepath.Join(c.opts.OutputDir, name)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	c.current = f
	c.bw = bufio.NewWriterSize(f, 64*1024)
	c.curSymbols = 0
	return nil
}

func (c *Chunker) WriteString(s string) error {
	return c.writeInternal(s, util.RuneCount(s))
}

func (c *Chunker) WriteBytes(b []byte, runes int) error {
	if len(b) == 0 || runes == 0 {
		return nil
	}
	return c.writeInternal(string(b), runes)
}

// writeInternal is the single core write path — both WriteString and WriteBytes
// delegate here. It acquires the lock and handles rotation, splitting, and
// symbol accounting in one consistent code path.
func (c *Chunker) writeInternal(s string, runes int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if s == "" || runes == 0 {
		return nil
	}
	if err := c.ensureOpen(); err != nil {
		return err
	}
	if c.curSymbols+runes <= c.opts.MaxSymbols {
		return c.rawWrite(s, runes)
	}
	if c.curSymbols > 0 {
		if err := c.openNew(); err != nil {
			return err
		}
	}
	if runes <= c.opts.MaxSymbols {
		return c.rawWrite(s, runes)
	}
	if !c.opts.SplitLongLines {
		return fmt.Errorf("content exceeds max symbols (%d > %d) and split_long_lines is false", runes, c.opts.MaxSymbols)
	}
	return c.writeSplit(s, runes)
}

func (c *Chunker) rawWrite(s string, runes int) error {
	_, err := c.bw.WriteString(s)
	if err == nil {
		c.curSymbols += runes
	}
	return err
}

func (c *Chunker) writeSplit(s string, totalRunes int) error {
	max := c.opts.MaxSymbols
	var pieceStart int
	runeCount := 0

	for i := range s {
		if runeCount == max {
			if err := c.rawWrite(s[pieceStart:i], runeCount); err != nil {
				return err
			}
			if err := c.openNew(); err != nil {
				return err
			}
			pieceStart = i
			runeCount = 0
		}
		runeCount++
	}

	if pieceStart < len(s) {
		return c.rawWrite(s[pieceStart:], runeCount)
	}
	return nil
}

func (c *Chunker) ChunkCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.chunkIdx
}

func (c *Chunker) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var firstErr error
	if c.bw != nil {
		if err := c.bw.Flush(); err != nil {
			firstErr = fmt.Errorf("flush: %w", err)
		}
		c.bw = nil
	}
	if c.current != nil {
		if err := c.current.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close: %w", err)
		}
		c.current = nil
	}
	return firstErr
}
