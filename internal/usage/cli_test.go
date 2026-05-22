package usage

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// writeJSONL writes one entry per line to a fresh log file.
func writeJSONL(t *testing.T, dir, name string, entries []contracts.UsageEntry, gz bool) {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	var w interface{ Write([]byte) (int, error) } = f
	var gzw *gzip.Writer
	if gz {
		gzw = gzip.NewWriter(f)
		w = gzw
	}
	for _, e := range entries {
		buf, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if _, err := w.Write(append(buf, '\n')); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if gzw != nil {
		if err := gzw.Close(); err != nil {
			t.Fatalf("gzip close: %v", err)
		}
	}
}

func mkEntry(ts time.Time, model string, tokensIn, tokensOut int, cost float64) contracts.UsageEntry {
	return contracts.UsageEntry{
		Timestamp:   ts,
		SessionID:   "sess-test",
		Provider:    "anthropic",
		Model:       model,
		TokensIn:    tokensIn,
		TokensOut:   tokensOut,
		TotalTokens: tokensIn + tokensOut,
		Status:      "success",
		CostUSD:     cost,
	}
}

func TestPurge_byDate_dropsOlderEntries(t *testing.T) {
	dir := t.TempDir()
	entries := []contracts.UsageEntry{
		mkEntry(time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), "haiku", 100, 50, 0.01),
		mkEntry(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), "haiku", 200, 100, 0.02),
	}
	writeJSONL(t, dir, "2026-01-05.jsonl", entries[:1], false)
	writeJSONL(t, dir, "2026-04-01.jsonl", entries[1:], false)

	report, err := Purge(dir, PurgeOptions{Before: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if report.Dropped != 1 || report.Kept != 1 {
		t.Fatalf("dropped=%d kept=%d, want 1/1", report.Dropped, report.Kept)
	}
	// Older file should be removed entirely.
	if _, err := os.Stat(filepath.Join(dir, "2026-01-05.jsonl")); !os.IsNotExist(err) {
		t.Errorf("expected 2026-01-05.jsonl removed, stat err = %v", err)
	}
	// Newer file should still exist with one entry.
	logs, _ := ListLogs(dir)
	if len(logs) != 1 {
		t.Fatalf("logs = %d, want 1", len(logs))
	}
}

func TestPurge_byModel_handlesGzippedFiles(t *testing.T) {
	dir := t.TempDir()
	entries := []contracts.UsageEntry{
		mkEntry(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), "haiku", 100, 50, 0.01),
		mkEntry(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), "sonnet", 200, 100, 0.05),
	}
	writeJSONL(t, dir, "2026-04-01.jsonl.gz", entries, true)

	report, err := Purge(dir, PurgeOptions{Model: "sonnet"})
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if report.Dropped != 1 || report.Kept != 1 {
		t.Fatalf("dropped=%d kept=%d, want 1/1", report.Dropped, report.Kept)
	}

	// File should still be gzipped after rewrite.
	logs, _ := ListLogs(dir)
	if len(logs) != 1 || !logs[0].Compressed {
		t.Fatalf("expected gzipped log retained, got %+v", logs)
	}
	var got []contracts.UsageEntry
	_ = ScanLog(logs[0], func(e contracts.UsageEntry) bool {
		got = append(got, e)
		return true
	})
	if len(got) != 1 || got[0].Model != "haiku" {
		t.Errorf("kept entries = %+v, want [haiku]", got)
	}
}

func TestPurge_dryRun_doesNotMutate(t *testing.T) {
	dir := t.TempDir()
	entries := []contracts.UsageEntry{
		mkEntry(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), "haiku", 100, 50, 0.01),
	}
	writeJSONL(t, dir, "2026-04-01.jsonl", entries, false)

	report, err := Purge(dir, PurgeOptions{Model: "haiku", DryRun: true})
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if report.Dropped != 1 {
		t.Errorf("Dropped = %d, want 1", report.Dropped)
	}
	// File still present.
	if _, err := os.Stat(filepath.Join(dir, "2026-04-01.jsonl")); err != nil {
		t.Errorf("file should still exist after dry run: %v", err)
	}
}

func TestPurge_requiresScope(t *testing.T) {
	if _, err := Purge(t.TempDir(), PurgeOptions{}); err == nil {
		t.Error("expected error for unscoped purge, got nil")
	}
}

func TestRunPurge_requiresYesFlagForDestructive(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "2026-04-01.jsonl",
		[]contracts.UsageEntry{mkEntry(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), "haiku", 1, 1, 0)},
		false)

	var out, errBuf bytes.Buffer
	err := runPurge([]string{"--model", "haiku"}, &out, &errBuf, dir)
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected --yes error, got %v", err)
	}
}

