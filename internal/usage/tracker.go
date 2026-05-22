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

const (
	defaultPricingPath    = ".conduit/pricing.json"
	defaultCurrency       = "USD"
	defaultElectricityUSD = 0.16
)

// Pricing holds input/output cost per 1M tokens in USD.
type Pricing struct {
	Provider       string  `json:"provider"`
	Model          string  `json:"model"`
	InputPer1MUSD  float64 `json:"input_per_1m_usd"`
	OutputPer1MUSD float64 `json:"output_per_1m_usd"`
	Currency       string  `json:"currency,omitempty"`
}

// PricingTable is a local, user-updateable provider/model price index.
type PricingTable struct {
	Entries []Pricing `json:"entries"`
	index   map[string]Pricing
}

const (
	defaultLogDir        = ".conduit/logs/usage"
	defaultCompressAfter = 7 * 24 * time.Hour
	defaultRetention     = 90 * 24 * time.Hour
)

// Options customizes usage accounting.
type Options struct {
	PricingPath              string
	Pricing                  PricingTable
	ElectricityRateUSDPerKWh float64
	MachineProfile           contracts.MachineProfile
}

// RecordOptions carries optional request details for richer cost accounting.
type RecordOptions struct {
	InferenceDuration        time.Duration
	LocalModel               bool
	MachineProfile           contracts.MachineProfile
	ElectricityRateUSDPerKWh float64
}

// Tracker appends usage entries to disk and maintains in-memory session totals.
// All methods are safe for concurrent use.
type Tracker struct {
	mu                 sync.Mutex
	sessionID          string
	logPath            string
	logDir             string
	compressAfter      time.Duration
	retention          time.Duration
	now                func() time.Time
	pricing            PricingTable
	electricityRateUSD float64
	machineProfile     contracts.MachineProfile
	summary            contracts.UsageSummary
}

// New creates a Tracker that writes daily logs under ~/.conduit/logs/usage.
func New(sessionID string) (*Tracker, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("usage: resolve home dir: %w", err)
	}
	return NewWithDirAndOptions(sessionID, filepath.Join(home, defaultLogDir), Options{
		PricingPath: filepath.Join(home, defaultPricingPath),
	})
}

// NewWithPath creates a Tracker writing to an explicit path (useful in tests).
func NewWithPath(sessionID, logPath string) (*Tracker, error) {
	return NewWithPathAndOptions(sessionID, logPath, Options{})
}

// NewWithPathAndOptions creates a Tracker writing to an explicit path with
// configurable pricing and local energy assumptions.
func NewWithPathAndOptions(sessionID, logPath string, opts Options) (*Tracker, error) {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, fmt.Errorf("usage: create log dir: %w", err)
	}
	return newTracker(sessionID, logPath, "", opts)
}

// NewWithDir creates a Tracker writing YYYY-MM-DD.jsonl files under logDir.
func NewWithDir(sessionID, logDir string) (*Tracker, error) {
	return NewWithDirAndOptions(sessionID, logDir, Options{})
}

// NewWithDirAndOptions creates a Tracker writing YYYY-MM-DD.jsonl files under
// logDir with configurable pricing and local energy assumptions.
func NewWithDirAndOptions(sessionID, logDir string, opts Options) (*Tracker, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("usage: create log dir: %w", err)
	}
	return newTracker(sessionID, "", logDir, opts)
}

func newTracker(sessionID, logPath, logDir string, opts Options) (*Tracker, error) {
	pricing := opts.Pricing
	if len(pricing.Entries) == 0 {
		pricing = DefaultPricingTable()
	}
	if opts.PricingPath != "" {
		if loaded, err := LoadPricingTable(opts.PricingPath); err == nil {
			pricing = loaded
		}
	}
	pricing.buildIndex()

	rate := opts.ElectricityRateUSDPerKWh
	if rate <= 0 {
		rate = defaultElectricityUSD
	}

	return &Tracker{
		sessionID:          sessionID,
		logPath:            logPath,
		logDir:             logDir,
		compressAfter:      defaultCompressAfter,
		retention:          defaultRetention,
		now:                func() time.Time { return time.Now().UTC() },
		pricing:            pricing,
		electricityRateUSD: rate,
		machineProfile:     opts.MachineProfile,
	}, nil
}

