// Package trends turns the rolling JSONL usage log into long-term views
// (PRD §14.9): model migration, cost trajectory with linear projection,
// feature adoption, efficiency gains, reliability trend.
//
// Insight generation is intentionally heuristic — threshold-based rules over
// the bucketed series. There are no model calls, no network I/O, and no
// fields are sent off the machine. This package only reads the existing
// usage logs (internal/usage.ScanAll) and computes summaries in memory.
package trends

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
	"github.com/jabreeflor/conduit/internal/usage"
)

// Bucket is one time-bucketed rollup of usage activity.
type Bucket struct {
	Start     time.Time
	Calls     int
	Errors    int
	TokensIn  int
	TokensOut int
	CostUSD   float64
	// PerModel and PerFeature track call counts within the bucket.
	PerModel   map[string]int
	PerFeature map[string]int
	// AvgTokensPerSec is the call-weighted mean tokens/sec for non-empty
	// observations. Zero if no calls in the bucket reported a rate.
	AvgTokensPerSec float64
}

// Trends is the materialized historical view.
type Trends struct {
	// Daily buckets, oldest first, gap-filled with zero buckets so charting
	// doesn't have to interpolate.
	Daily []Bucket
	// Weekly buckets (Mon-Sun), oldest first. Built from Daily.
	Weekly []Bucket
	// First / Last are the timestamp bounds covered by the data.
	First time.Time
	Last  time.Time
	// TotalCalls and TotalCost are convenience rollups across all buckets.
	TotalCalls int
	TotalCost  float64
}

// Build reads every entry under logDir and rolls them into Trends.
//
// `now` is the reference "today" used to gap-fill from Last → now-1d so the
// last bucket the UI sees is current. Callers in production pass time.Now();
// tests pass a fixed clock.
func Build(logDir string, now time.Time) (*Trends, error) {
	daily := map[time.Time]*Bucket{}
	rateSum := map[time.Time]float64{}
	rateCount := map[time.Time]int{}

	var first, last time.Time

	err := usage.ScanAll(logDir, func(e contracts.UsageEntry) bool {
		ts := entryTime(e)
		if ts.IsZero() {
			return true
		}
		if first.IsZero() || ts.Before(first) {
			first = ts
		}
		if last.IsZero() || ts.After(last) {
			last = ts
		}
		key := truncDay(ts)
		b := daily[key]
		if b == nil {
			b = &Bucket{Start: key, PerModel: map[string]int{}, PerFeature: map[string]int{}}
			daily[key] = b
		}
		b.Calls++
		if isError(e) {
			b.Errors++
		}
		b.TokensIn += effectiveTokensIn(e)
		b.TokensOut += effectiveTokensOut(e)
		b.CostUSD += e.CostUSD
		if e.Model != "" {
			b.PerModel[e.Model]++
		}
		if e.Feature != "" {
			b.PerFeature[e.Feature]++
		}
		if e.TokensPerSecond > 0 {
			rateSum[key] += e.TokensPerSecond
			rateCount[key]++
		}
		return true
	})
	if err != nil {
		return nil, fmt.Errorf("trends: scan logs: %w", err)
	}

	t := &Trends{First: first, Last: last}
	if len(daily) == 0 {
		return t, nil
	}

	// Compute the avg-tokens-per-sec for each populated bucket.
	for k, b := range daily {
		if rateCount[k] > 0 {
			b.AvgTokensPerSec = rateSum[k] / float64(rateCount[k])
		}
	}

	// Gap-fill from the first observed day through `now` (or `last`,
	// whichever is later). Charts look much better without holes.
	end := truncDay(now)
	if end.Before(truncDay(last)) {
		end = truncDay(last)
	}
	for d := truncDay(first); !d.After(end); d = d.AddDate(0, 0, 1) {
		if _, ok := daily[d]; !ok {
			daily[d] = &Bucket{Start: d, PerModel: map[string]int{}, PerFeature: map[string]int{}}
		}
	}

	keys := make([]time.Time, 0, len(daily))
	for k := range daily {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Before(keys[j]) })

	for _, k := range keys {
		b := daily[k]
		t.Daily = append(t.Daily, *b)
		t.TotalCalls += b.Calls
		t.TotalCost += b.CostUSD
	}

	t.Weekly = bucketize(t.Daily, weekStart)
	return t, nil
}

