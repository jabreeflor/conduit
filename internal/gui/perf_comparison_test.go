package gui

import (
	"math"
	"testing"
	"time"
)

func perfSample(provider, model string, ttft, total int64, tokens int, cost float64, err, fb bool) PerfSample {
	return PerfSample{
		Timestamp:    time.Now(),
		Provider:     provider,
		Model:        model,
		TTFTMS:       ttft,
		TotalMS:      total,
		OutputTokens: tokens,
		CostUSD:      cost,
		Error:        err,
		Fallback:     fb,
	}
}

func almost(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestPerfComparison_RowsByModel(t *testing.T) {
	p := NewPerfComparison()
	p.Ingest(perfSample("anthropic", "opus", 100, 500, 1000, 0.05, false, false))
	p.Ingest(perfSample("anthropic", "opus", 200, 1000, 2000, 0.10, false, false))
	p.Ingest(perfSample("anthropic", "sonnet", 50, 200, 1000, 0.01, false, false))

	rows := p.Rows()
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	// Sorted ascending by total latency: sonnet (200) before opus (750 median).
	if rows[0].Model != "sonnet" {
		t.Errorf("first row = %q, want sonnet", rows[0].Model)
	}
}

func TestPerfComparison_TokensPerSec(t *testing.T) {
	p := NewPerfComparison()
	// 1000 tokens in 1000ms = 1000 tokens/sec.
	p.Ingest(perfSample("a", "m", 50, 1000, 1000, 0.01, false, false))
	rows := p.Rows()
	if !almost(rows[0].TokensPerSec, 1000, 0.001) {
		t.Errorf("tokens/sec = %f, want 1000", rows[0].TokensPerSec)
	}
}

func TestPerfComparison_ErrorAndFallbackRate(t *testing.T) {
	p := NewPerfComparison()
	for i := 0; i < 8; i++ {
		p.Ingest(perfSample("a", "m", 10, 100, 100, 0.01, false, false))
	}
	p.Ingest(perfSample("a", "m", 10, 100, 100, 0.01, true, false))
	p.Ingest(perfSample("a", "m", 10, 100, 100, 0.01, false, true))

	row := p.Rows()[0]
	if !almost(row.ErrorRate, 0.10, 0.01) {
		t.Errorf("error rate = %f, want ~0.10", row.ErrorRate)
	}
	if !almost(row.FallbackRate, 0.10, 0.01) {
		t.Errorf("fallback rate = %f, want ~0.10", row.FallbackRate)
	}
}

func TestPerfComparison_CostPer1K(t *testing.T) {
	p := NewPerfComparison()
	// 2 calls, 5000 tokens total, $0.10 total → $0.02 per 1K.
	p.Ingest(perfSample("a", "m", 10, 100, 2000, 0.04, false, false))
	p.Ingest(perfSample("a", "m", 10, 100, 3000, 0.06, false, false))
	row := p.Rows()[0]
	if !almost(row.CostPer1KTokens, 0.02, 0.0001) {
		t.Errorf("cost/1k = %f, want 0.02", row.CostPer1KTokens)
	}
}

func TestPerfComparison_QualityProxiesAveraged(t *testing.T) {
	p := NewPerfComparison()
	p.Ingest(PerfSample{Provider: "a", Model: "m", EditRate: 0.4, RetryRate: 0.0, CompletionRate: 1.0})
	p.Ingest(PerfSample{Provider: "a", Model: "m", EditRate: 0.6, RetryRate: 0.2, CompletionRate: 0.8})

	row := p.Rows()[0]
	if !almost(row.EditRate, 0.5, 0.001) {
		t.Errorf("edit = %f, want 0.5", row.EditRate)
	}
	if !almost(row.RetryRate, 0.1, 0.001) {
		t.Errorf("retry = %f, want 0.1", row.RetryRate)
	}
	if !almost(row.CompletionRate, 0.9, 0.001) {
		t.Errorf("completion = %f, want 0.9", row.CompletionRate)
	}
}

func TestPerfComparison_IncludeAllowlist(t *testing.T) {
	p := NewPerfComparison()
	p.Ingest(perfSample("a", "x", 10, 100, 100, 0.01, false, false))
	p.Ingest(perfSample("a", "y", 10, 100, 100, 0.01, false, false))
	p.Ingest(perfSample("b", "z", 10, 100, 100, 0.01, false, false))

	p.Include("a", "x")
	p.Include("b", "z")
	rows := p.Rows()
	if len(rows) != 2 {
		t.Errorf("rows = %d, want 2", len(rows))
	}
	for _, r := range rows {
		if r.Model == "y" {
			t.Error("y should be excluded by allowlist")
		}
	}
}

func TestPerfComparison_Exclude(t *testing.T) {
	p := NewPerfComparison()
	p.Ingest(perfSample("a", "x", 10, 100, 100, 0.01, false, false))
	p.Ingest(perfSample("a", "y", 10, 100, 100, 0.01, false, false))
	p.Exclude("a", "y")
	rows := p.Rows()
	if len(rows) != 1 || rows[0].Model != "x" {
		t.Errorf("rows = %+v, want only x", rows)
	}
}

func TestPerfComparison_Scatter(t *testing.T) {
	p := NewPerfComparison()
	p.Ingest(perfSample("a", "fast", 10, 100, 1000, 0.005, false, false))
	p.Ingest(perfSample("a", "slow", 100, 5000, 1000, 0.10, false, false))

	pts := p.Scatter()
	if len(pts) != 2 {
		t.Fatalf("scatter points = %d, want 2", len(pts))
	}
	// Fast first (lower latency).
	if pts[0].Model != "fast" {
		t.Errorf("first = %q, want fast", pts[0].Model)
	}
	if pts[0].LatencyMS != 100 || pts[1].LatencyMS != 5000 {
		t.Errorf("latencies = %f, %f", pts[0].LatencyMS, pts[1].LatencyMS)
	}
}

func TestPerfComparison_MedianHandlesEvenAndOdd(t *testing.T) {
	if median(nil) != 0 {
		t.Error("median of nil should be 0")
	}
	if median([]float64{5}) != 5 {
		t.Error("single-elem median")
	}
	if median([]float64{1, 2, 3}) != 2 {
		t.Error("odd median")
	}
	if median([]float64{1, 2, 3, 4}) != 2.5 {
		t.Error("even median")
	}
}

func TestPerfComparison_DisclaimerNonEmpty(t *testing.T) {
	if QualityProxyDisclaimer == "" {
		t.Error("disclaimer must be set so renderers can show it")
	}
}

func TestPerfComparison_Concurrent(t *testing.T) {
	p := NewPerfComparison()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			p.Ingest(perfSample("a", "m", 10, 100, 100, 0.01, false, false))
		}
		done <- struct{}{}
	}()
	for i := 0; i < 200; i++ {
		_ = p.Rows()
	}
	<-done
}
