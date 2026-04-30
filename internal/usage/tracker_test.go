package usage

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestRecord_appendsJSONL(t *testing.T) {
	dir := t.TempDir()

	tracker, err := NewWithDir("sess-1", dir)
	if err != nil {
		t.Fatalf("NewWithDir: %v", err)
	}
	tracker.now = func() time.Time { return time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC) }

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
	if entry.TokensIn != 1000 || entry.TokensOut != 200 {
		t.Errorf("tokens = %d/%d, want 1000/200", entry.TokensIn, entry.TokensOut)
	}
	if entry.Status != "success" {
		t.Errorf("Status = %q, want success", entry.Status)
	}
	if entry.CostUSD == 0 {
		t.Error("CostUSD should be non-zero for a known model")
	}

	logPath := filepath.Join(dir, "2026-04-30.jsonl")
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
	if parsed.Timestamp.IsZero() {
		t.Error("parsed.Timestamp should be populated")
	}
}

func TestRecordEvent_capturesRequestMetrics(t *testing.T) {
	dir := t.TempDir()
	tracker, _ := NewWithDir("sess-metrics", dir)
	at := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)

	entry, err := tracker.RecordEvent(Record{
		Provider:     "anthropic",
		Model:        "claude-haiku-4-5",
		TokensIn:     100,
		TokensOut:    50,
		TTFT:         150 * time.Millisecond,
		TotalLatency: 2 * time.Second,
		Feature:      "code",
		Plugin:       "reviewer",
		Timestamp:    at,
	})
	if err != nil {
		t.Fatalf("RecordEvent: %v", err)
	}

	if entry.TTFMS != 150 {
		t.Errorf("TTFMS = %d, want 150", entry.TTFMS)
	}
	if entry.TotalMS != 2000 {
		t.Errorf("TotalMS = %d, want 2000", entry.TotalMS)
	}
	if entry.TokensPerSecond != 25 {
		t.Errorf("TokensPerSecond = %f, want 25", entry.TokensPerSecond)
	}
	if entry.Feature != "code" || entry.Plugin != "reviewer" {
		t.Errorf("metadata = %q/%q, want code/reviewer", entry.Feature, entry.Plugin)
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

func TestRecord_usesProviderPricingTableOverride(t *testing.T) {
	dir := t.TempDir()
	tracker, err := NewWithPathAndOptions("sess-pricing", filepath.Join(dir, "usage.jsonl"), Options{
		Pricing: PricingTable{Entries: []Pricing{
			{Provider: "custom", Model: "tiny", InputPer1MUSD: 1.50, OutputPer1MUSD: 2.50},
		}},
	})
	if err != nil {
		t.Fatalf("NewWithPathAndOptions: %v", err)
	}

	entry, err := tracker.Record("custom", "tiny", 1_000_000, 2_000_000)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if entry.CostUSD != 6.50 {
		t.Errorf("CostUSD = %f, want 6.50", entry.CostUSD)
	}
	if entry.CostSource != "pricing_table" {
		t.Errorf("CostSource = %q, want pricing_table", entry.CostSource)
	}
}

func TestLoadPricingTable_userEditableJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pricing.json")
	if err := os.WriteFile(path, []byte(`{"entries":[{"provider":"openai","model":"custom-gpt","input_per_1m_usd":2,"output_per_1m_usd":4}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	table, err := LoadPricingTable(path)
	if err != nil {
		t.Fatalf("LoadPricingTable: %v", err)
	}
	price, ok := table.Lookup("openai", "custom-gpt")
	if !ok {
		t.Fatal("Lookup(openai, custom-gpt) missing")
	}
	if price.InputPer1MUSD != 2 || price.OutputPer1MUSD != 4 {
		t.Errorf("price = %+v, want input=2 output=4", price)
	}
}

func TestRecordWithOptions_localModelEnergyEstimate(t *testing.T) {
	dir := t.TempDir()
	tracker, err := NewWithPathAndOptions("sess-local", filepath.Join(dir, "usage.jsonl"), Options{
		ElectricityRateUSDPerKWh: 0.20,
		MachineProfile: contracts.MachineProfile{
			ProfiledAt: time.Now(),
			Memory:     contracts.MemInfo{TotalGB: 32},
		},
	})
	if err != nil {
		t.Fatalf("NewWithPathAndOptions: %v", err)
	}

	entry, err := tracker.RecordWithOptions("ollama", "llama3", 100, 50, RecordOptions{
		LocalModel:        true,
		InferenceDuration: time.Hour,
	})
	if err != nil {
		t.Fatalf("RecordWithOptions: %v", err)
	}
	want := 0.013 // 65W for one hour at $0.20/kWh.
	if math.Abs(entry.CostUSD-want) > 0.000001 {
		t.Errorf("CostUSD = %f, want %f", entry.CostUSD, want)
	}
	if !entry.CostEstimated {
		t.Error("CostEstimated should be true for local model costs")
	}
	if entry.CostSource != "local_energy_estimate" {
		t.Errorf("CostSource = %q, want local_energy_estimate", entry.CostSource)
	}
	if !entry.LocalComparisonEstimated {
		t.Error("LocalComparisonEstimated should be true")
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

func TestDailyLogs_compressAndRetain(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "2026-04-20.jsonl")
	expired := filepath.Join(dir, "2026-01-01.jsonl")
	if err := os.WriteFile(old, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(expired, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tracker, _ := NewWithDir("sess-retain", dir)
	tracker.now = func() time.Time { return time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC) }
	if _, err := tracker.Record("anthropic", "claude-haiku-4-5", 1, 1); err != nil {
		t.Fatalf("Record: %v", err)
	}

	if _, err := os.Stat(expired); !os.IsNotExist(err) {
		t.Fatalf("expired log still exists or stat failed: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old log should be compressed and removed, stat: %v", err)
	}
	gzPath := old + ".gz"
	gzFile, err := os.Open(gzPath)
	if err != nil {
		t.Fatalf("open compressed log: %v", err)
	}
	defer gzFile.Close()
	gz, err := gzip.NewReader(gzFile)
	if err != nil {
		t.Fatalf("read compressed log: %v", err)
	}
	defer gz.Close()
	data, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("read gzip data: %v", err)
	}
	if string(data) != "{}\n" {
		t.Errorf("compressed data = %q, want original JSONL", data)
	}
}
