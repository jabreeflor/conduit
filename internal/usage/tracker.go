// Package usage logs per-call token and latency data to ~/.conduit/logs/usage
// and exposes running session totals for the TUI status bar.
package usage

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
	"github.com/jabreeflor/conduit/internal/router"
)

// modelPricing holds input/output cost per 1M tokens in USD.
type modelPricing struct {
	inputPer1M  float64
	outputPer1M float64
}

// knownPricing is the versioned cost table for supported models.
// Costs are in USD per 1 million tokens.
var knownPricing = map[string]modelPricing{
	"claude-haiku-4-5":  {inputPer1M: 0.80, outputPer1M: 4.00},
	"claude-sonnet-4-6": {inputPer1M: 3.00, outputPer1M: 15.00},
	"claude-opus-4-6":   {inputPer1M: 15.00, outputPer1M: 75.00},
	"claude-opus-4-7":   {inputPer1M: 15.00, outputPer1M: 75.00},
	"gpt-4o":            {inputPer1M: 5.00, outputPer1M: 15.00},
	"gpt-4o-mini":       {inputPer1M: 0.15, outputPer1M: 0.60},
	"gemini-1.5-pro":    {inputPer1M: 3.50, outputPer1M: 10.50},
	"gemini-1.5-flash":  {inputPer1M: 0.075, outputPer1M: 0.30},
}

const (
	defaultLogDir        = ".conduit/logs/usage"
	defaultCompressAfter = 7 * 24 * time.Hour
	defaultRetention     = 90 * 24 * time.Hour
)

// Tracker appends usage entries to disk and maintains in-memory session totals.
// All methods are safe for concurrent use.
type Tracker struct {
	mu            sync.Mutex
	sessionID     string
	logPath       string
	logDir        string
	compressAfter time.Duration
	retention     time.Duration
	now           func() time.Time
	summary       contracts.UsageSummary
}

// New creates a Tracker that writes daily logs under ~/.conduit/logs/usage.
func New(sessionID string) (*Tracker, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("usage: resolve home dir: %w", err)
	}
	return NewWithDir(sessionID, filepath.Join(home, defaultLogDir))
}

// NewWithPath creates a Tracker writing to an explicit path (useful in tests).
func NewWithPath(sessionID, logPath string) (*Tracker, error) {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, fmt.Errorf("usage: create log dir: %w", err)
	}
	return newTracker(sessionID, logPath, "")
}

// NewWithDir creates a Tracker writing YYYY-MM-DD.jsonl files under logDir.
func NewWithDir(sessionID, logDir string) (*Tracker, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("usage: create log dir: %w", err)
	}
	return newTracker(sessionID, "", logDir)
}

func newTracker(sessionID, logPath, logDir string) (*Tracker, error) {
	return &Tracker{
		sessionID:     sessionID,
		logPath:       logPath,
		logDir:        logDir,
		compressAfter: defaultCompressAfter,
		retention:     defaultRetention,
		now:           func() time.Time { return time.Now().UTC() },
	}, nil
}

// Record appends one model-call entry to the JSONL log and updates session totals.
func (t *Tracker) Record(provider, model string, inputTokens, outputTokens int) (contracts.UsageEntry, error) {
	return t.RecordEvent(Record{
		Provider:     provider,
		Model:        model,
		TokensIn:     inputTokens,
		TokensOut:    outputTokens,
		Status:       "success",
		TotalLatency: 0,
	})
}

// Record captures all per-request usage fields written to JSONL.
type Record struct {
	Provider     string
	Model        string
	TokensIn     int
	TokensOut    int
	TTFT         time.Duration
	TotalLatency time.Duration
	Status       string
	ErrorType    string
	Feature      string
	Plugin       string
	Timestamp    time.Time
	TokensPerSec float64
	CostUSD      float64
}

