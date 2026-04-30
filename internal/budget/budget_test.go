package budget_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/budget"
	"github.com/jabreeflor/conduit/internal/config"
	"github.com/jabreeflor/conduit/internal/contracts"
)

func writeUsageLog(t *testing.T, dir string, entries []contracts.UsageEntry) string {
	t.Helper()
	path := filepath.Join(dir, "usage.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create usage log: %v", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			t.Fatalf("encode entry: %v", err)
		}
	}
	return path
}

func entry(model string, costUSD float64, at time.Time) contracts.UsageEntry {
	return contracts.UsageEntry{
		At:      at,
		Model:   model,
		CostUSD: costUSD,
	}
}

var thisMonth = time.Now()
var lastMonth = time.Now().AddDate(0, -1, 0)

// --- ReadMonthlySpend ---

func TestReadMonthlySpend_emptyLog(t *testing.T) {
	dir := t.TempDir()
	path := writeUsageLog(t, dir, nil)
	spend, err := budget.ReadMonthlySpend(path, thisMonth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spend.Overall != 0 {
		t.Errorf("overall: got %v, want 0", spend.Overall)
	}
}

func TestReadMonthlySpend_missingFile(t *testing.T) {
	spend, err := budget.ReadMonthlySpend("/no/such/path.jsonl", thisMonth)
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if spend.Overall != 0 {
		t.Errorf("expected zero spend, got %v", spend.Overall)
	}
}

func TestReadMonthlySpend_filtersCurrentMonth(t *testing.T) {
	dir := t.TempDir()
	path := writeUsageLog(t, dir, []contracts.UsageEntry{
		entry("claude-opus-4-6", 1.50, thisMonth),
		entry("gpt-4o", 0.80, thisMonth),
		entry("claude-opus-4-6", 2.00, lastMonth), // previous month — excluded
	})
	spend, err := budget.ReadMonthlySpend(path, thisMonth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	const want = 2.30
	if spend.Overall < want-0.001 || spend.Overall > want+0.001 {
		t.Errorf("overall: got %v, want %v", spend.Overall, want)
	}
	if spend.ByModel["claude-opus-4-6"] < 1.49 || spend.ByModel["claude-opus-4-6"] > 1.51 {
		t.Errorf("claude-opus-4-6: got %v, want 1.50", spend.ByModel["claude-opus-4-6"])
	}
}

// --- Check ---

func TestCheck_noLimits_alwaysAllowed(t *testing.T) {
	dir := t.TempDir()
	path := writeUsageLog(t, dir, nil)
	e := budget.NewWithLogPath(config.BudgetsConfig{}, path)

	d, err := e.Check("claude-opus-4-6", 5.00)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !d.Allowed {
		t.Error("expected allowed with no limits configured")
	}
}

func TestCheck_belowWarning(t *testing.T) {
	dir := t.TempDir()
	path := writeUsageLog(t, dir, []contracts.UsageEntry{
		entry("claude-opus-4-6", 10.00, thisMonth),
	})
	cfg := config.BudgetsConfig{
		Models: map[string]config.ModelBudget{
			"claude-opus-4-6": {MonthlyLimit: 80.00, WarningPct: 75, HardStop: true},
		},
	}
	e := budget.NewWithLogPath(cfg, path)
	d, err := e.Check("claude-opus-4-6", 0.50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !d.Allowed {
		t.Error("expected allowed")
	}
	if d.Level != budget.SpendLevelOK {
		t.Errorf("level: got %v, want SpendLevelOK", d.Level)
	}
}

func TestCheck_warningThreshold(t *testing.T) {
	dir := t.TempDir()
	// $61 of $80 = 76.25% — above 75% warning threshold
	path := writeUsageLog(t, dir, []contracts.UsageEntry{
		entry("claude-opus-4-6", 61.00, thisMonth),
	})
	cfg := config.BudgetsConfig{
		Models: map[string]config.ModelBudget{
			"claude-opus-4-6": {MonthlyLimit: 80.00, WarningPct: 75, HardStop: false},
		},
	}
	e := budget.NewWithLogPath(cfg, path)
	d, err := e.Check("claude-opus-4-6", 0.00)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !d.Allowed {
		t.Error("expected allowed at warning level")
	}
	if d.Level != budget.SpendLevelWarning {
		t.Errorf("level: got %v, want SpendLevelWarning", d.Level)
	}
}

func TestCheck_criticalThreshold(t *testing.T) {
	dir := t.TempDir()
	// $73 of $80 = 91.25% — above 90% critical threshold
	path := writeUsageLog(t, dir, []contracts.UsageEntry{
		entry("claude-opus-4-6", 73.00, thisMonth),
	})
	cfg := config.BudgetsConfig{
		Models: map[string]config.ModelBudget{
			"claude-opus-4-6": {MonthlyLimit: 80.00, WarningPct: 75, HardStop: false},
		},
	}
	e := budget.NewWithLogPath(cfg, path)
	d, err := e.Check("claude-opus-4-6", 0.00)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !d.Allowed {
		t.Error("expected allowed at critical level (no hard stop)")
	}
	if d.Level != budget.SpendLevelCritical {
		t.Errorf("level: got %v, want SpendLevelCritical", d.Level)
	}
}

func TestCheck_hardStopBlocks(t *testing.T) {
	dir := t.TempDir()
	// $79.50 spent + $1.00 call = $80.50 — over $80 limit with hard_stop
	path := writeUsageLog(t, dir, []contracts.UsageEntry{
		entry("claude-opus-4-6", 79.50, thisMonth),
	})
	cfg := config.BudgetsConfig{
		Models: map[string]config.ModelBudget{
			"claude-opus-4-6": {MonthlyLimit: 80.00, WarningPct: 75, HardStop: true},
		},
	}
	e := budget.NewWithLogPath(cfg, path)
	d, err := e.Check("claude-opus-4-6", 1.00)
	if err == nil {
		t.Fatal("expected ErrHardStop, got nil")
	}
	if !errors.Is(err, budget.ErrHardStop) {
		t.Errorf("error: got %v, want ErrHardStop", err)
	}
	if d.Allowed {
		t.Error("expected not allowed")
	}
	if d.Level != budget.SpendLevelBlocked {
		t.Errorf("level: got %v, want SpendLevelBlocked", d.Level)
	}
}

func TestCheck_overLimitNoHardStop_stillAllowed(t *testing.T) {
	dir := t.TempDir()
	path := writeUsageLog(t, dir, []contracts.UsageEntry{
		entry("gpt-4o", 85.00, thisMonth),
	})
	cfg := config.BudgetsConfig{
		Models: map[string]config.ModelBudget{
			"gpt-4o": {MonthlyLimit: 50.00, WarningPct: 75, HardStop: false},
		},
	}
	e := budget.NewWithLogPath(cfg, path)
	d, err := e.Check("gpt-4o", 0.00)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !d.Allowed {
		t.Error("expected allowed when hard_stop is false")
	}
	if d.Level != budget.SpendLevelCritical {
		t.Errorf("level: got %v, want SpendLevelCritical (over limit, no hard stop)", d.Level)
	}
}

func TestCheck_modelNotConfigured_usesOverall(t *testing.T) {
	dir := t.TempDir()
	path := writeUsageLog(t, dir, []contracts.UsageEntry{
		entry("claude-haiku-4-5", 160.00, thisMonth),
	})
	cfg := config.BudgetsConfig{
		Overall: config.BudgetLimit{MonthlyLimit: 200.00},
	}
	e := budget.NewWithLogPath(cfg, path)
	// $160 + $1 = $161 of $200 = 80.5% → warning (default 75%)
	d, err := e.Check("claude-haiku-4-5", 1.00)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !d.Allowed {
		t.Error("expected allowed")
	}
	if d.Level != budget.SpendLevelWarning {
		t.Errorf("level: got %v, want SpendLevelWarning", d.Level)
	}
	if d.Key != "overall" {
		t.Errorf("key: got %q, want %q", d.Key, "overall")
	}
}

// --- Report ---

func TestReport_allModels(t *testing.T) {
	dir := t.TempDir()
	path := writeUsageLog(t, dir, []contracts.UsageEntry{
		entry("claude-opus-4-6", 64.00, thisMonth),
		entry("gpt-4o", 20.00, thisMonth),
	})
	cfg := config.BudgetsConfig{
		Overall: config.BudgetLimit{MonthlyLimit: 200.00},
		Models: map[string]config.ModelBudget{
			"claude-opus-4-6": {MonthlyLimit: 80.00, WarningPct: 75, HardStop: true},
			"gpt-4o":          {MonthlyLimit: 50.00, WarningPct: 75, HardStop: false},
		},
	}
	e := budget.NewWithLogPath(cfg, path)
	r := e.Report()

	opusReport, ok := r.ByModel["claude-opus-4-6"]
	if !ok {
		t.Fatal("report missing claude-opus-4-6")
	}
	// 64/80 = 80% → warning
	if opusReport.Level != budget.SpendLevelWarning {
		t.Errorf("opus level: got %v, want SpendLevelWarning", opusReport.Level)
	}
	if !opusReport.HardStop {
		t.Error("opus hard_stop: expected true")
	}

	gptReport, ok := r.ByModel["gpt-4o"]
	if !ok {
		t.Fatal("report missing gpt-4o")
	}
	// 20/50 = 40% → OK
	if gptReport.Level != budget.SpendLevelOK {
		t.Errorf("gpt level: got %v, want SpendLevelOK", gptReport.Level)
	}

	// overall: 84/200 = 42% → OK
	if r.Overall.Level != budget.SpendLevelOK {
		t.Errorf("overall level: got %v, want SpendLevelOK", r.Overall.Level)
	}
}

func TestReport_overallProjectedOvershoot(t *testing.T) {
	if time.Now().Day() < 2 {
		t.Skip("projection test requires day ≥ 2 to have a non-zero daily rate")
	}
	dir := t.TempDir()
	// Spend 99.5% of limit: remaining $0.40 at any daily rate > $0.40/day means
	// overshoot is within today — reliably this month regardless of date.
	path := writeUsageLog(t, dir, []contracts.UsageEntry{
		entry("claude-opus-4-6", 79.60, thisMonth),
	})
	cfg := config.BudgetsConfig{
		Models: map[string]config.ModelBudget{
			"claude-opus-4-6": {MonthlyLimit: 80.00, WarningPct: 75, HardStop: false},
		},
	}
	e := budget.NewWithLogPath(cfg, path)
	r := e.Report()

	opusReport := r.ByModel["claude-opus-4-6"]
	if opusReport.ProjectedOvershootDate == nil {
		t.Error("expected a projected overshoot date when spend is 99.5% of limit")
	}
}
