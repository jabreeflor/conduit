package trends

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func writeLog(t *testing.T, dir, name string, entries []contracts.UsageEntry) {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
}

// Build an N-day series with controllable per-day attributes. base is a
// Monday so day 0 / day 7 / day 14 / day 21 line up cleanly on week starts.
func seedDirN(t *testing.T, days int, fn func(day int) []contracts.UsageEntry) string {
	t.Helper()
	dir := t.TempDir()
	// 2026-04-06 is a Monday in UTC.
	base := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	all := []contracts.UsageEntry{}
	for d := 0; d < days; d++ {
		day := base.AddDate(0, 0, d)
		for _, e := range fn(d) {
			e.Timestamp = day
			all = append(all, e)
		}
	}
	writeLog(t, dir, "log.jsonl", all)
	return dir
}

// seedDir is the 21-day flavor used by the original tests. `now` returned
// alongside is the natural reference time for Build().
func seedDir(t *testing.T, fn func(day int) []contracts.UsageEntry) string {
	return seedDirN(t, 21, fn)
}

// nowFor returns a now-time on the last seeded day, so gap-fill doesn't add
// an empty trailing week beyond the data.
func nowFor(days int) time.Time {
	base := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	return base.AddDate(0, 0, days-1)
}

func TestBuildBucketsDailyAndWeekly(t *testing.T) {
	dir := seedDir(t, func(day int) []contracts.UsageEntry {
		return []contracts.UsageEntry{
			{Provider: "anthropic", Model: "sonnet", TokensIn: 100, TokensOut: 200, CostUSD: 0.5, Status: "ok"},
		}
	})
	now := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	tr, err := Build(dir, now)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(tr.Daily) != 21 {
		t.Errorf("daily: want 21, got %d", len(tr.Daily))
	}
	if tr.TotalCalls != 21 {
		t.Errorf("total calls: want 21, got %d", tr.TotalCalls)
	}
	if len(tr.Weekly) < 3 {
		t.Errorf("weekly: want >= 3 buckets, got %d", len(tr.Weekly))
	}
}

func TestCostProjectionLinearTrend(t *testing.T) {
	// Cost rises by $1/day. Slope should be ~1, projected 30d ~= sum of next 30 days.
	dir := seedDir(t, func(day int) []contracts.UsageEntry {
		return []contracts.UsageEntry{{Model: "m", CostUSD: float64(day) + 1, Status: "ok"}}
	})
	tr, err := Build(dir, time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	p := tr.CostProjection(14)
	if p.WindowDays != 14 {
		t.Errorf("window: want 14, got %d", p.WindowDays)
	}
	if p.SlopeUSDPerDay < 0.9 || p.SlopeUSDPerDay > 1.1 {
		t.Errorf("slope: want ~1.0, got %f", p.SlopeUSDPerDay)
	}
	if p.ProjectedNext30USD < 500 {
		t.Errorf("30d projection looks low: %f", p.ProjectedNext30USD)
	}
}

func TestCostProjectionEmpty(t *testing.T) {
	tr := &Trends{}
	p := tr.CostProjection(14)
	if p.WindowDays != 0 {
		t.Errorf("empty trend: want zero projection, got %+v", p)
	}
}

func TestCostTrajectoryInsightFires(t *testing.T) {
	dir := seedDir(t, func(day int) []contracts.UsageEntry {
		return []contracts.UsageEntry{{Model: "m", CostUSD: float64(day) + 1, Status: "ok"}}
	})
	tr, _ := Build(dir, time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC))
	got := tr.Insights()
	found := false
	for _, in := range got {
		if in.ID == "cost-trending-up" {
			found = true
			if in.Severity != InsightWarning {
				t.Errorf("severity: want warning, got %v", in.Severity)
			}
		}
	}
	if !found {
		t.Errorf("expected cost-trending-up insight; got %+v", got)
	}
}

func TestModelMigrationInsight(t *testing.T) {
	// First 14 days: all "old". Last 7 days: all "new".
	dir := seedDir(t, func(day int) []contracts.UsageEntry {
		model := "old"
		if day >= 14 {
			model = "new"
		}
		out := []contracts.UsageEntry{}
		for i := 0; i < 5; i++ {
			out = append(out, contracts.UsageEntry{Model: model, CostUSD: 0.1, Status: "ok"})
		}
		return out
	})
	tr, _ := Build(dir, time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC))
	got := tr.Insights()
	var up, down bool
	for _, in := range got {
		if in.ID == "model-migration-up:new" {
			up = true
		}
		if in.ID == "model-migration-down:old" {
			down = true
		}
	}
	if !up || !down {
		t.Errorf("expected both up:new and down:old insights; got %+v", got)
	}
}

