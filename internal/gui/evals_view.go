package gui

import (
	"sort"
	"sync"
	"time"

	"github.com/jabreeflor/conduit/internal/eval"
)

// EvalsView is the view-model for the GUI Evals tab (PRD §6.23 + §11.2).
//
// It accepts a stream of eval.CaseResult records (sourced from JSONL on disk
// via eval.Store, or pushed live as runs complete) and produces:
//
//   - per-model scorecards (pass rate, average cost, average latency)
//   - per-model trend points over time, suitable for sparkline rendering
//   - per-suite filtering
//
// The view-model is read-mostly; it precomputes summaries on every Set so
// the render path stays cheap. Result records are not retained verbatim —
// only the derived rollups, which keeps memory bounded under long history.
type EvalsView struct {
	mu sync.RWMutex

	// Filter state.
	suite string // "" means all suites
	model string // "" means all models

	// Derived rollups.
	scorecards []ModelScorecard
	trends     map[string][]TrendPoint // keyed by model
	allSuites  []string
	allModels  []string
}

// ModelScorecard is the per-model summary row rendered in the scorecard table.
//
// It mirrors eval.Summary but is GUI-shaped: columns the table displays,
// already typed for direct binding (no need to re-derive on each render).
type ModelScorecard struct {
	Model          string
	Cases          int
	Passed         int
	PassRate       float64 // 0..1
	AvgCostUSD     float64
	AvgLatencySecs float64
	LastObserved   time.Time
	HarnessMetrics eval.MetricSummary // for the metric breakdown popover
	NeedsAttention bool               // pass rate < 0.7 OR cost rising trend
	TotalCostUSD   float64
}

// TrendPoint is one (timestamp, score%) sample for a model's trend line.
//
// Score% is the rolling pass percentage over a configurable window; the
// initial implementation uses a per-day bucket so a week of runs renders
// as 7 points. The bucket is exposed so the GUI can offer hour/day/week
// granularity without re-binning every render.
type TrendPoint struct {
	At     time.Time
	Score  float64 // 0..100, percent passed in the bucket
	Bucket time.Duration
}

// NewEvalsView returns an empty view; populate via SetResults.
func NewEvalsView() *EvalsView {
	return &EvalsView{
		trends: map[string][]TrendPoint{},
	}
}

// SetResults replaces the rollups using the given results. Suite and model
// filters are preserved across SetResults; recomputation is automatic.
func (v *EvalsView) SetResults(results []eval.CaseResult) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.recomputeLocked(results)
}

// SetSuite filters by suite name. Pass "" for "all suites".
func (v *EvalsView) SetSuite(suite string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.suite = suite
}

// SetModel filters by model. Pass "" for "all models".
func (v *EvalsView) SetModel(model string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.model = model
}

// Scorecards returns a snapshot of the per-model scorecards, sorted by
// model name for stable rendering. Filtered by suite and model.
func (v *EvalsView) Scorecards() []ModelScorecard {
	v.mu.RLock()
	defer v.mu.RUnlock()
	out := make([]ModelScorecard, 0, len(v.scorecards))
	for _, s := range v.scorecards {
		if v.model != "" && s.Model != v.model {
			continue
		}
		out = append(out, s)
	}
	return out
}

// Trend returns the trend points for one model, sorted by timestamp.
func (v *EvalsView) Trend(model string) []TrendPoint {
	v.mu.RLock()
	defer v.mu.RUnlock()
	pts := v.trends[model]
	out := make([]TrendPoint, len(pts))
	copy(out, pts)
	return out
}

// AllSuites returns every suite name observed in the most recent SetResults.
// Useful for populating the suite filter dropdown.
func (v *EvalsView) AllSuites() []string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	out := make([]string, len(v.allSuites))
	copy(out, v.allSuites)
	return out
}

// AllModels returns every model name observed in the most recent SetResults.
func (v *EvalsView) AllModels() []string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	out := make([]string, len(v.allModels))
	copy(out, v.allModels)
	return out
}

// recomputeLocked rebuilds scorecards and trends from the input slice.
// Must be called with v.mu held.
func (v *EvalsView) recomputeLocked(results []eval.CaseResult) {
	type bucket struct {
		passed, total int
		costSum       float64
		latencySum    float64
		last          time.Time
		metrics       eval.MetricSummary
	}
	per := map[string]*bucket{}
	suites := map[string]struct{}{}
	models := map[string]struct{}{}

	for _, r := range results {
		if v.suite != "" && r.Suite != v.suite {
			continue
		}
		suites[r.Suite] = struct{}{}
		models[r.Model] = struct{}{}

		b := per[r.Model]
		if b == nil {
			b = &bucket{}
			per[r.Model] = b
		}
		b.total++
		if r.Passed {
			b.passed++
		}
		b.costSum += r.Observed.CostUSD
		b.latencySum += r.Observed.DurationSeconds
		if r.At.After(b.last) {
			b.last = r.At
		}
	}

	// Scorecards.
	cards := make([]ModelScorecard, 0, len(per))
	for model, b := range per {
		passRate := 0.0
		avgCost := 0.0
		avgLatency := 0.0
		if b.total > 0 {
			passRate = float64(b.passed) / float64(b.total)
			avgCost = b.costSum / float64(b.total)
			avgLatency = b.latencySum / float64(b.total)
		}
		cards = append(cards, ModelScorecard{
			Model:          model,
			Cases:          b.total,
			Passed:         b.passed,
			PassRate:       passRate,
			AvgCostUSD:     avgCost,
			AvgLatencySecs: avgLatency,
			LastObserved:   b.last,
			HarnessMetrics: b.metrics,
			TotalCostUSD:   b.costSum,
			NeedsAttention: passRate < 0.7,
		})
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].Model < cards[j].Model })
	v.scorecards = cards

	// Trends: per-model per-day buckets.
	const bucketDur = 24 * time.Hour
	type tk struct {
		model string
		day   time.Time
	}
	trendBuckets := map[tk]*bucket{}
	for _, r := range results {
		if v.suite != "" && r.Suite != v.suite {
			continue
		}
		key := tk{model: r.Model, day: r.At.Truncate(bucketDur)}
		b := trendBuckets[key]
		if b == nil {
			b = &bucket{}
			trendBuckets[key] = b
		}
		b.total++
		if r.Passed {
			b.passed++
		}
	}
	v.trends = map[string][]TrendPoint{}
	for k, b := range trendBuckets {
		score := 0.0
		if b.total > 0 {
			score = 100.0 * float64(b.passed) / float64(b.total)
		}
		v.trends[k.model] = append(v.trends[k.model], TrendPoint{
			At: k.day, Score: score, Bucket: bucketDur,
		})
	}
	for m := range v.trends {
		pts := v.trends[m]
		sort.Slice(pts, func(i, j int) bool { return pts[i].At.Before(pts[j].At) })
		v.trends[m] = pts
	}

	v.allSuites = setKeys(suites)
	v.allModels = setKeys(models)
}

func setKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
