// Package gui — usage dashboard view-model (issue #116).
//
// Coordinates the panels described in PRD §14.3 (cost overview, cost by
// model/feature, request volume, latency percentiles, error rate, token
// usage, model comparison, plugin usage, budget status) plus a time-range
// selector, multi-dimensional filters, and drill-down to the request log.
//
// The view-model holds raw UsageSamples in memory; aggregation is done on
// demand so filter changes are immediate. Backed by the same data the
// `internal/usage` exporter writes to disk.
package gui

import (
	"sort"
	"sync"
	"time"
)

// TimeRange is one of the preset selectors offered above the dashboard. The
// renderer translates this into concrete From/To timestamps.
type TimeRange int

const (
	RangeLast24h TimeRange = iota
	RangeLast7d
	RangeLast30d
	RangeMonthToDate
	RangeAllTime
	RangeCustom
)

// PanelKind identifies a dashboard panel. The renderer maps these to widgets.
type PanelKind int

const (
	PanelCostOverview PanelKind = iota
	PanelCostByModel
	PanelCostByFeature
	PanelRequestVolume
	PanelLatencyPercentiles
	PanelErrorRate
	PanelTokenUsage
	PanelModelComparison
	PanelPluginUsage
	PanelBudgetStatus
)

// UsageSample is the per-request datum the dashboard aggregates. It mirrors
// the public fields of internal/contracts.UsageEntry plus the fields the
// dashboard needs that aren't in the export schema (latency, error, plugin).
type UsageSample struct {
	Timestamp time.Time
	Provider  string
	Model     string
	Feature   string // chat / coding / spotlight / workflow / …
	Plugin    string // plugin attribution; "" when none
	Calls     int
	Tokens    int
	CostUSD   float64
	LatencyMS int64
	Error     bool
}

// UsageFilter selects a subset of samples. Empty fields match anything.
type UsageFilter struct {
	From, To time.Time // zero means unbounded
	Provider string
	Model    string
	Feature  string
	Plugin   string
}

// PanelData holds the aggregate the renderer should draw for one panel.
// Different panels populate different fields; consumers should switch on
// the PanelKind to know which fields are valid.
type PanelData struct {
	Kind PanelKind

	// Scalar metrics (PanelCostOverview, PanelErrorRate, PanelBudgetStatus).
	Total float64

	// Categorical breakdown (PanelCostByModel, PanelPluginUsage, …).
	Series []SeriesPoint

	// Time-bucketed metric (PanelRequestVolume, PanelTokenUsage).
	TimeSeries []TimePoint

	// Latency percentiles for PanelLatencyPercentiles.
	P50, P95, P99 float64

	// Per-model rows for PanelModelComparison.
	Rows []ModelRow
}

// SeriesPoint is one bar in a categorical chart.
type SeriesPoint struct {
	Label string
	Value float64
}

// TimePoint is one bar in a time-series chart.
type TimePoint struct {
	At    time.Time
	Value float64
}

// ModelRow is one row in the model comparison table.
type ModelRow struct {
	Provider string
	Model    string
	Calls    int
	Tokens   int
	CostUSD  float64
	AvgMS    float64
	ErrorPct float64
}

// UsageDashboard is the view-model for the GUI usage dashboard surface.
// Safe for concurrent use: ingestion runs in the tracker goroutine while the
// UI thread re-queries panels on filter changes.
type UsageDashboard struct {
	mu      sync.RWMutex
	samples []UsageSample
	filter  UsageFilter
	rng     TimeRange
	budget  float64 // monthly budget USD; 0 means no budget set
}

// NewUsageDashboard creates an empty dashboard with no filter and no budget.
func NewUsageDashboard() *UsageDashboard {
	return &UsageDashboard{rng: RangeLast7d}
}

// Ingest appends a sample. The tracker calls this per request.
func (d *UsageDashboard) Ingest(s UsageSample) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.samples = append(d.samples, s)
}

// IngestBatch appends many samples in one critical section.
func (d *UsageDashboard) IngestBatch(samples []UsageSample) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.samples = append(d.samples, samples...)
}

// SetFilter replaces the active filter. The next Panel() call reflects it.
func (d *UsageDashboard) SetFilter(f UsageFilter) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.filter = f
}

// Filter returns the active filter.
func (d *UsageDashboard) Filter() UsageFilter {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.filter
}

// SetRange sets the preset time range. RangeCustom is a hint that the caller
// will set From/To via SetFilter directly.
func (d *UsageDashboard) SetRange(r TimeRange) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.rng = r
	if r != RangeCustom {
		now := time.Now()
		d.filter.From, d.filter.To = rangeBounds(r, now)
	}
}

// Range returns the current range preset.
func (d *UsageDashboard) Range() TimeRange {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.rng
}

// SetBudget configures the monthly budget USD; 0 disables the budget panel.
func (d *UsageDashboard) SetBudget(usd float64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.budget = usd
}

