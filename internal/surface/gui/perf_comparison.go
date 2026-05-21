// Package gui — performance comparison view-model (issue #118).
//
// Side-by-side model metrics for the GUI performance panel (PRD §14.5):
// TTFT, total latency, tokens/sec, error rate, cost per 1K, fallback
// frequency, plus quality proxies (edit / retry / completion rate). Also
// produces points for a latency-vs-cost scatter plot.
//
// Quality signals are explicitly labeled as proxies — they should be drawn
// with a "proxy" badge so users don't read them as ground truth.
package gui

import (
	"sort"
	"sync"
	"time"
)

// PerfSample is one observation about a single model invocation. The tracker
// emits these alongside UsageSamples; the comparison view aggregates them.
type PerfSample struct {
	Timestamp time.Time
	Provider  string
	Model     string

	// Latency metrics.
	TTFTMS  int64 // time-to-first-token in milliseconds
	TotalMS int64 // request wall-clock duration

	// Token throughput.
	OutputTokens int

	// Error / routing flags.
	Error    bool
	Fallback bool // true when the router fell back to this model from another

	// Cost.
	CostUSD float64

	// Quality proxies. Each is a 0..1 ratio derived from downstream signals;
	// callers may leave them at zero when not applicable. They are
	// intentionally noisy and must be rendered with a "proxy" disclaimer.
	EditRate       float64 // user edited the model output before keeping it
	RetryRate      float64 // user invoked retry / "regenerate" on this turn
	CompletionRate float64 // request hit a normal stop (not cap, not error)
}

// ModelMetrics is the row the GUI table draws for one model.
type ModelMetrics struct {
	Provider string
	Model    string

	Samples int

	// Latency.
	TTFTMSMedian  float64
	TotalMSMedian float64

	// Throughput.
	TokensPerSec float64

	// Reliability.
	ErrorRate    float64 // 0..1
	FallbackRate float64 // 0..1

	// Cost.
	CostPer1KTokens float64

	// Quality proxies (averages of the per-sample 0..1 values).
	EditRate       float64
	RetryRate      float64
	CompletionRate float64
}

// ScatterPoint is one dot in the latency-vs-cost plot.
type ScatterPoint struct {
	Provider  string
	Model     string
	LatencyMS float64 // median TotalMS
	CostUSD   float64 // average per-call cost
}

// PerfComparison is the view-model. It holds raw samples and derives the
// row table and scatter plot on demand. Callers can scope the view to a
// subset of models via Include / Exclude.
//
// Safe for concurrent use.
type PerfComparison struct {
	mu       sync.RWMutex
	samples  []PerfSample
	included map[string]bool // "provider/model" allowlist; empty = all
	excluded map[string]bool
}

// NewPerfComparison returns an empty view.
func NewPerfComparison() *PerfComparison {
	return &PerfComparison{
		included: make(map[string]bool),
		excluded: make(map[string]bool),
	}
}

// Ingest appends a sample.
func (p *PerfComparison) Ingest(s PerfSample) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.samples = append(p.samples, s)
}

// Include limits the comparison to the listed models. An empty allowlist
// means "all models".
func (p *PerfComparison) Include(provider, model string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.included[modelKey(provider, model)] = true
}

// Exclude hides a model from the comparison.
func (p *PerfComparison) Exclude(provider, model string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.excluded[modelKey(provider, model)] = true
}

// ClearFilters removes all include/exclude entries.
func (p *PerfComparison) ClearFilters() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.included = make(map[string]bool)
	p.excluded = make(map[string]bool)
}

// Rows returns the per-model metric rows sorted by total-latency ascending
// (faster first).
func (p *PerfComparison) Rows() []ModelMetrics {
	p.mu.RLock()
	defer p.mu.RUnlock()

	groups := p.groupedLocked()
	out := make([]ModelMetrics, 0, len(groups))
	for _, g := range groups {
		out = append(out, computeRow(g))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].TotalMSMedian < out[j].TotalMSMedian
	})
	return out
}

// Scatter returns one point per model for the latency-vs-cost plot.
func (p *PerfComparison) Scatter() []ScatterPoint {
	rows := p.Rows()
	out := make([]ScatterPoint, 0, len(rows))
	for _, r := range rows {
		var perCall float64
		if r.Samples > 0 {
			// Cost per call ≈ cost per 1K * (output tokens / 1000) — but we
			// don't have output tokens at the row level; fall back to using
			// CostPer1KTokens as the y-axis proxy. Renderers can override.
			perCall = r.CostPer1KTokens
		}
		out = append(out, ScatterPoint{
			Provider:  r.Provider,
			Model:     r.Model,
			LatencyMS: r.TotalMSMedian,
			CostUSD:   perCall,
		})
	}
	return out
}

// groupedLocked partitions samples by (provider, model) honouring the
// include / exclude filters. Must be called with p.mu held.
func (p *PerfComparison) groupedLocked() map[string][]PerfSample {
	out := map[string][]PerfSample{}
	for _, s := range p.samples {
		k := modelKey(s.Provider, s.Model)
		if len(p.included) > 0 && !p.included[k] {
			continue
		}
		if p.excluded[k] {
			continue
		}
		out[k] = append(out[k], s)
	}
	return out
}

func computeRow(samples []PerfSample) ModelMetrics {
	if len(samples) == 0 {
		return ModelMetrics{}
	}
	row := ModelMetrics{
		Provider: samples[0].Provider,
		Model:    samples[0].Model,
		Samples:  len(samples),
	}

	ttfts := make([]float64, 0, len(samples))
	totals := make([]float64, 0, len(samples))
	var (
		tokens   int
		costSum  float64
		errCnt   int
		fbCnt    int
		editSum  float64
		retrySum float64
		compSum  float64
		totalMS  float64
	)
	for _, s := range samples {
		ttfts = append(ttfts, float64(s.TTFTMS))
		totals = append(totals, float64(s.TotalMS))
		tokens += s.OutputTokens
		costSum += s.CostUSD
		totalMS += float64(s.TotalMS)
		if s.Error {
			errCnt++
		}
		if s.Fallback {
			fbCnt++
		}
		editSum += s.EditRate
		retrySum += s.RetryRate
		compSum += s.CompletionRate
	}
	row.TTFTMSMedian = median(ttfts)
	row.TotalMSMedian = median(totals)
	if totalMS > 0 {
		row.TokensPerSec = float64(tokens) / (totalMS / 1000.0)
	}
	n := float64(len(samples))
	row.ErrorRate = float64(errCnt) / n
	row.FallbackRate = float64(fbCnt) / n
	if tokens > 0 {
		row.CostPer1KTokens = costSum / (float64(tokens) / 1000.0)
	}
	row.EditRate = editSum / n
	row.RetryRate = retrySum / n
	row.CompletionRate = compSum / n
	return row
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	cp := make([]float64, len(xs))
	copy(cp, xs)
	sort.Float64s(cp)
	mid := len(cp) / 2
	if len(cp)%2 == 1 {
		return cp[mid]
	}
	return (cp[mid-1] + cp[mid]) / 2
}

func modelKey(provider, model string) string {
	return provider + "/" + model
}

// QualityProxyDisclaimer is the text the renderer should attach to any
// column derived from EditRate / RetryRate / CompletionRate. Keeping it as a
// constant guarantees TUI and GUI use identical wording.
const QualityProxyDisclaimer = "Quality signals are proxies derived from user behaviour, not ground-truth ratings."