func bucketize(daily []Bucket, anchor func(time.Time) time.Time) []Bucket {
	if len(daily) == 0 {
		return nil
	}
	groups := map[time.Time]*Bucket{}
	rateAccum := map[time.Time]float64{}
	rateN := map[time.Time]int{}
	for _, d := range daily {
		k := anchor(d.Start)
		g := groups[k]
		if g == nil {
			g = &Bucket{Start: k, PerModel: map[string]int{}, PerFeature: map[string]int{}}
			groups[k] = g
		}
		g.Calls += d.Calls
		g.Errors += d.Errors
		g.TokensIn += d.TokensIn
		g.TokensOut += d.TokensOut
		g.CostUSD += d.CostUSD
		for m, n := range d.PerModel {
			g.PerModel[m] += n
		}
		for f, n := range d.PerFeature {
			g.PerFeature[f] += n
		}
		if d.AvgTokensPerSec > 0 {
			rateAccum[k] += d.AvgTokensPerSec
			rateN[k]++
		}
	}
	keys := make([]time.Time, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Before(keys[j]) })
	out := make([]Bucket, 0, len(keys))
	for _, k := range keys {
		g := groups[k]
		if rateN[k] > 0 {
			g.AvgTokensPerSec = rateAccum[k] / float64(rateN[k])
		}
		out = append(out, *g)
	}
	return out
}

// Projection is a simple linear extrapolation over the recent window.
type Projection struct {
	// SlopeUSDPerDay is the daily-cost regression slope.
	SlopeUSDPerDay float64
	// InterceptUSD is the regression intercept (cost on day 0 of the window).
	InterceptUSD float64
	// ProjectedNext30USD is the linear projection of total spend over the
	// next 30 days, starting one day after Last.
	ProjectedNext30USD float64
	// WindowDays is the number of days the regression was fit on.
	WindowDays int
}

// CostProjection runs an OLS linear regression over the trailing windowDays
// daily cost values and projects 30 days forward. Returns a zero Projection
// if there's insufficient data (need at least 7 buckets).
func (t *Trends) CostProjection(windowDays int) Projection {
	if windowDays <= 0 {
		windowDays = 30
	}
	n := len(t.Daily)
	if n < 7 {
		return Projection{}
	}
	if windowDays > n {
		windowDays = n
	}
	tail := t.Daily[n-windowDays:]
	var sumX, sumY, sumXY, sumXX float64
	for i, b := range tail {
		x := float64(i)
		y := b.CostUSD
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	w := float64(windowDays)
	denom := w*sumXX - sumX*sumX
	if denom == 0 {
		return Projection{WindowDays: windowDays, InterceptUSD: sumY / w}
	}
	slope := (w*sumXY - sumX*sumY) / denom
	intercept := (sumY - slope*sumX) / w
	// Project days [windowDays, windowDays+30).
	var projected float64
	for d := 0; d < 30; d++ {
		x := float64(windowDays + d)
		y := intercept + slope*x
		if y < 0 {
			y = 0
		}
		projected += y
	}
	return Projection{
		SlopeUSDPerDay:     slope,
		InterceptUSD:       intercept,
		ProjectedNext30USD: projected,
		WindowDays:         windowDays,
	}
}

// InsightSeverity ranks insights by importance for sorting in the UI.
type InsightSeverity int

const (
	InsightInfo InsightSeverity = iota
	InsightNotice
	InsightWarning
)

func (s InsightSeverity) String() string {
	switch s {
	case InsightWarning:
		return "warning"
	case InsightNotice:
		return "notice"
	default:
		return "info"
	}
}

// Insight is one heuristically-derived observation about the trends.
type Insight struct {
	ID       string
	Title    string
	Detail   string
	Severity InsightSeverity
}

// Insights runs the threshold-based rules and returns the produced insights,
// sorted from most-severe to least.
func (t *Trends) Insights() []Insight {
	var out []Insight
	out = append(out, modelMigrationInsight(t)...)
	out = append(out, costTrajectoryInsight(t)...)
	out = append(out, featureAdoptionInsight(t)...)
	out = append(out, efficiencyInsight(t)...)
	out = append(out, reliabilityInsight(t)...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Severity > out[j].Severity })
	return out
}