func TestExport_csv_filtersByDateRange(t *testing.T) {
	dir := t.TempDir()
	entries := []contracts.UsageEntry{
		mkEntry(time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), "haiku", 100, 50, 0.01),
		mkEntry(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), "sonnet", 200, 100, 0.05),
		mkEntry(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), "opus", 300, 150, 0.5),
	}
	writeJSONL(t, dir, "2026-01-05.jsonl", entries[:1], false)
	writeJSONL(t, dir, "2026-04-01.jsonl", entries[1:2], false)
	writeJSONL(t, dir, "2026-05-01.jsonl", entries[2:], false)

	var buf bytes.Buffer
	err := Export(dir, ExportOptions{
		From:   time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		To:     time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		Format: ExportFormatCSV,
	}, &buf)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "sonnet") {
		t.Errorf("output missing sonnet entry: %q", got)
	}
	if strings.Contains(got, "haiku") || strings.Contains(got, "opus") {
		t.Errorf("output should exclude out-of-range entries: %q", got)
	}
	// Header row must be present.
	if !strings.HasPrefix(got, "timestamp,session_id,") {
		t.Errorf("missing CSV header: %q", got[:60])
	}
}

func TestExport_json_emitsValidArray(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "2026-04-01.jsonl",
		[]contracts.UsageEntry{
			mkEntry(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), "haiku", 1, 1, 0.01),
			mkEntry(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), "sonnet", 1, 1, 0.05),
		}, false)

	var buf bytes.Buffer
	if err := Export(dir, ExportOptions{Format: ExportFormatJSON}, &buf); err != nil {
		t.Fatalf("Export: %v", err)
	}
	var entries []contracts.UsageEntry
	if err := json.Unmarshal(buf.Bytes(), &entries); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, buf.String())
	}
	if len(entries) != 2 {
		t.Errorf("entries = %d, want 2", len(entries))
	}
}

func TestAggregate_groupByDay_dropsPerCallFields(t *testing.T) {
	dir := t.TempDir()
	day := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	writeJSONL(t, dir, "2026-04-01.jsonl",
		[]contracts.UsageEntry{
			mkEntry(day, "haiku", 100, 50, 0.01),
			mkEntry(day, "haiku", 200, 100, 0.02),
			mkEntry(day.Add(24*time.Hour), "haiku", 300, 150, 0.03),
		}, false)
	writeJSONL(t, dir, "2026-04-02.jsonl",
		[]contracts.UsageEntry{
			mkEntry(day.Add(24*time.Hour), "sonnet", 100, 50, 0.10),
		}, false)

	buckets, err := Aggregate(dir, AggregateOptions{GroupBy: GroupDay})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if len(buckets) != 3 {
		t.Fatalf("buckets = %d, want 3 (4-01 haiku, 4-02 haiku, 4-02 sonnet)", len(buckets))
	}
	// First bucket should be the 4-01 haiku rollup with 2 calls.
	if buckets[0].Period != "2026-04-01" || buckets[0].Calls != 2 || buckets[0].TotalTokens != 450 {
		t.Errorf("bucket[0] = %+v", buckets[0])
	}
}

func TestAggregate_groupByProvider_isCoarsest(t *testing.T) {
	dir := t.TempDir()
	day := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	writeJSONL(t, dir, "2026-04-01.jsonl",
		[]contracts.UsageEntry{
			mkEntry(day, "haiku", 100, 50, 0.01),
			mkEntry(day, "sonnet", 200, 100, 0.05),
		}, false)

	buckets, err := Aggregate(dir, AggregateOptions{GroupBy: GroupProvider})
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if len(buckets) != 1 {
		t.Fatalf("buckets = %d, want 1 (single provider)", len(buckets))
	}
	if buckets[0].Provider != "anthropic" || buckets[0].Calls != 2 {
		t.Errorf("bucket = %+v", buckets[0])
	}
	if buckets[0].Model != "" || buckets[0].Period != "" {
		t.Errorf("provider bucket leaked finer fields: %+v", buckets[0])
	}
}

func TestDashboard_isSelfContained(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "2026-04-01.jsonl",
		[]contracts.UsageEntry{mkEntry(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), "haiku", 100, 50, 0.01)},
		false)

	var buf bytes.Buffer
	if err := Dashboard(dir, DashboardOptions{}, &buf); err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	html := buf.String()
	if !strings.HasPrefix(html, "<!DOCTYPE html>") {
		t.Errorf("not an HTML document: %q", html[:80])
	}
	for _, forbidden := range []string{"<script src=", "<link rel=\"stylesheet\""} {
		if strings.Contains(html, forbidden) {
			t.Errorf("dashboard references external asset %q", forbidden)
		}
	}
	if !strings.Contains(html, "haiku") {
		t.Error("dashboard missing entry data")
	}
}

func TestRunCLI_unknownSubcommand(t *testing.T) {
	var out, errBuf bytes.Buffer
	err := RunCLI(context.Background(), []string{"bogus"}, &out, &errBuf)
	if err == nil {
		t.Error("expected error for unknown subcommand")
	}
}

func TestRunCLI_helpPrintsUsage(t *testing.T) {
	var out, errBuf bytes.Buffer
	if err := RunCLI(context.Background(), []string{}, &out, &errBuf); err != nil {
		t.Fatalf("RunCLI: %v", err)
	}
	if !strings.Contains(out.String(), "purge") || !strings.Contains(out.String(), "dashboard") {
		t.Errorf("usage missing subcommands: %q", out.String())
	}
}