// RecordEvent appends one request usage event to the JSONL log and updates
// session totals for successful token-bearing calls.
func (t *Tracker) RecordEvent(record Record) (contracts.UsageEntry, error) {
	if record.Timestamp.IsZero() {
		record.Timestamp = t.now()
	}
	if record.Status == "" {
		record.Status = "success"
	}
	cost := record.CostUSD
	if cost == 0 {
		cost = computeCost(record.Model, record.TokensIn, record.TokensOut)
	}
	tokensPerSec := record.TokensPerSec
	if tokensPerSec == 0 && record.TotalLatency > 0 && record.TokensOut > 0 {
		tokensPerSec = float64(record.TokensOut) / record.TotalLatency.Seconds()
	}

	entry := contracts.UsageEntry{
		Timestamp:       record.Timestamp.UTC(),
		SessionID:       t.sessionID,
		Provider:        record.Provider,
		Model:           record.Model,
		TokensIn:        record.TokensIn,
		TokensOut:       record.TokensOut,
		TotalTokens:     record.TokensIn + record.TokensOut,
		TTFMS:           record.TTFT.Milliseconds(),
		TotalMS:         record.TotalLatency.Milliseconds(),
		TokensPerSecond: tokensPerSec,
		Status:          record.Status,
		ErrorType:       record.ErrorType,
		Feature:         record.Feature,
		Plugin:          record.Plugin,
		CostUSD:         cost,
	}

	if err := t.appendLine(entry); err != nil {
		return entry, err
	}

	t.mu.Lock()
	t.summary.Model = record.Model
	t.summary.TotalTokens += entry.TotalTokens
	t.summary.TotalCostUSD += cost
	t.mu.Unlock()

	return entry, nil
}

// RecordUsage lets Tracker satisfy router.UsageSink for direct per-request
// accounting from the model router.
func (t *Tracker) RecordUsage(_ context.Context, record router.UsageRecord) error {
	_, err := t.RecordEvent(Record{
		Provider:     record.Provider,
		Model:        record.Model,
		TokensIn:     record.InputTokens,
		TokensOut:    record.OutputTokens,
		TTFT:         record.TTFT,
		TotalLatency: record.TotalLatency,
		Status:       record.Status,
		ErrorType:    record.ErrorType,
		Feature:      record.Feature,
		Plugin:       record.Plugin,
		Timestamp:    record.RecordedAt,
		CostUSD:      record.CostUSD,
	})
	return err
}

// Summary returns the running session totals without blocking on disk I/O.
func (t *Tracker) Summary() contracts.UsageSummary {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.summary
	s.SessionID = t.sessionID
	return s
}

func (t *Tracker) appendLine(entry contracts.UsageEntry) error {
	if t.logDir != "" {
		if err := t.maintainLogs(entry.Timestamp); err != nil {
			return err
		}
		t.logPath = filepath.Join(t.logDir, entry.Timestamp.Format("2006-01-02")+".jsonl")
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("usage: marshal entry: %w", err)
	}
	line = append(line, '\n')

	f, err := os.OpenFile(t.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("usage: open log: %w", err)
	}
	defer f.Close()

	_, err = f.Write(line)
	return err
}

func (t *Tracker) maintainLogs(now time.Time) error {
	entries, err := os.ReadDir(t.logDir)
	if err != nil {
		return fmt.Errorf("usage: read log dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonl") && !strings.HasSuffix(name, ".jsonl.gz") {
			continue
		}
		dateText := strings.TrimSuffix(strings.TrimSuffix(name, ".gz"), ".jsonl")
		logDate, err := time.Parse("2006-01-02", dateText)
		if err != nil {
			continue
		}
		age := now.Sub(logDate)
		path := filepath.Join(t.logDir, name)
		if age > t.retention {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("usage: remove expired log: %w", err)
			}
			continue
		}
		if age > t.compressAfter && strings.HasSuffix(name, ".jsonl") {
			if err := gzipFile(path); err != nil {
				return err
			}
		}
	}
	return nil
}

func gzipFile(path string) error {
	in, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("usage: open log for compression: %w", err)
	}
	defer in.Close()

	gzPath := path + ".gz"
	out, err := os.OpenFile(gzPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if os.IsExist(err) {
		return os.Remove(path)
	}
	if err != nil {
		return fmt.Errorf("usage: create compressed log: %w", err)
	}
	removeGZ := true
	defer func() {
		out.Close()
		if removeGZ {
			_ = os.Remove(gzPath)
		}
	}()

	gz := gzip.NewWriter(out)
	if _, err := io.Copy(gz, in); err != nil {
		gz.Close()
		return fmt.Errorf("usage: compress log: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("usage: finish compressed log: %w", err)
	}
	removeGZ = false
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("usage: remove uncompressed log: %w", err)
	}
	return nil
}

// computeCost returns the USD cost using the known pricing table.
// Returns zero for unrecognised models so a missing entry never blocks logging.
func computeCost(model string, inputTokens, outputTokens int) float64 {
	p, ok := knownPricing[model]
	if !ok {
		return 0
	}
	return float64(inputTokens)/1_000_000*p.inputPer1M +
		float64(outputTokens)/1_000_000*p.outputPer1M
}
