package efficiency

import (
	"testing"
	"time"
)

func mockMonitor(events []TaskEvent, now time.Time) *Monitor {
	m := New(Config{Now: func() time.Time { return now }})
	for _, e := range events {
		m.Record(e)
	}
	return m
}

func TestSnapshotComputesAggregateMetrics(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	events := []TaskEvent{
		{TaskID: "1", InputTokens: 1000, OutputTokens: 200, CostUSD: 0.05, Success: true,
			CacheHits: 1, CacheLookups: 2, ContextTokens: 800, ContextDropped: 200, RecordedAt: now.Add(-time.Hour)},
		{TaskID: "2", InputTokens: 500, OutputTokens: 100, CostUSD: 0.02, Success: true,
			CacheHits: 0, CacheLookups: 1, ContextTokens: 400, ContextDropped: 0, RecordedAt: now.Add(-time.Minute)},
		{TaskID: "3", Success: false, Retries: 2, Escalated: true, RecordedAt: now},
	}
	m := mockMonitor(events, now)
	snap := m.Snapshot()
	if snap.Tasks != 3 || snap.SuccessfulTasks != 2 {
		t.Fatalf("Tasks=%d Successful=%d", snap.Tasks, snap.SuccessfulTasks)
	}
	if snap.TokensPerSuccessTask != float64(1000+200+500+100)/2 {
		t.Fatalf("TokensPerSuccessTask = %v", snap.TokensPerSuccessTask)
	}
	if snap.CostPerSuccessTask != 0.035 {
		t.Fatalf("CostPerSuccessTask = %v, want 0.035", snap.CostPerSuccessTask)
	}
	if snap.CacheHitRate != float64(1)/3 {
		t.Fatalf("CacheHitRate = %v", snap.CacheHitRate)
	}
	if snap.EscalationRate != 1.0/3 {
		t.Fatalf("EscalationRate = %v", snap.EscalationRate)
	}
	if snap.RetryRate != 2.0/3 {
		t.Fatalf("RetryRate = %v", snap.RetryRate)
	}
	if snap.RoutingEfficiency != 1-snap.EscalationRate {
		t.Fatalf("RoutingEfficiency = %v", snap.RoutingEfficiency)
	}
}

func TestMonitorEvictsOldEvents(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	m := New(Config{Window: time.Hour, Now: func() time.Time { return now }})
	m.Record(TaskEvent{TaskID: "old", RecordedAt: now.Add(-2 * time.Hour)})
	m.Record(TaskEvent{TaskID: "fresh", RecordedAt: now.Add(-time.Minute)})
	snap := m.Snapshot()
	if snap.Tasks != 1 {
		t.Fatalf("Tasks = %d, want 1 (old should evict)", snap.Tasks)
	}
}

func TestAutoTunerShrinksContextOnHighWaste(t *testing.T) {
	tuner := NewAutoTuner(Bounds{})
	current := Settings{ContextBudgetTokens: 50_000, RelevanceThreshold: 0.3, CacheTTLSeconds: 600, CascadeMinConfidence: 0.5}
	m := Metrics{Tasks: 30, SuccessfulTasks: 25, WasteRatio: 0.6}
	adj := tuner.Tune(current, m)
	if !adj.Changed {
		t.Fatal("expected change")
	}
	if adj.NewSettings.ContextBudgetTokens >= current.ContextBudgetTokens {
		t.Fatalf("context budget not shrunk: %d", adj.NewSettings.ContextBudgetTokens)
	}
	if adj.NewSettings.RelevanceThreshold <= current.RelevanceThreshold {
		t.Fatalf("relevance not raised: %v", adj.NewSettings.RelevanceThreshold)
	}
	if len(adj.Reasons) < 2 {
		t.Fatalf("Reasons = %v, want at least 2", adj.Reasons)
	}
}

func TestAutoTunerGrowsContextOnLowWaste(t *testing.T) {
	tuner := NewAutoTuner(Bounds{})
	current := Settings{ContextBudgetTokens: 50_000, RelevanceThreshold: 0.3, CacheTTLSeconds: 600, CascadeMinConfidence: 0.5}
	m := Metrics{Tasks: 30, SuccessfulTasks: 25, WasteRatio: 0.05}
	adj := tuner.Tune(current, m)
	if adj.NewSettings.ContextBudgetTokens <= current.ContextBudgetTokens {
		t.Fatalf("context budget not grown: %d", adj.NewSettings.ContextBudgetTokens)
	}
}

