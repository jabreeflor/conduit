package gui

import (
	"testing"
	"time"
)

func sampleAt(when time.Time, model string, cost float64, latMS int64, err bool) UsageSample {
	return UsageSample{
		Timestamp: when,
		Provider:  "anthropic",
		Model:     model,
		Feature:   "chat",
		Calls:     1,
		Tokens:    1000,
		CostUSD:   cost,
		LatencyMS: latMS,
		Error:     err,
	}
}

func TestUsageDashboard_CostOverviewSumsFiltered(t *testing.T) {
	d := NewUsageDashboard()
	now := time.Now()
	d.Ingest(sampleAt(now, "opus", 0.50, 100, false))
	d.Ingest(sampleAt(now.Add(-2*24*time.Hour), "sonnet", 0.10, 80, false))

	d.SetFilter(UsageFilter{From: now.Add(-24 * time.Hour)})
	got := d.Panel(PanelCostOverview)
	if got.Total != 0.50 {
		t.Errorf("total = %f, want 0.50 (older sample filtered out)", got.Total)
	}
}

func TestUsageDashboard_CostByModelSorted(t *testing.T) {
	d := NewUsageDashboard()
	now := time.Now()
	d.Ingest(sampleAt(now, "sonnet", 0.10, 50, false))
	d.Ingest(sampleAt(now, "opus", 1.00, 50, false))
	d.Ingest(sampleAt(now, "opus", 0.50, 50, false))

	got := d.Panel(PanelCostByModel)
	if len(got.Series) != 2 {
		t.Fatalf("series len = %d, want 2", len(got.Series))
	}
	if got.Series[0].Label != "anthropic/opus" {
		t.Errorf("top label = %q, want anthropic/opus", got.Series[0].Label)
	}
	if got.Series[0].Value != 1.50 {
		t.Errorf("top value = %f, want 1.50", got.Series[0].Value)
	}
}

func TestUsageDashboard_LatencyPercentiles(t *testing.T) {
	d := NewUsageDashboard()
	now := time.Now()
	for i := int64(1); i <= 100; i++ {
		d.Ingest(sampleAt(now, "opus", 0.01, i, false))
	}
	got := d.Panel(PanelLatencyPercentiles)
	// p50 around 50, p95 around 95, p99 around 99.
	if got.P50 < 40 || got.P50 > 60 {
		t.Errorf("p50 = %f", got.P50)
	}
	if got.P95 < 90 || got.P95 > 100 {
		t.Errorf("p95 = %f", got.P95)
	}
	if got.P99 < 95 {
		t.Errorf("p99 = %f", got.P99)
	}
}

func TestUsageDashboard_ErrorRate(t *testing.T) {
	d := NewUsageDashboard()
	now := time.Now()
	for i := 0; i < 9; i++ {
		d.Ingest(sampleAt(now, "opus", 0.01, 10, false))
	}
	d.Ingest(sampleAt(now, "opus", 0.01, 10, true))

	got := d.Panel(PanelErrorRate)
	if got.Total < 0.099 || got.Total > 0.101 {
		t.Errorf("error rate = %f, want ~0.10", got.Total)
	}
}

func TestUsageDashboard_RequestVolumeBucketsByDay(t *testing.T) {
	d := NewUsageDashboard()
	day1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	day2 := day1.Add(24 * time.Hour)
	d.Ingest(sampleAt(day1, "opus", 0, 0, false))
	d.Ingest(sampleAt(day1.Add(time.Hour), "opus", 0, 0, false))
	d.Ingest(sampleAt(day2, "opus", 0, 0, false))

	got := d.Panel(PanelRequestVolume)
	if len(got.TimeSeries) != 2 {
		t.Fatalf("buckets = %d, want 2", len(got.TimeSeries))
	}
	if got.TimeSeries[0].Value != 2 {
		t.Errorf("day1 calls = %f, want 2", got.TimeSeries[0].Value)
	}
}

func TestUsageDashboard_ModelComparisonRows(t *testing.T) {
	d := NewUsageDashboard()
	now := time.Now()
	d.Ingest(sampleAt(now, "opus", 1.00, 200, false))
	d.Ingest(sampleAt(now, "sonnet", 0.20, 80, false))
	d.Ingest(sampleAt(now, "opus", 0.50, 100, true))

	got := d.Panel(PanelModelComparison)
	if len(got.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(got.Rows))
	}
	// Sorted by cost descending — opus first.
	if got.Rows[0].Model != "opus" {
		t.Errorf("top row model = %q, want opus", got.Rows[0].Model)
	}
	if got.Rows[0].Calls != 2 {
		t.Errorf("opus calls = %d, want 2", got.Rows[0].Calls)
	}
	if got.Rows[0].ErrorPct < 0.49 || got.Rows[0].ErrorPct > 0.51 {
		t.Errorf("opus error pct = %f, want 0.50", got.Rows[0].ErrorPct)
	}
}

func TestUsageDashboard_RangePresetSetsFilter(t *testing.T) {
	d := NewUsageDashboard()
	d.SetRange(RangeLast24h)
	f := d.Filter()
	if f.From.IsZero() || f.To.IsZero() {
		t.Error("Last24h preset should populate From/To")
	}
	d.SetRange(RangeAllTime)
	f = d.Filter()
	if !f.From.IsZero() {
		t.Error("AllTime should clear From")
	}
}

func TestUsageDashboard_BudgetStatus(t *testing.T) {
	d := NewUsageDashboard()
	d.SetBudget(100.0)
	now := time.Now()
	d.Ingest(sampleAt(now, "opus", 30.0, 10, false))
	d.Ingest(sampleAt(now, "opus", 15.0, 10, false))

	got := d.Panel(PanelBudgetStatus)
	if got.Total != 45.0 {
		t.Errorf("spent = %f, want 45", got.Total)
	}
	if got.P50 != 100.0 {
		t.Errorf("budget = %f, want 100", got.P50)
	}
}

func TestUsageDashboard_DrillDownMatchesFilter(t *testing.T) {
	d := NewUsageDashboard()
	now := time.Now()
	d.Ingest(sampleAt(now, "opus", 1, 10, false))
	d.Ingest(sampleAt(now, "sonnet", 0.5, 10, false))

	d.SetFilter(UsageFilter{Model: "opus"})
	rows := d.DrillDown()
	if len(rows) != 1 || rows[0].Model != "opus" {
		t.Errorf("drill-down = %+v, want one opus row", rows)
	}
}

func TestUsageDashboard_PluginUsageGroupsNone(t *testing.T) {
	d := NewUsageDashboard()
	now := time.Now()
	a := sampleAt(now, "opus", 1, 10, false)
	b := sampleAt(now, "opus", 2, 10, false)
	b.Plugin = "github"
	d.Ingest(a)
	d.Ingest(b)

	got := d.Panel(PanelPluginUsage)
	if len(got.Series) != 2 {
		t.Fatalf("series = %d, want 2", len(got.Series))
	}
	// github first (higher cost).
	if got.Series[0].Label != "github" {
		t.Errorf("top plugin = %q, want github", got.Series[0].Label)
	}
}

func TestUsageDashboard_Concurrent(t *testing.T) {
	d := NewUsageDashboard()
	now := time.Now()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			d.Ingest(sampleAt(now, "opus", 0.01, 10, false))
		}
		done <- struct{}{}
	}()
	for i := 0; i < 200; i++ {
		_ = d.Panel(PanelCostOverview)
	}
	<-done
}