// --- individual rules ---

func modelMigrationInsight(t *Trends) []Insight {
	if len(t.Weekly) < 2 {
		return nil
	}
	prev := t.Weekly[len(t.Weekly)-2]
	cur := t.Weekly[len(t.Weekly)-1]
	if cur.Calls == 0 || prev.Calls == 0 {
		return nil
	}
	// Find any model whose share of weekly calls jumped >= 25 percentage points.
	prevShare := func(m string) float64 { return float64(prev.PerModel[m]) / float64(prev.Calls) }
	curShare := func(m string) float64 { return float64(cur.PerModel[m]) / float64(cur.Calls) }

	var out []Insight
	seen := map[string]bool{}
	for m := range cur.PerModel {
		seen[m] = true
		delta := curShare(m) - prevShare(m)
		if delta >= 0.25 {
			out = append(out, Insight{
				ID:       "model-migration-up:" + m,
				Title:    fmt.Sprintf("Migrating to %s", m),
				Detail:   fmt.Sprintf("%s rose from %.0f%% to %.0f%% of calls week-over-week.", m, prevShare(m)*100, curShare(m)*100),
				Severity: InsightInfo,
			})
		}
	}
	for m := range prev.PerModel {
		if seen[m] {
			continue
		}
		if prevShare(m) >= 0.25 {
			out = append(out, Insight{
				ID:       "model-migration-down:" + m,
				Title:    fmt.Sprintf("Stopped using %s", m),
				Detail:   fmt.Sprintf("%s was %.0f%% of last week's calls but 0%% this week.", m, prevShare(m)*100),
				Severity: InsightNotice,
			})
		}
	}
	return out
}

func costTrajectoryInsight(t *Trends) []Insight {
	p := t.CostProjection(14)
	if p.WindowDays == 0 {
		return nil
	}
	var out []Insight
	if p.SlopeUSDPerDay > 0.10 {
		out = append(out, Insight{
			ID:       "cost-trending-up",
			Title:    "Costs trending up",
			Detail:   fmt.Sprintf("Daily spend is rising by ~$%.2f/day. Linear projection: $%.2f over the next 30 days.", p.SlopeUSDPerDay, p.ProjectedNext30USD),
			Severity: InsightWarning,
		})
	} else if p.SlopeUSDPerDay < -0.10 {
		out = append(out, Insight{
			ID:       "cost-trending-down",
			Title:    "Costs trending down",
			Detail:   fmt.Sprintf("Daily spend is falling by ~$%.2f/day.", math.Abs(p.SlopeUSDPerDay)),
			Severity: InsightInfo,
		})
	}
	return out
}

func featureAdoptionInsight(t *Trends) []Insight {
	if len(t.Weekly) < 2 {
		return nil
	}
	prev := t.Weekly[len(t.Weekly)-2]
	cur := t.Weekly[len(t.Weekly)-1]
	var out []Insight
	for f, n := range cur.PerFeature {
		if prev.PerFeature[f] == 0 && n >= 5 {
			out = append(out, Insight{
				ID:       "feature-adopted:" + f,
				Title:    fmt.Sprintf("Started using %s", f),
				Detail:   fmt.Sprintf("%d uses this week, none last week.", n),
				Severity: InsightInfo,
			})
		}
	}
	for f, n := range prev.PerFeature {
		if cur.PerFeature[f] == 0 && n >= 10 {
			out = append(out, Insight{
				ID:       "feature-dropped:" + f,
				Title:    fmt.Sprintf("Stopped using %s", f),
				Detail:   fmt.Sprintf("%d uses last week, none this week.", n),
				Severity: InsightNotice,
			})
		}
	}
	return out
}

