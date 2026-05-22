package gui

import (
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/eval"
)

func mkResult(suite, model string, passed bool, costUSD, durSecs float64, at time.Time) eval.CaseResult {
	return eval.CaseResult{
		At:     at,
		Suite:  suite,
		Model:  model,
		Passed: passed,
		Observed: eval.ObservedTrace{
			CostUSD:         costUSD,
			DurationSeconds: durSecs,
		},
	}
}

func TestEvalsView_emptyByDefault(t *testing.T) {
	v := NewEvalsView()
	if got := v.Scorecards(); len(got) != 0 {
		t.Fatalf("expected no scorecards, got %d", len(got))
	}
	if got := v.AllModels(); len(got) != 0 {
		t.Fatalf("expected no models, got %v", got)
	}
}

func TestEvalsView_scorecardsAggregateByModel(t *testing.T) {
	day := time.Date(2026, 5, 6, 9, 0, 0, 0, time.UTC)
	v := NewEvalsView()
	v.SetResults([]eval.CaseResult{
		mkResult("smoke", "claude-opus-4-7", true, 0.10, 2.0, day),
		mkResult("smoke", "claude-opus-4-7", true, 0.20, 3.0, day),
		mkResult("smoke", "claude-opus-4-7", false, 0.05, 1.0, day),
		mkResult("smoke", "claude-sonnet-4-6", true, 0.04, 1.5, day),
	})

	cards := v.Scorecards()
	if len(cards) != 2 {
		t.Fatalf("expected 2 scorecards, got %d", len(cards))
	}

	// Sorted by model name → opus first.
	if cards[0].Model != "claude-opus-4-7" {
		t.Fatalf("first card should be opus, got %q", cards[0].Model)
	}
	opus := cards[0]
	if opus.Cases != 3 {
		t.Fatalf("opus cases: want 3, got %d", opus.Cases)
	}
	if opus.Passed != 2 {
		t.Fatalf("opus passed: want 2, got %d", opus.Passed)
	}
	wantPass := 2.0 / 3.0
	if opus.PassRate < wantPass-1e-9 || opus.PassRate > wantPass+1e-9 {
		t.Fatalf("opus pass rate: want %.4f, got %.4f", wantPass, opus.PassRate)
	}
	wantCost := (0.10 + 0.20 + 0.05) / 3
	if opus.AvgCostUSD < wantCost-1e-9 || opus.AvgCostUSD > wantCost+1e-9 {
		t.Fatalf("opus avg cost: want %.4f, got %.4f", wantCost, opus.AvgCostUSD)
	}
	if opus.TotalCostUSD < 0.349 || opus.TotalCostUSD > 0.351 {
		t.Fatalf("opus total cost out of range: %.4f", opus.TotalCostUSD)
	}
}

func TestEvalsView_needsAttentionThreshold(t *testing.T) {
	day := time.Now()
	v := NewEvalsView()
	v.SetResults([]eval.CaseResult{
		// 1/3 passed → 33% → below 70% threshold → NeedsAttention.
		mkResult("smoke", "model-a", true, 0, 1, day),
		mkResult("smoke", "model-a", false, 0, 1, day),
		mkResult("smoke", "model-a", false, 0, 1, day),
		// 4/4 passed → above threshold.
		mkResult("smoke", "model-b", true, 0, 1, day),
		mkResult("smoke", "model-b", true, 0, 1, day),
		mkResult("smoke", "model-b", true, 0, 1, day),
		mkResult("smoke", "model-b", true, 0, 1, day),
	})
	cards := v.Scorecards()
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(cards))
	}
	if !cards[0].NeedsAttention {
		t.Fatalf("model-a should need attention at %.2f pass rate", cards[0].PassRate)
	}
	if cards[1].NeedsAttention {
		t.Fatalf("model-b should not need attention at %.2f pass rate", cards[1].PassRate)
	}
}

func TestEvalsView_suiteFilter(t *testing.T) {
	day := time.Now()
	v := NewEvalsView()
	v.SetResults([]eval.CaseResult{
		mkResult("smoke", "m", true, 0, 1, day),
		mkResult("regression", "m", false, 0, 1, day),
	})
	if got := len(v.Scorecards()); got != 1 {
		t.Fatalf("unfiltered: expected 1 model card, got %d", got)
	}
	// Re-bind to filtered suite via SetSuite + SetResults to recompute.
	v.SetSuite("smoke")
	v.SetResults([]eval.CaseResult{
		mkResult("smoke", "m", true, 0, 1, day),
		mkResult("regression", "m", false, 0, 1, day),
	})
	cards := v.Scorecards()
	if len(cards) != 1 {
		t.Fatalf("expected 1 card after filter, got %d", len(cards))
	}
	if cards[0].Cases != 1 || cards[0].Passed != 1 {
		t.Fatalf("smoke filter should leave 1/1 passing; got %+v", cards[0])
	}
}

func TestEvalsView_trendBucketsByDay(t *testing.T) {
	d1 := time.Date(2026, 5, 4, 9, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 5, 5, 9, 0, 0, 0, time.UTC)
	d3 := time.Date(2026, 5, 5, 18, 0, 0, 0, time.UTC) // same day as d2

	v := NewEvalsView()
	v.SetResults([]eval.CaseResult{
		mkResult("smoke", "m", true, 0, 1, d1),
		mkResult("smoke", "m", false, 0, 1, d2),
		mkResult("smoke", "m", true, 0, 1, d3),
	})
	pts := v.Trend("m")
	if len(pts) != 2 {
		t.Fatalf("expected 2 day buckets, got %d (%+v)", len(pts), pts)
	}
	if !pts[0].At.Before(pts[1].At) {
		t.Fatalf("trend points should be sorted ascending: %+v", pts)
	}
	if pts[0].Score != 100 {
		t.Fatalf("day1 should be 100%%, got %.1f", pts[0].Score)
	}
	if pts[1].Score != 50 {
		t.Fatalf("day2 should be 50%% (1/2), got %.1f", pts[1].Score)
	}
}

func TestEvalsView_allSuitesAndModelsSorted(t *testing.T) {
	day := time.Now()
	v := NewEvalsView()
	v.SetResults([]eval.CaseResult{
		mkResult("zeta", "model-z", true, 0, 1, day),
		mkResult("alpha", "model-a", false, 0, 1, day),
		mkResult("alpha", "model-z", true, 0, 1, day), // duplicate model
	})
	suites := v.AllSuites()
	models := v.AllModels()
	if len(suites) != 2 || suites[0] != "alpha" || suites[1] != "zeta" {
		t.Fatalf("AllSuites should be sorted: got %v", suites)
	}
	if len(models) != 2 || models[0] != "model-a" || models[1] != "model-z" {
		t.Fatalf("AllModels should be sorted unique: got %v", models)
	}
}

func TestEvalsView_modelFilterAffectsScorecards(t *testing.T) {
	day := time.Now()
	v := NewEvalsView()
	v.SetResults([]eval.CaseResult{
		mkResult("smoke", "model-a", true, 0, 1, day),
		mkResult("smoke", "model-b", true, 0, 1, day),
	})
	v.SetModel("model-a")
	cards := v.Scorecards()
	if len(cards) != 1 || cards[0].Model != "model-a" {
		t.Fatalf("model filter should leave only model-a, got %v", cards)
	}
	v.SetModel("")
	if got := len(v.Scorecards()); got != 2 {
		t.Fatalf("clearing filter should restore both cards, got %d", got)
	}
}
