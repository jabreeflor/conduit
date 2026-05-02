package guardrails

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// AuditSink consumes one guardrail decision per call. The default file-based
// implementation appends one JSONL record per call (matching the per-session
// JSONL convention from PR #15). Tests use the slice-backed MemoryAuditSink.
type AuditSink interface {
	Write(entry AuditEntry) error
}

// FileAuditSink writes one JSON record per Write call to the configured
// path, separated by newlines. Safe for concurrent use.
type FileAuditSink struct {
	mu   sync.Mutex
	path string
}

// NewFileAuditSink returns a sink that appends to path. Parent directories
// are created on demand.
func NewFileAuditSink(path string) (*FileAuditSink, error) {
	if path == "" {
		return nil, fmt.Errorf("guardrails: audit path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("guardrails: create audit dir: %w", err)
	}
	return &FileAuditSink{path: path}, nil
}

// Write appends one JSONL entry to the audit file.
func (s *FileAuditSink) Write(entry AuditEntry) error {
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("guardrails: marshal audit entry: %w", err)
	}
	line = append(line, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("guardrails: open audit log: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("guardrails: write audit entry: %w", err)
	}
	return nil
}

// WriterAuditSink wraps an arbitrary io.Writer (e.g. a buffer in tests).
type WriterAuditSink struct {
	mu sync.Mutex
	w  io.Writer
}

// NewWriterAuditSink returns a sink that JSONL-encodes to w.
func NewWriterAuditSink(w io.Writer) *WriterAuditSink {
	return &WriterAuditSink{w: w}
}

// Write JSONL-encodes one entry to the underlying writer.
func (s *WriterAuditSink) Write(entry AuditEntry) error {
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("guardrails: marshal audit entry: %w", err)
	}
	line = append(line, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.w.Write(line); err != nil {
		return fmt.Errorf("guardrails: write audit entry: %w", err)
	}
	return nil
}

// MemoryAuditSink keeps every entry in-memory; useful in tests.
type MemoryAuditSink struct {
	mu      sync.Mutex
	entries []AuditEntry
}

// Write appends entry to the in-memory slice.
func (s *MemoryAuditSink) Write(entry AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
	return nil
}

// Entries returns a copy of every recorded entry, in arrival order.
func (s *MemoryAuditSink) Entries() []AuditEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]AuditEntry(nil), s.entries...)
}

// nopAuditSink is the default when no sink is configured.
type nopAuditSink struct{}

func (nopAuditSink) Write(_ AuditEntry) error { return nil }
