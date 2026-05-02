package usage

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// LogFile describes one daily usage log on disk.
type LogFile struct {
	Path       string
	Compressed bool
}

// ListLogs returns daily usage logs (.jsonl and .jsonl.gz) under dir, sorted by name.
// A missing directory is not an error and returns no files.
func ListLogs(dir string) ([]LogFile, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("usage: read log dir: %w", err)
	}
	var logs []LogFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		switch {
		case strings.HasSuffix(name, ".jsonl"):
			logs = append(logs, LogFile{Path: filepath.Join(dir, name)})
		case strings.HasSuffix(name, ".jsonl.gz"):
			logs = append(logs, LogFile{Path: filepath.Join(dir, name), Compressed: true})
		}
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].Path < logs[j].Path })
	return logs, nil
}

// openReader returns a reader for a log file, transparently decompressing .gz.
// Callers must close both the returned ReadCloser and the underlying file.
func openReader(log LogFile) (io.ReadCloser, error) {
	f, err := os.Open(log.Path)
	if err != nil {
		return nil, err
	}
	if !log.Compressed {
		return f, nil
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("usage: gzip reader %s: %w", log.Path, err)
	}
	return &gzipReadCloser{gz: gz, file: f}, nil
}

type gzipReadCloser struct {
	gz   *gzip.Reader
	file *os.File
}

func (g *gzipReadCloser) Read(p []byte) (int, error) { return g.gz.Read(p) }
func (g *gzipReadCloser) Close() error {
	gzErr := g.gz.Close()
	fileErr := g.file.Close()
	if gzErr != nil {
		return gzErr
	}
	return fileErr
}

// ScanLog calls fn for each entry in log. Returning false from fn stops iteration.
// Malformed lines are skipped silently; corruption shouldn't break user reports.
func ScanLog(log LogFile, fn func(contracts.UsageEntry) bool) error {
	r, err := openReader(log)
	if err != nil {
		return err
	}
	defer r.Close()

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry contracts.UsageEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if !fn(entry) {
			return nil
		}
	}
	return scanner.Err()
}

// ScanAll iterates every entry across every log file in dir.
func ScanAll(dir string, fn func(contracts.UsageEntry) bool) error {
	logs, err := ListLogs(dir)
	if err != nil {
		return err
	}
	for _, log := range logs {
		stop := false
		if err := ScanLog(log, func(e contracts.UsageEntry) bool {
			if !fn(e) {
				stop = true
				return false
			}
			return true
		}); err != nil {
			return err
		}
		if stop {
			return nil
		}
	}
	return nil
}

// RewriteResult reports how many entries were kept and dropped from one file.
type RewriteResult struct {
	Path    string
	Kept    int
	Dropped int
	Removed bool // true if the file was deleted because no entries remained
}

// RewriteLog atomically rewrites a log, keeping only entries for which keep returns true.
// Files are written to a sibling .tmp and renamed; the format (.jsonl vs .gz) is preserved.
// If every entry is dropped, the file is removed entirely.
func RewriteLog(log LogFile, keep func(contracts.UsageEntry) bool) (RewriteResult, error) {
	result := RewriteResult{Path: log.Path}

	r, err := openReader(log)
	if err != nil {
		return result, err
	}

	tmp, err := os.CreateTemp(filepath.Dir(log.Path), filepath.Base(log.Path)+".tmp-*")
	if err != nil {
		r.Close()
		return result, fmt.Errorf("usage: create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	var writer io.Writer = tmp
	var gzWriter *gzip.Writer
	if log.Compressed {
		gzWriter = gzip.NewWriter(tmp)
		writer = gzWriter
	}

	cleanup := func() {
		if gzWriter != nil {
			_ = gzWriter.Close()
		}
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var entry contracts.UsageEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			// Preserve malformed lines verbatim — never silently drop user data on parse error.
			line := append([]byte{}, raw...)
			line = append(line, '\n')
			if _, err := writer.Write(line); err != nil {
				cleanup()
				r.Close()
				return result, fmt.Errorf("usage: write malformed line: %w", err)
			}
			result.Kept++
			continue
		}
		if !keep(entry) {
			result.Dropped++
			continue
		}
		line := append(raw, '\n')
		if _, err := writer.Write(line); err != nil {
			cleanup()
			r.Close()
			return result, fmt.Errorf("usage: write entry: %w", err)
		}
		result.Kept++
	}
	if err := scanner.Err(); err != nil {
		cleanup()
		r.Close()
		return result, fmt.Errorf("usage: scan log: %w", err)
	}
	r.Close()

	if gzWriter != nil {
		if err := gzWriter.Close(); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
			return result, fmt.Errorf("usage: finalize gzip: %w", err)
		}
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return result, fmt.Errorf("usage: close temp file: %w", err)
	}

	if result.Kept == 0 {
		_ = os.Remove(tmpPath)
		if err := os.Remove(log.Path); err != nil && !os.IsNotExist(err) {
			return result, fmt.Errorf("usage: remove emptied log: %w", err)
		}
		result.Removed = true
		return result, nil
	}

	if err := os.Rename(tmpPath, log.Path); err != nil {
		_ = os.Remove(tmpPath)
		return result, fmt.Errorf("usage: replace log: %w", err)
	}
	return result, nil
}