func TestReliabilityInsightHighErrors(t *testing.T) {
	dir := seedDir(t, func(day int) []contracts.UsageEntry {
		out := []contracts.UsageEntry{}
		for i := 0; i < 10; i++ {
			status := "ok"
			if i < 1 { // 10% error rate
				status = "error"
			}
			out = append(out, contracts.UsageEntry{Model: "m", CostUSD: 0.01, Status: status})
		}
		return out
	})
	tr, _ := Build(dir, time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC))
	got := tr.Insights()
	found := false
	for _, in := range got {
		if in.ID == "error-rate-high" {
			found = true
			if in.Severity != InsightWarning {
				t.Errorf("severity: want warning, got %v", in.Severity)
			}
		}
	}
	if !found {
		t.Errorf("expected error-rate-high insight; got %+v", got)
	}
}

func TestFeatureAdoptionInsight(t *testing.T) {
	// Days 0-20 (3 weeks): no feature. Days 21-27 (4th week, Mon-Sun): voice.
	dir := seedDirN(t, 28, func(day int) []contracts.UsageEntry {
		feature := ""
		if day >= 21 {
			feature = "voice"
		}
		out := []contracts.UsageEntry{}
		for i := 0; i < 3; i++ {
			out = append(out, contracts.UsageEntry{Model: "m", Feature: feature, Status: "ok"})
		}
		return out
	})
	tr, _ := Build(dir, nowFor(28))
	got := tr.Insights()
	for _, in := range got {
		if in.ID == "feature-adopted:voice" {
			return
		}
	}
	t.Errorf("expected feature-adopted:voice; got %+v", got)
}

func TestEfficiencyInsightImprovement(t *testing.T) {
	// Three full weeks at 10 tok/s, then a clean Mon-Sun week at 20 tok/s.
	dir := seedDirN(t, 28, func(day int) []contracts.UsageEntry {
		rate := 10.0
		if day >= 21 {
			rate = 20.0 // doubled
		}
		return []contracts.UsageEntry{{Model: "m", TokensPerSecond: rate, Status: "ok"}}
	})
	tr, _ := Build(dir, nowFor(28))
	got := tr.Insights()
	for _, in := range got {
		if in.ID == "efficiency-up" {
			return
		}
	}
	t.Errorf("expected efficiency-up; got %+v", got)
}

func TestNoInsightsOnFlatData(t *testing.T) {
	dir := seedDir(t, func(day int) []contracts.UsageEntry {
		return []contracts.UsageEntry{{Model: "m", CostUSD: 0.5, TokensPerSecond: 10, Status: "ok"}}
	})
	tr, _ := Build(dir, time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC))
	got := tr.Insights()
	for _, in := range got {
		if in.Severity == InsightWarning {
			t.Errorf("flat data should not produce warnings; got %+v", in)
		}
	}
}

func TestBuildEmptyDir(t *testing.T) {
	dir := t.TempDir()
	tr, err := Build(dir, time.Now())
	if err != nil {
		t.Fatalf("Build empty: %v", err)
	}
	if len(tr.Daily) != 0 || len(tr.Weekly) != 0 {
		t.Errorf("empty trend should have no buckets")
	}
	if got := tr.Insights(); len(got) != 0 {
		t.Errorf("empty trend should have no insights, got %+v", got)
	}
}

func TestBuildMissingDirIsOK(t *testing.T) {
	tr, err := Build("/no/such/path/aaaaa", time.Now())
	if err != nil {
		t.Fatalf("missing dir should not error: %v", err)
	}
	if tr == nil {
		t.Fatal("nil trend")
	}
}

func TestLegacyTokenFieldsRespected(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, dir, "x.jsonl", []contracts.UsageEntry{
		{Timestamp: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), Model: "m", InputTokens: 50, OutputTokens: 75, Status: "ok"},
	})
	tr, _ := Build(dir, time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC))
	if len(tr.Daily) != 1 {
		t.Fatalf("want 1 bucket, got %d", len(tr.Daily))
	}
	if tr.Daily[0].TokensIn != 50 || tr.Daily[0].TokensOut != 75 {
		t.Errorf("legacy fields not respected: %+v", tr.Daily[0])
	}
}
