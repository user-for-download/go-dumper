package stats

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type SkippedReason string

const (
	ReasonBinary SkippedReason = "binary"
	ReasonError  SkippedReason = "error"
)

type SkippedFile struct {
	Path   string        `json:"path"`
	Reason SkippedReason `json:"reason"`
	Err    string        `json:"error,omitempty"`
}

type statsData struct {
	StartedAt      time.Time     `json:"started_at"`
	FinishedAt     time.Time     `json:"finished_at"`
	DurationSec    float64       `json:"duration_seconds"`
	TotalFiles     int           `json:"total_files"`
	ProcessedFiles int           `json:"processed_files"`
	SkippedFiles   int           `json:"skipped_files"`
	TotalBytes     int64         `json:"total_bytes"`
	TotalRunes     int64         `json:"total_runes"`
	ChunksCreated  int           `json:"chunks_created"`
	Skipped        []SkippedFile `json:"skipped,omitempty"`
	Errors         []string      `json:"errors,omitempty"`
}

type Stats struct {
	mu   sync.Mutex
	data statsData
}

func New() *Stats {
	return &Stats{data: statsData{StartedAt: time.Now()}}
}

func (s *Stats) SetTotalFiles(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.TotalFiles = n
}

func (s *Stats) IncProcessed(bytes, runes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.ProcessedFiles++
	s.data.TotalBytes += bytes
	s.data.TotalRunes += runes
}

func (s *Stats) AddSkipped(path string, reason SkippedReason, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.SkippedFiles++
	sf := SkippedFile{Path: path, Reason: reason}
	if err != nil {
		sf.Err = err.Error()
	}
	s.data.Skipped = append(s.data.Skipped, sf)
}

func (s *Stats) AddError(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Errors = append(s.data.Errors, msg)
}

func (s *Stats) Finish(chunks int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.FinishedAt = time.Now()
	s.data.DurationSec = s.data.FinishedAt.Sub(s.data.StartedAt).Seconds()
	s.data.ChunksCreated = chunks
}

func (s *Stats) WriteJSON(path string) error {
	s.mu.Lock()
	cp := s.data
	cp.Skipped = append([]SkippedFile(nil), s.data.Skipped...)
	cp.Errors = append([]string(nil), s.data.Errors...)
	s.mu.Unlock()
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (s *Stats) ProcessedFiles() int  { s.mu.Lock(); defer s.mu.Unlock(); return s.data.ProcessedFiles }
func (s *Stats) TotalFiles() int      { s.mu.Lock(); defer s.mu.Unlock(); return s.data.TotalFiles }
func (s *Stats) ChunksCreated() int   { s.mu.Lock(); defer s.mu.Unlock(); return s.data.ChunksCreated }
func (s *Stats) DurationSec() float64 { s.mu.Lock(); defer s.mu.Unlock(); return s.data.DurationSec }
func (s *Stats) SkippedFiles() int    { s.mu.Lock(); defer s.mu.Unlock(); return s.data.SkippedFiles }
