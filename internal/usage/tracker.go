// Package usage logs per-call token and cost data to ~/.conduit/usage.jsonl
// and exposes running session totals for the TUI status bar.
package usage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
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

const defaultLogPath = ".conduit/usage.jsonl"

// Tracker appends usage entries to disk and maintains in-memory session totals.
// All methods are safe for concurrent use.
type Tracker struct {
	mu        sync.Mutex
	sessionID string
	logPath   string
	summary   contracts.UsageSummary
}

// New creates a Tracker that writes to ~/.conduit/usage.jsonl.
func New(sessionID string) (*Tracker, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("usage: resolve home dir: %w", err)
	}
	logPath := filepath.Join(home, defaultLogPath)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, fmt.Errorf("usage: create log dir: %w", err)
	}
	return &Tracker{sessionID: sessionID, logPath: logPath}, nil
}

// NewWithPath creates a Tracker writing to an explicit path (useful in tests).
func NewWithPath(sessionID, logPath string) (*Tracker, error) {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, fmt.Errorf("usage: create log dir: %w", err)
	}
	return &Tracker{sessionID: sessionID, logPath: logPath}, nil
}

// Record appends one model-call entry to the JSONL log and updates session totals.
func (t *Tracker) Record(provider, model string, inputTokens, outputTokens int) (contracts.UsageEntry, error) {
	cost := computeCost(model, inputTokens, outputTokens)
	entry := contracts.UsageEntry{
		At:           time.Now().UTC(),
		SessionID:    t.sessionID,
		Provider:     provider,
		Model:        model,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  inputTokens + outputTokens,
		CostUSD:      cost,
	}

	if err := t.appendLine(entry); err != nil {
		return entry, err
	}

	t.mu.Lock()
	t.summary.Model = model
	t.summary.TotalTokens += entry.TotalTokens
	t.summary.TotalCostUSD += cost
	t.mu.Unlock()

	return entry, nil
}

// Summary returns the running session totals without blocking on disk I/O.
func (t *Tracker) Summary() contracts.UsageSummary {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.summary
}

func (t *Tracker) appendLine(entry contracts.UsageEntry) error {
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