func TestAutoTunerGrowsCacheTTLOnLowHitRate(t *testing.T) {
	tuner := NewAutoTuner(Bounds{})
	current := Settings{CacheTTLSeconds: 600}
	m := Metrics{Tasks: 30, SuccessfulTasks: 25, CacheHitRate: 0.1}
	adj := tuner.Tune(current, m)
	if adj.NewSettings.CacheTTLSeconds <= current.CacheTTLSeconds {
		t.Fatalf("TTL not grown: %d", adj.NewSettings.CacheTTLSeconds)
	}
}

func TestAutoTunerLowersCascadeOnHighEscalation(t *testing.T) {
	tuner := NewAutoTuner(Bounds{})
	current := Settings{CascadeMinConfidence: 0.7}
	m := Metrics{Tasks: 30, SuccessfulTasks: 25, EscalationRate: 0.8}
	adj := tuner.Tune(current, m)
	if adj.NewSettings.CascadeMinConfidence >= current.CascadeMinConfidence {
		t.Fatalf("cascade conf not lowered: %v", adj.NewSettings.CascadeMinConfidence)
	}
}

func TestAutoTunerRaisesCascadeOnLowEscalation(t *testing.T) {
	tuner := NewAutoTuner(Bounds{})
	current := Settings{CascadeMinConfidence: 0.4}
	m := Metrics{Tasks: 30, SuccessfulTasks: 25, EscalationRate: 0.02}
	adj := tuner.Tune(current, m)
	if adj.NewSettings.CascadeMinConfidence <= current.CascadeMinConfidence {
		t.Fatalf("cascade conf not raised: %v", adj.NewSettings.CascadeMinConfidence)
	}
}

func TestAutoTunerRespectsBounds(t *testing.T) {
	tuner := NewAutoTuner(Bounds{MinContextBudget: 40_000, MaxContextBudget: 60_000})
	current := Settings{ContextBudgetTokens: 41_000, RelevanceThreshold: 0.3}
	// massive waste — would shrink below MinContextBudget if unclamped
	m := Metrics{Tasks: 30, SuccessfulTasks: 25, WasteRatio: 0.9}
	adj := tuner.Tune(current, m)
	if adj.NewSettings.ContextBudgetTokens < 40_000 {
		t.Fatalf("budget clamp violated: %d", adj.NewSettings.ContextBudgetTokens)
	}
}

func TestAutoTunerNoOpWhenStable(t *testing.T) {
	tuner := NewAutoTuner(Bounds{})
	current := Settings{ContextBudgetTokens: 50_000, RelevanceThreshold: 0.3, CacheTTLSeconds: 600, CascadeMinConfidence: 0.5}
	m := Metrics{Tasks: 5, SuccessfulTasks: 5, WasteRatio: 0.2, CacheHitRate: 0.4, EscalationRate: 0.1}
	adj := tuner.Tune(current, m)
	if adj.Changed {
		t.Fatalf("unexpected change: %#v reasons=%v", adj.NewSettings, adj.Reasons)
	}
}

func TestBuildWeeklyReportBucketsEvents(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	m := New(Config{Window: 60 * 24 * time.Hour, Now: func() time.Time { return now }})
	m.Record(TaskEvent{TaskID: "thisweek", Success: true, InputTokens: 100, OutputTokens: 100, RecordedAt: now.Add(-2 * 24 * time.Hour)})
	m.Record(TaskEvent{TaskID: "lastweek", Success: true, InputTokens: 200, OutputTokens: 200, RecordedAt: now.Add(-9 * 24 * time.Hour)})
	rep, err := BuildWeeklyReport(m, now, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Weeks) != 4 {
		t.Fatalf("weeks = %d, want 4", len(rep.Weeks))
	}
	// most recent week is last
	last := rep.Weeks[3]
	if last.Metrics.Tasks != 1 {
		t.Fatalf("last bucket tasks = %d, want 1", last.Metrics.Tasks)
	}
	prior := rep.Weeks[2]
	if prior.Metrics.Tasks != 1 {
		t.Fatalf("prior bucket tasks = %d, want 1", prior.Metrics.Tasks)
	}
}

func TestBuildWeeklyReportRejectsNilMonitor(t *testing.T) {
	if _, err := BuildWeeklyReport(nil, time.Now(), 4); err == nil {
		t.Fatal("expected nil-monitor error")
	}
}

func TestBudgetExceedRateCounted(t *testing.T) {
	now := time.Now().UTC()
	m := mockMonitor([]TaskEvent{
		{TaskID: "a", Success: true, BudgetUSD: 0.1, CostUSD: 0.2, RecordedAt: now},
		{TaskID: "b", Success: true, BudgetUSD: 0.1, CostUSD: 0.05, RecordedAt: now},
	}, now)
	snap := m.Snapshot()
	if snap.BudgetExceedRate != 0.5 {
		t.Fatalf("BudgetExceedRate = %v, want 0.5", snap.BudgetExceedRate)
	}
}
