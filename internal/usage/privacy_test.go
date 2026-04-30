package usage

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestPurge_filtersByBeforeAndModel(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "usage.jsonl")
	old := usageEntry("2026-01-01T00:00:00Z", "anthropic", "claude-haiku-4-5", 0.10)
	kept := usageEntry("2026-02-01T00:00:00Z", "openai", "gpt-4o-mini", 0.20)
	model := usageEntry("2026-03-01T00:00:00Z", "openai", "gpt-4o", 0.30)
	writeUsageLog(t, logPath, old, kept, model)

	before := mustDate(t, "2026-01-15")
	removed, err := Purge(logPath, PurgeOptions{Before: &before, Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}

	entries, err := ReadLog(logPath)
	if err != nil {
		t.Fatalf("ReadLog: %v", err)
	}
	if len(entries) != 1 || entries[0].Model != kept.Model {
		t.Fatalf("entries = %#v, want only kept model", entries)
	}
}

func TestExportRaw_writesCSVAndJSON(t *testing.T) {
	entries := []contracts.UsageEntry{usageEntry("2026-01-01T00:00:00Z", "anthropic", "claude-haiku-4-5", 0.125)}

	var csvOut bytes.Buffer
	if err := ExportRaw(entries, ExportFormatCSV, &csvOut); err != nil {
		t.Fatalf("ExportRaw CSV: %v", err)
	}
	if got := csvOut.String(); !strings.Contains(got, "session_id,provider,model") || !strings.Contains(got, "claude-haiku-4-5") {
		t.Fatalf("CSV output = %q", got)
	}

	var jsonOut bytes.Buffer
	if err := ExportRaw(entries, ExportFormatJSON, &jsonOut); err != nil {
		t.Fatalf("ExportRaw JSON: %v", err)
	}
	var parsed []contracts.UsageEntry
	if err := json.Unmarshal(jsonOut.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON output did not parse: %v", err)
	}
	if len(parsed) != 1 || parsed[0].Provider != "anthropic" {
		t.Fatalf("parsed = %#v", parsed)
	}
}

func TestBuildAggregateReport_omitsSessionAndGroupsTotals(t *testing.T) {
	entries := []contracts.UsageEntry{
		usageEntry("2026-01-01T00:00:00Z", "anthropic", "claude-haiku-4-5", 0.10),
		usageEntry("2026-01-02T00:00:00Z", "anthropic", "claude-haiku-4-5", 0.20),
		usageEntry("2026-01-02T00:00:00Z", "openai", "gpt-4o", 0.30),
	}

	report := BuildAggregateReport(entries, nil, nil)
	if report.Total.Requests != 3 || math.Abs(report.Total.CostUSD-0.60) > 0.000001 {
		t.Fatalf("total = %#v", report.Total)
	}
	if len(report.Providers) != 2 || report.Providers[0].Key != "anthropic" {
		t.Fatalf("providers = %#v", report.Providers)
	}

	var out bytes.Buffer
	if err := WriteAggregateReport(report, &out); err != nil {
		t.Fatalf("WriteAggregateReport: %v", err)
	}
	if strings.Contains(out.String(), "session_id") || strings.Contains(out.String(), "sess-") {
		t.Fatalf("aggregate report leaked session detail: %s", out.String())
	}
}

func TestWriteDashboardHTML_isSelfContained(t *testing.T) {
	report := BuildAggregateReport([]contracts.UsageEntry{
		usageEntry("2026-01-01T00:00:00Z", "anthropic", "claude-haiku-4-5", 0.10),
	}, nil, nil)

	var out bytes.Buffer
	if err := WriteDashboardHTML(report, &out); err != nil {
		t.Fatalf("WriteDashboardHTML: %v", err)
	}
	html := out.String()
	for _, forbidden := range []string{"https://", "http://", "src=", "href="} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("dashboard is not self-contained, found %q in %s", forbidden, html)
		}
	}
	if !strings.Contains(html, "Conduit Usage Dashboard") || !strings.Contains(html, "application/json") {
		t.Fatalf("dashboard missing expected content: %s", html)
	}
}

func usageEntry(atRaw, provider, model string, cost float64) contracts.UsageEntry {
	at, _ := time.Parse(time.RFC3339, atRaw)
	return contracts.UsageEntry{
		At:           at,
		SessionID:    "sess-private",
		Provider:     provider,
		Model:        model,
		InputTokens:  100,
		OutputTokens: 50,
		TotalTokens:  150,
		CostUSD:      cost,
	}
}

func writeUsageLog(t *testing.T, path string, entries ...contracts.UsageEntry) {
	t.Helper()
	var b bytes.Buffer
	for _, entry := range entries {
		line, err := json.Marshal(entry)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
}

func mustDate(t *testing.T, raw string) time.Time {
	t.Helper()
	at, err := time.Parse("2006-01-02", raw)
	if err != nil {
		t.Fatalf("parse date: %v", err)
	}
	return at
}
