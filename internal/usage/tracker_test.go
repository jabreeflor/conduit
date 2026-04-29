package usage

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestRecord_appendsJSONL(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "usage.jsonl")

	tracker, err := NewWithPath("sess-1", logPath)
	if err != nil {
		t.Fatalf("NewWithPath: %v", err)
	}

	entry, err := tracker.Record("anthropic", "claude-haiku-4-5", 1000, 200)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	if entry.SessionID != "sess-1" {
		t.Errorf("SessionID = %q, want sess-1", entry.SessionID)
	}
	if entry.TotalTokens != 1200 {
		t.Errorf("TotalTokens = %d, want 1200", entry.TotalTokens)
	}
	if entry.CostUSD == 0 {
		t.Error("CostUSD should be non-zero for a known model")
	}

	f, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()

	var parsed contracts.UsageEntry
	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		t.Fatal("log file is empty")
	}
	if err := json.Unmarshal(scanner.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal line: %v", err)
	}
	if parsed.Model != "claude-haiku-4-5" {
		t.Errorf("parsed.Model = %q, want claude-haiku-4-5", parsed.Model)
	}
}

func TestRecord_multipleCallsAccumulate(t *testing.T) {
	dir := t.TempDir()
	tracker, _ := NewWithPath("sess-2", filepath.Join(dir, "usage.jsonl"))

	tracker.Record("anthropic", "claude-haiku-4-5", 500, 100)  //nolint:errcheck
	tracker.Record("anthropic", "claude-sonnet-4-6", 800, 200) //nolint:errcheck

	summary := tracker.Summary()
	if summary.TotalTokens != 1600 {
		t.Errorf("TotalTokens = %d, want 1600", summary.TotalTokens)
	}
	if summary.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q, want claude-sonnet-4-6 (last used)", summary.Model)
	}
	if summary.TotalCostUSD == 0 {
		t.Error("TotalCostUSD should be non-zero")
	}
}

func TestRecord_unknownModelZeroCost(t *testing.T) {
	dir := t.TempDir()
	tracker, _ := NewWithPath("sess-3", filepath.Join(dir, "usage.jsonl"))

	entry, err := tracker.Record("mystery-corp", "grok-99", 1000, 500)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if entry.CostUSD != 0 {
		t.Errorf("CostUSD = %f, want 0 for unknown model", entry.CostUSD)
	}
}

func TestComputeCost_knownModel(t *testing.T) {
	// claude-haiku-4-5: $0.80/1M input, $4.00/1M output
	cost := computeCost("claude-haiku-4-5", 1_000_000, 1_000_000)
	want := 0.80 + 4.00
	if cost != want {
		t.Errorf("cost = %f, want %f", cost, want)
	}
}

func TestSummary_statusBarFormat(t *testing.T) {
	dir := t.TempDir()
	tracker, _ := NewWithPath("sess-4", filepath.Join(dir, "usage.jsonl"))
	tracker.Record("anthropic", "claude-haiku-4-5", 2000, 500) //nolint:errcheck

	s := tracker.Summary()
	if s.Model == "" {
		t.Error("Summary.Model should be populated after a Record call")
	}
	if s.TotalTokens <= 0 {
		t.Error("Summary.TotalTokens should be positive")
	}
}
