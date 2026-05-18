package chunker

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/yourname/dumper/internal/util"
)

type Options struct {
	OutputDir      string
	Prefix         string
	MaxSymbols     int
	SplitLongLines bool
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
	name := fmt.Sprintf("%s_%05d.txt", c.opts.Prefix, c.chunkIdx)
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
	return c.writeKnown(s, util.RuneCount(s))
}

func (c *Chunker) WriteBytes(b []byte, runes int) error {
	if len(b) == 0 && runes == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureOpen(); err != nil {
		return err
	}

	if c.curSymbols > 0 && c.curSymbols+runes > c.opts.MaxSymbols {
		if err := c.openNew(); err != nil {
			return err
		}
	}

	if runes <= c.opts.MaxSymbols {
		_, err := c.bw.Write(b)
		if err == nil {
			c.curSymbols += runes
		}
		return err
	}

	if !c.opts.SplitLongLines {
		return fmt.Errorf("file exceeds max symbols (%d > %d) and split_long_lines is false", runes, c.opts.MaxSymbols)
	}
	return c.writeSplit(string(b), runes)
}

func (c *Chunker) writeKnown(s string, runes int) error {
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
		return fmt.Errorf("line exceeds max symbols (%d > %d) and split_long_lines is false", runes, c.opts.MaxSymbols)
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
	runes := []rune(s)
	max := c.opts.MaxSymbols
	i := 0
	for i < len(runes) {
		if c.curSymbols > 0 {
			if err := c.openNew(); err != nil {
				return err
			}
		}
		end := i + max
		if end > len(runes) {
			end = len(runes)
		}
		piece := string(runes[i:end])
		pieceRunes := end - i
		if err := c.rawWrite(piece, pieceRunes); err != nil {
			return err
		}
		i = end
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