// Panel computes the aggregate for one panel using the current filter.
func (d *UsageDashboard) Panel(p PanelKind) PanelData {
	d.mu.RLock()
	defer d.mu.RUnlock()

	matched := d.matchedLocked()
	out := PanelData{Kind: p}

	switch p {
	case PanelCostOverview:
		for _, s := range matched {
			out.Total += s.CostUSD
		}
	case PanelCostByModel:
		out.Series = sumBy(matched, func(s UsageSample) string { return s.Provider + "/" + s.Model }, func(s UsageSample) float64 { return s.CostUSD })
	case PanelCostByFeature:
		out.Series = sumBy(matched, func(s UsageSample) string { return s.Feature }, func(s UsageSample) float64 { return s.CostUSD })
	case PanelRequestVolume:
		out.TimeSeries = bucketByDay(matched, func(s UsageSample) float64 { return float64(s.Calls) })
	case PanelTokenUsage:
		out.TimeSeries = bucketByDay(matched, func(s UsageSample) float64 { return float64(s.Tokens) })
	case PanelLatencyPercentiles:
		out.P50, out.P95, out.P99 = percentiles(matched)
	case PanelErrorRate:
		var errs, total int
		for _, s := range matched {
			total += s.Calls
			if s.Error {
				errs += s.Calls
			}
		}
		if total > 0 {
			out.Total = float64(errs) / float64(total)
		}
	case PanelModelComparison:
		out.Rows = modelRows(matched)
	case PanelPluginUsage:
		out.Series = sumBy(matched, func(s UsageSample) string {
			if s.Plugin == "" {
				return "(none)"
			}
			return s.Plugin
		}, func(s UsageSample) float64 { return s.CostUSD })
	case PanelBudgetStatus:
		var spent float64
		for _, s := range matched {
			spent += s.CostUSD
		}
		out.Total = spent
		// Encode budget as a P50 reuse so the renderer can compute fraction.
		out.P50 = d.budget
	}
	return out
}

// DrillDown returns the raw samples that match the current filter — this is
// the row-level request log the dashboard drills down into.
func (d *UsageDashboard) DrillDown() []UsageSample {
	d.mu.RLock()
	defer d.mu.RUnlock()
	matched := d.matchedLocked()
	out := make([]UsageSample, len(matched))
	copy(out, matched)
	return out
}

// matchedLocked applies the active filter. Must be called with d.mu held.
func (d *UsageDashboard) matchedLocked() []UsageSample {
	out := make([]UsageSample, 0, len(d.samples))
	for _, s := range d.samples {
		if !d.filter.From.IsZero() && s.Timestamp.Before(d.filter.From) {
			continue
		}
		if !d.filter.To.IsZero() && s.Timestamp.After(d.filter.To) {
			continue
		}
		if d.filter.Provider != "" && s.Provider != d.filter.Provider {
			continue
		}
		if d.filter.Model != "" && s.Model != d.filter.Model {
			continue
		}
		if d.filter.Feature != "" && s.Feature != d.filter.Feature {
			continue
		}
		if d.filter.Plugin != "" && s.Plugin != d.filter.Plugin {
			continue
		}
		out = append(out, s)
	}
	return out
}

// rangeBounds returns the From/To pair implied by a preset range relative
// to now. RangeAllTime returns the zero values.
func rangeBounds(r TimeRange, now time.Time) (time.Time, time.Time) {
	switch r {
	case RangeLast24h:
		return now.Add(-24 * time.Hour), now
	case RangeLast7d:
		return now.Add(-7 * 24 * time.Hour), now
	case RangeLast30d:
		return now.Add(-30 * 24 * time.Hour), now
	case RangeMonthToDate:
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()), now
	case RangeAllTime, RangeCustom:
		return time.Time{}, time.Time{}
	}
	return time.Time{}, time.Time{}
}

// sumBy returns labeled sums sorted by value descending.
func sumBy(samples []UsageSample, key func(UsageSample) string, val func(UsageSample) float64) []SeriesPoint {
	totals := map[string]float64{}
	for _, s := range samples {
		totals[key(s)] += val(s)
	}
	out := make([]SeriesPoint, 0, len(totals))
	for k, v := range totals {
		out = append(out, SeriesPoint{Label: k, Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value > out[j].Value })
	return out
}

// bucketByDay collapses samples into UTC day buckets.
func bucketByDay(samples []UsageSample, val func(UsageSample) float64) []TimePoint {
	totals := map[time.Time]float64{}
	for _, s := range samples {
		day := time.Date(s.Timestamp.Year(), s.Timestamp.Month(), s.Timestamp.Day(), 0, 0, 0, 0, time.UTC)
		totals[day] += val(s)
	}
	out := make([]TimePoint, 0, len(totals))
	for k, v := range totals {
		out = append(out, TimePoint{At: k, Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

// percentiles returns the 50th / 95th / 99th percentile of latency in ms.
func percentiles(samples []UsageSample) (p50, p95, p99 float64) {
	if len(samples) == 0 {
		return
	}
	xs := make([]int64, 0, len(samples))
	for _, s := range samples {
		xs = append(xs, s.LatencyMS)
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })
	pick := func(p float64) float64 {
		idx := int(float64(len(xs)-1) * p)
		return float64(xs[idx])
	}
	return pick(0.50), pick(0.95), pick(0.99)
}

// modelRows builds the per-(provider,model) table for the model comparison
// panel.
func modelRows(samples []UsageSample) []ModelRow {
	type acc struct {
		calls, tokens int
		cost, latSum  float64
		errs          int
	}
	totals := map[string]*acc{}
	keys := map[string][2]string{}
	for _, s := range samples {
		k := s.Provider + "|" + s.Model
		a, ok := totals[k]
		if !ok {
			a = &acc{}
			totals[k] = a
			keys[k] = [2]string{s.Provider, s.Model}
		}
		a.calls += s.Calls
		a.tokens += s.Tokens
		a.cost += s.CostUSD
		a.latSum += float64(s.LatencyMS) * float64(s.Calls)
		if s.Error {
			a.errs += s.Calls
		}
	}
	out := make([]ModelRow, 0, len(totals))
	for k, a := range totals {
		row := ModelRow{
			Provider: keys[k][0],
			Model:    keys[k][1],
			Calls:    a.calls,
			Tokens:   a.tokens,
			CostUSD:  a.cost,
		}
		if a.calls > 0 {
			row.AvgMS = a.latSum / float64(a.calls)
			row.ErrorPct = float64(a.errs) / float64(a.calls)
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CostUSD > out[j].CostUSD })
	return out
}
