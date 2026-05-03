package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func writeEntries(t *testing.T, dir string, entries []contracts.UsageEntry) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Group by date for daily-log realism.
	files := map[string][]contracts.UsageEntry{}
	for _, e := range entries {
		key := e.Timestamp.UTC().Format("2006-01-02") + ".jsonl"
		files[key] = append(files[key], e)
	}
	for name, list := range files {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		enc := json.NewEncoder(f)
		for _, e := range list {
			if err := enc.Encode(e); err != nil {
				t.Fatal(err)
			}
		}
		f.Close()
	}
}

func TestQueryAggregatesPerPluginAndModel(t *testing.T) {
	dir := t.TempDir()
	t0 := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	writeEntries(t, dir, []contracts.UsageEntry{
		{Plugin: "p1", Model: "m1", Provider: "anthropic", Timestamp: t0, TokensIn: 10, TokensOut: 5, TotalTokens: 15, CostUSD: 0.01, TotalMS: 100, Status: "ok"},
		{Plugin: "p1", Model: "m1", Provider: "anthropic", Timestamp: t0, TokensIn: 20, TokensOut: 10, TotalTokens: 30, CostUSD: 0.02, TotalMS: 200, Status: "error"},
		{Plugin: "p1", Model: "m2", Provider: "anthropic", Timestamp: t0, TokensIn: 5, TokensOut: 5, TotalTokens: 10, CostUSD: 0.005, TotalMS: 50, Status: "ok"},
		{Plugin: "p2", Model: "m1", Provider: "anthropic", Timestamp: t0, TokensIn: 1, TokensOut: 1, TotalTokens: 2, CostUSD: 0.001, TotalMS: 10, Status: "ok"},
		{Plugin: "", Model: "m1", Provider: "anthropic", Timestamp: t0, TotalTokens: 100}, // ignored
	})
	rows, err := Query(dir, PluginQuery{})
	if err != nil {
		t.Fatal(err)
	}
	// p1 has 2 models -> 2 rows + 1 rollup = 3 rows; p2 has 1 model -> 1 row.
	if len(rows) != 4 {
		t.Fatalf("want 4 rows, got %d (%+v)", len(rows), rows)
	}
	var p1All *PluginUsage
	for i, r := range rows {
		if r.PluginID == "p1" && r.Model == "" {
			p1All = &rows[i]
		}
	}
	if p1All == nil {
		t.Fatal("expected p1 rollup row")
	}
	if p1All.Requests != 3 || p1All.TotalTokens != 55 || p1All.Errors != 1 {
		t.Fatalf("rollup wrong: %+v", p1All)
	}
	if want := 1.0 / 3.0; p1All.ErrorRate < want-1e-9 || p1All.ErrorRate > want+1e-9 {
		t.Fatalf("error rate wrong: %v", p1All.ErrorRate)
	}
}

func TestQueryFilterByPluginAndDate(t *testing.T) {
	dir := t.TempDir()
	t1 := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	writeEntries(t, dir, []contracts.UsageEntry{
		{Plugin: "p1", Model: "m", Timestamp: t1, TotalTokens: 10, Status: "ok"},
		{Plugin: "p2", Model: "m", Timestamp: t1, TotalTokens: 10, Status: "ok"},
		{Plugin: "p1", Model: "m", Timestamp: t2, TotalTokens: 10, Status: "ok"},
	})
	rows, err := Query(dir, PluginQuery{PluginID: "p1", From: t1.Add(-time.Hour), To: t1.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].PluginID != "p1" || rows[0].Requests != 1 {
		t.Fatalf("want filtered single row, got %+v", rows)
	}
}

func TestDetectRequestSpike(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2024, 5, 8, 0, 0, 0, 0, time.UTC)
	// 7-day baseline of 1 request each, then 50 in the most recent window.
	var entries []contracts.UsageEntry
	for d := 7; d >= 1; d-- {
		entries = append(entries, contracts.UsageEntry{
			Plugin: "p1", Model: "m", Timestamp: now.AddDate(0, 0, -d).Add(2 * time.Hour),
			TotalTokens: 10, Status: "ok", CostUSD: 0.001,
		})
	}
	for i := 0; i < 50; i++ {
		entries = append(entries, contracts.UsageEntry{
			Plugin: "p1", Model: "m", Timestamp: now.Add(-time.Hour),
			TotalTokens: 10, Status: "ok", CostUSD: 0.001,
		})
	}
	writeEntries(t, dir, entries)
	out, err := Detect(dir, AnomalyOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	hasSpike := false
	for _, a := range out {
		if a.Kind == AnomalyRequestsSpike && a.PluginID == "p1" {
			hasSpike = true
		}
	}
	if !hasSpike {
		t.Fatalf("expected request spike, got %+v", out)
	}
}

func TestDetectErrorRateSpike(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2024, 5, 8, 0, 0, 0, 0, time.UTC)
	var entries []contracts.UsageEntry
	// Baseline: 10 ok per day for 7 days, no errors.
	for d := 7; d >= 1; d-- {
		for i := 0; i < 10; i++ {
			entries = append(entries, contracts.UsageEntry{
				Plugin: "p", Model: "m", Status: "ok",
				Timestamp: now.AddDate(0, 0, -d).Add(time.Hour),
			})
		}
	}
	// Current window: 5 ok + 5 error -> 50% error rate.
	for i := 0; i < 5; i++ {
		entries = append(entries, contracts.UsageEntry{Plugin: "p", Model: "m", Status: "ok", Timestamp: now.Add(-time.Hour)})
		entries = append(entries, contracts.UsageEntry{Plugin: "p", Model: "m", Status: "error", Timestamp: now.Add(-time.Hour)})
	}
	writeEntries(t, dir, entries)
	out, err := Detect(dir, AnomalyOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range out {
		if a.Kind == AnomalyErrorRateSpike {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected error rate spike, got %+v", out)
	}
}

func TestIsErrorStatus(t *testing.T) {
	cases := map[string]bool{"": false, "ok": false, "OK": false, "success": false, "error": true, "fail": true, "timeout": true}
	for s, want := range cases {
		if isErrorStatus(s) != want {
			t.Errorf("isErrorStatus(%q)=%v want %v", s, !want, want)
		}
	}
}