// Record appends one model-call entry to the JSONL log and updates session totals.
func (t *Tracker) Record(provider, model string, inputTokens, outputTokens int) (contracts.UsageEntry, error) {
	return t.RecordWithOptions(provider, model, inputTokens, outputTokens, RecordOptions{})
}

// RecordWithOptions appends one model-call entry with optional local estimate
// fields used for API-vs-local cost comparisons.
func (t *Tracker) RecordWithOptions(provider, model string, inputTokens, outputTokens int, opts RecordOptions) (contracts.UsageEntry, error) {
	return t.RecordEvent(Record{
		Provider:  provider,
		Model:     model,
		TokensIn:  inputTokens,
		TokensOut: outputTokens,
		Status:    "success",
		Options:   opts,
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
	Options      RecordOptions
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
	entryCost := record.CostUSD
	if entryCost == 0 {
		entryCost = t.apiCost(record.Provider, record.Model, record.TokensIn, record.TokensOut)
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
		CostUSD:         entryCost,
		CostCurrency:    defaultCurrency,
		CostSource:      "pricing_table",
	}
	if entryCost == 0 {
		entry.CostSource = "unknown"
	}

	opts := record.Options
	localCost, watts, rate := t.localEstimate(opts)
	if opts.InferenceDuration > 0 {
		entry.InferenceSeconds = opts.InferenceDuration.Seconds()
	}
	if localCost > 0 {
		entry.EstimatedLocalCostUSD = localCost
		entry.LocalComparisonEstimated = true
		entry.EstimatedPowerDrawWatts = watts
		entry.ElectricityRateUSDPerKWh = rate
	}
	if opts.LocalModel {
		entry.CostUSD = localCost
		entry.CostEstimated = true
		entry.CostSource = "local_energy_estimate"
	}

	if err := t.appendLine(entry); err != nil {
		return entry, err
	}

	t.mu.Lock()
	t.summary.Model = entry.Model
	t.summary.TotalTokens += entry.TotalTokens
	t.summary.TotalCostUSD += entry.CostUSD
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
func (t *Tracker) apiCost(provider, model string, inputTokens, outputTokens int) float64 {
	p, ok := t.pricing.Lookup(provider, model)
	if !ok {
		return 0
	}
	return computeCostFromPricing(p, inputTokens, outputTokens)
}

func (t *Tracker) localEstimate(opts RecordOptions) (costUSD, watts, rate float64) {
	if opts.InferenceDuration <= 0 {
		return 0, 0, 0
	}
	profile := opts.MachineProfile
	if profile.ProfiledAt.IsZero() {
		profile = t.machineProfile
	}
	watts = EstimatePowerDrawWatts(profile)
	rate = opts.ElectricityRateUSDPerKWh
	if rate <= 0 {
		rate = t.electricityRateUSD
	}
	hours := opts.InferenceDuration.Hours()
	return (watts / 1000) * hours * rate, watts, rate
}

// DefaultPricingTable returns bundled fallback prices for common API models.
func DefaultPricingTable() PricingTable {
	return PricingTable{Entries: []Pricing{
		{Provider: "anthropic", Model: "claude-haiku-4-5", InputPer1MUSD: 0.80, OutputPer1MUSD: 4.00, Currency: defaultCurrency},
		{Provider: "anthropic", Model: "claude-sonnet-4-6", InputPer1MUSD: 3.00, OutputPer1MUSD: 15.00, Currency: defaultCurrency},
		{Provider: "anthropic", Model: "claude-opus-4-6", InputPer1MUSD: 15.00, OutputPer1MUSD: 75.00, Currency: defaultCurrency},
		{Provider: "anthropic", Model: "claude-opus-4-7", InputPer1MUSD: 15.00, OutputPer1MUSD: 75.00, Currency: defaultCurrency},
		{Provider: "openai", Model: "gpt-4o", InputPer1MUSD: 5.00, OutputPer1MUSD: 15.00, Currency: defaultCurrency},
		{Provider: "openai", Model: "gpt-4o-mini", InputPer1MUSD: 0.15, OutputPer1MUSD: 0.60, Currency: defaultCurrency},
		{Provider: "google", Model: "gemini-1.5-pro", InputPer1MUSD: 3.50, OutputPer1MUSD: 10.50, Currency: defaultCurrency},
		{Provider: "google", Model: "gemini-1.5-flash", InputPer1MUSD: 0.075, OutputPer1MUSD: 0.30, Currency: defaultCurrency},
		{Provider: "mistral", Model: "mistral-large-latest", InputPer1MUSD: 2.00, OutputPer1MUSD: 6.00, Currency: defaultCurrency},
		{Provider: "mistral", Model: "codestral-latest", InputPer1MUSD: 0.30, OutputPer1MUSD: 0.90, Currency: defaultCurrency},
	}}
}

// LoadPricingTable reads a user-editable JSON pricing table from disk.
func LoadPricingTable(path string) (PricingTable, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PricingTable{}, err
	}
	var table PricingTable
	if err := json.Unmarshal(data, &table); err != nil {
		return PricingTable{}, fmt.Errorf("usage: decode pricing table: %w", err)
	}
	table.buildIndex()
	return table, nil
}

// Lookup returns the provider/model price, falling back to model-only matches.
func (p *PricingTable) Lookup(provider, model string) (Pricing, bool) {
	p.buildIndex()
	if pricing, ok := p.index[pricingKey(provider, model)]; ok {
		return pricing, true
	}
	pricing, ok := p.index[pricingKey("", model)]
	return pricing, ok
}

func (p *PricingTable) buildIndex() {
	if p.index != nil {
		return
	}
	p.index = make(map[string]Pricing, len(p.Entries)*2)
	for _, entry := range p.Entries {
		if entry.Currency == "" {
			entry.Currency = defaultCurrency
		}
		if entry.Model == "" {
			continue
		}
		p.index[pricingKey(entry.Provider, entry.Model)] = entry
		if entry.Provider != "" {
			if _, exists := p.index[pricingKey("", entry.Model)]; !exists {
				p.index[pricingKey("", entry.Model)] = entry
			}
		}
	}
}

func pricingKey(provider, model string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + "/" + strings.ToLower(strings.TrimSpace(model))
}

func computeCost(model string, inputTokens, outputTokens int) float64 {
	table := DefaultPricingTable()
	p, ok := table.Lookup("", model)
	if !ok {
		return 0
	}
	return computeCostFromPricing(p, inputTokens, outputTokens)
}

func computeCostFromPricing(p Pricing, inputTokens, outputTokens int) float64 {
	return float64(inputTokens)/1_000_000*p.InputPer1MUSD +
		float64(outputTokens)/1_000_000*p.OutputPer1MUSD
}

// EstimatePowerDrawWatts derives a coarse inference wattage from a cached
// machine profile. It is intentionally approximate for local/cloud comparisons.
func EstimatePowerDrawWatts(profile contracts.MachineProfile) float64 {
	for _, gpu := range profile.GPU {
		if strings.EqualFold(gpu.VRAMType, "dedicated") {
			if gpu.VRAMGB >= 16 {
				return 260
			}
			return 180
		}
	}
	if profile.Memory.TotalGB >= 64 {
		return 90
	}
	if profile.Memory.TotalGB >= 32 {
		return 65
	}
	if profile.CPU.PhysicalCores >= 10 {
		return 55
	}
	return 35
}