func efficiencyInsight(t *Trends) []Insight {
	if len(t.Weekly) < 2 {
		return nil
	}
	prev := t.Weekly[len(t.Weekly)-2]
	cur := t.Weekly[len(t.Weekly)-1]
	if prev.AvgTokensPerSec <= 0 || cur.AvgTokensPerSec <= 0 {
		return nil
	}
	delta := (cur.AvgTokensPerSec - prev.AvgTokensPerSec) / prev.AvgTokensPerSec
	if delta >= 0.20 {
		return []Insight{{
			ID:       "efficiency-up",
			Title:    "Throughput improved",
			Detail:   fmt.Sprintf("Avg %.1f → %.1f tok/s week-over-week (+%.0f%%).", prev.AvgTokensPerSec, cur.AvgTokensPerSec, delta*100),
			Severity: InsightInfo,
		}}
	}
	if delta <= -0.20 {
		return []Insight{{
			ID:       "efficiency-down",
			Title:    "Throughput dropped",
			Detail:   fmt.Sprintf("Avg %.1f → %.1f tok/s week-over-week (%.0f%%).", prev.AvgTokensPerSec, cur.AvgTokensPerSec, delta*100),
			Severity: InsightNotice,
		}}
	}
	return nil
}

func reliabilityInsight(t *Trends) []Insight {
	if len(t.Weekly) == 0 {
		return nil
	}
	cur := t.Weekly[len(t.Weekly)-1]
	if cur.Calls < 10 {
		return nil
	}
	rate := float64(cur.Errors) / float64(cur.Calls)
	if rate >= 0.05 {
		return []Insight{{
			ID:       "error-rate-high",
			Title:    "Elevated error rate",
			Detail:   fmt.Sprintf("%.1f%% of calls failed this week (%d / %d).", rate*100, cur.Errors, cur.Calls),
			Severity: InsightWarning,
		}}
	}
	if len(t.Weekly) >= 2 {
		prev := t.Weekly[len(t.Weekly)-2]
		if prev.Calls >= 10 {
			prevRate := float64(prev.Errors) / float64(prev.Calls)
			if rate > 0 && prevRate > 0 && rate <= prevRate/2 {
				return []Insight{{
					ID:       "error-rate-improved",
					Title:    "Reliability improving",
					Detail:   fmt.Sprintf("Error rate fell from %.1f%% to %.1f%% week-over-week.", prevRate*100, rate*100),
					Severity: InsightInfo,
				}}
			}
		}
	}
	return nil
}

// --- helpers ---

func entryTime(e contracts.UsageEntry) time.Time {
	if !e.Timestamp.IsZero() {
		return e.Timestamp
	}
	return e.At
}

func effectiveTokensIn(e contracts.UsageEntry) int {
	if e.TokensIn > 0 {
		return e.TokensIn
	}
	return e.InputTokens
}

func effectiveTokensOut(e contracts.UsageEntry) int {
	if e.TokensOut > 0 {
		return e.TokensOut
	}
	return e.OutputTokens
}

func isError(e contracts.UsageEntry) bool {
	if e.ErrorType != "" {
		return true
	}
	s := strings.ToLower(e.Status)
	return s != "" && s != "ok" && s != "success" && s != "completed"
}

func truncDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func weekStart(t time.Time) time.Time {
	t = truncDay(t)
	// ISO week starts Monday.
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	return t.AddDate(0, 0, -(wd - 1))
}
