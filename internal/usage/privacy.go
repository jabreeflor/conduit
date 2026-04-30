package usage

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// ExportFormat names the supported raw usage export encodings.
type ExportFormat string

const (
	ExportFormatJSON ExportFormat = "json"
	ExportFormatCSV  ExportFormat = "csv"
)

// PurgeOptions controls selective usage-log deletion. Empty options are invalid
// so a typo cannot accidentally wipe data.
type PurgeOptions struct {
	Before *time.Time
	Model  string
}

// AggregateReport is the prompt-free report shape intended for expenses and
// accounting workflows. It deliberately omits sessions and per-call detail.
type AggregateReport struct {
	GeneratedAt time.Time           `json:"generated_at"`
	From        *time.Time          `json:"from,omitempty"`
	To          *time.Time          `json:"to,omitempty"`
	Total       AggregateBucket     `json:"total"`
	Providers   []AggregateBucket   `json:"providers"`
	Models      []AggregateBucket   `json:"models"`
	ByDay       []DailyUsageSummary `json:"by_day"`
}

// AggregateBucket summarizes usage for one provider/model or the grand total.
type AggregateBucket struct {
	Key          string  `json:"key"`
	Requests     int     `json:"requests"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

// DailyUsageSummary summarizes usage for one UTC day.
type DailyUsageSummary struct {
	Date        string  `json:"date"`
	Requests    int     `json:"requests"`
	TotalTokens int     `json:"total_tokens"`
	CostUSD     float64 `json:"cost_usd"`
}

// DefaultLogPath resolves the local usage log path under the current user's
// home directory.
func DefaultLogPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("usage: resolve home dir: %w", err)
	}
	return filepath.Join(home, defaultLogPath), nil
}

// ReadLog loads valid usage entries from logPath. Missing logs return no rows.
func ReadLog(logPath string) ([]contracts.UsageEntry, error) {
	f, err := os.Open(logPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("usage: open log: %w", err)
	}
	defer f.Close()

	var entries []contracts.UsageEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry contracts.UsageEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, scanner.Err()
}

// Purge rewrites the log without entries matched by opts and returns the number
// of valid usage entries removed.
func Purge(logPath string, opts PurgeOptions) (int, error) {
	if opts.Before == nil && opts.Model == "" {
		return 0, errors.New("usage: purge requires --before and/or --model")
	}

	f, err := os.Open(logPath)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("usage: open log: %w", err)
	}
	defer f.Close()

	var kept [][]byte
	removed := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		var entry contracts.UsageEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			kept = append(kept, line)
			continue
		}
		if purgeMatches(entry, opts) {
			removed++
			continue
		}
		kept = append(kept, line)
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}

	tmp, err := os.CreateTemp(filepath.Dir(logPath), ".usage-purge-*")
	if err != nil {
		return 0, fmt.Errorf("usage: create temp log: %w", err)
	}
	tmpName := tmp.Name()
	for _, line := range kept {
		if _, err := tmp.Write(append(line, '\n')); err != nil {
			tmp.Close()
			os.Remove(tmpName)
			return 0, fmt.Errorf("usage: write temp log: %w", err)
		}
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return 0, fmt.Errorf("usage: close temp log: %w", err)
	}
	if err := os.Rename(tmpName, logPath); err != nil {
		os.Remove(tmpName)
		return 0, fmt.Errorf("usage: replace log: %w", err)
	}
	return removed, nil
}

// ExportRaw writes valid usage entries as JSON or CSV.
func ExportRaw(entries []contracts.UsageEntry, format ExportFormat, w io.Writer) error {
	switch format {
	case ExportFormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	case ExportFormatCSV:
		cw := csv.NewWriter(w)
		if err := cw.Write([]string{"at", "session_id", "provider", "model", "input_tokens", "output_tokens", "total_tokens", "cost_usd"}); err != nil {
			return err
		}
		for _, entry := range entries {
			if err := cw.Write([]string{
				entry.At.Format(time.RFC3339),
				entry.SessionID,
				entry.Provider,
				entry.Model,
				strconv.Itoa(entry.InputTokens),
				strconv.Itoa(entry.OutputTokens),
				strconv.Itoa(entry.TotalTokens),
				strconv.FormatFloat(entry.CostUSD, 'f', 6, 64),
			}); err != nil {
				return err
			}
		}
		cw.Flush()
		return cw.Error()
	default:
		return fmt.Errorf("usage: unsupported export format %q", format)
	}
}

// BuildAggregateReport creates an aggregate-only view with no session IDs or
// per-call records.
func BuildAggregateReport(entries []contracts.UsageEntry, from, to *time.Time) AggregateReport {
	report := AggregateReport{
		GeneratedAt: time.Now().UTC(),
		From:        from,
		To:          to,
	}
	byProvider := map[string]*AggregateBucket{}
	byModel := map[string]*AggregateBucket{}
	byDay := map[string]*DailyUsageSummary{}

	for _, entry := range entries {
		if !inRange(entry.At, from, to) {
			continue
		}
		addBucket(&report.Total, entry)
		addBucket(bucketFor(byProvider, entry.Provider), entry)
		addBucket(bucketFor(byModel, entry.Model), entry)

		dayKey := entry.At.UTC().Format("2006-01-02")
		day := byDay[dayKey]
		if day == nil {
			day = &DailyUsageSummary{Date: dayKey}
			byDay[dayKey] = day
		}
		day.Requests++
		day.TotalTokens += entry.TotalTokens
		day.CostUSD += entry.CostUSD
	}

	report.Providers = sortedBuckets(byProvider)
	report.Models = sortedBuckets(byModel)
	report.ByDay = sortedDays(byDay)
	return report
}

// WriteAggregateReport writes an aggregate report as indented JSON.
func WriteAggregateReport(report AggregateReport, w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// WriteDashboardHTML writes a self-contained offline usage dashboard. The JSON
// data is embedded locally and no external scripts, fonts, or beacons are used.
func WriteDashboardHTML(report AggregateReport, w io.Writer) error {
	data, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("usage: marshal dashboard data: %w", err)
	}
	rangeLabel := "All time"
	if report.From != nil || report.To != nil {
		from := "beginning"
		to := "now"
		if report.From != nil {
			from = report.From.Format("2006-01-02")
		}
		if report.To != nil {
			to = report.To.Format("2006-01-02")
		}
		rangeLabel = from + " to " + to
	}

	_, err = fmt.Fprintf(w, `<!doctype html>
<html lang="en">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Conduit Usage Dashboard</title>
<style>
:root { color-scheme: light dark; font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
body { margin: 0; background: #f7f5ef; color: #1f2933; }
main { max-width: 1080px; margin: 0 auto; padding: 32px 20px 48px; }
h1 { font-size: 28px; margin: 0 0 6px; }
h2 { font-size: 18px; margin: 28px 0 12px; }
.muted { color: #62717f; }
.grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 12px; margin-top: 20px; }
.card, table { background: #ffffff; border: 1px solid #d8dee6; border-radius: 8px; }
.card { padding: 16px; }
.label { color: #62717f; font-size: 13px; }
.value { font-size: 24px; font-weight: 700; margin-top: 4px; }
table { width: 100%%; border-collapse: collapse; overflow: hidden; }
th, td { padding: 10px 12px; border-bottom: 1px solid #e5e9ef; text-align: left; }
th { background: #eef2f6; font-size: 12px; text-transform: uppercase; letter-spacing: .04em; color: #475663; }
td.num, th.num { text-align: right; font-variant-numeric: tabular-nums; }
tr:last-child td { border-bottom: 0; }
.bar { height: 8px; background: #dfe7ee; border-radius: 999px; overflow: hidden; min-width: 90px; }
.fill { height: 100%%; background: #2f8f83; }
@media (prefers-color-scheme: dark) {
  body { background: #17191c; color: #eef2f6; }
  .muted, .label { color: #9aa7b4; }
  .card, table { background: #20252b; border-color: #3a444f; }
  th { background: #2b333c; color: #b9c5d0; }
  th, td { border-bottom-color: #36414c; }
  .bar { background: #3a444f; }
}
</style>
<main>
<h1>Conduit Usage Dashboard</h1>
<div class="muted">%s - generated %s - local-only, self-contained export</div>
<section class="grid">
<div class="card"><div class="label">Requests</div><div class="value">%d</div></div>
<div class="card"><div class="label">Tokens</div><div class="value">%d</div></div>
<div class="card"><div class="label">Cost</div><div class="value">$%.4f</div></div>
</section>
%s
%s
%s
<script type="application/json" id="usage-data">%s</script>
</main>
</html>
`, html.EscapeString(rangeLabel), report.GeneratedAt.Format(time.RFC3339), report.Total.Requests, report.Total.TotalTokens, report.Total.CostUSD,
		bucketTable("Providers", report.Providers, report.Total.CostUSD),
		bucketTable("Models", report.Models, report.Total.CostUSD),
		dayTable(report.ByDay),
		html.EscapeString(string(data)))
	return err
}

func purgeMatches(entry contracts.UsageEntry, opts PurgeOptions) bool {
	if opts.Before != nil && entry.At.Before(*opts.Before) {
		return true
	}
	return opts.Model != "" && entry.Model == opts.Model
}

func inRange(at time.Time, from, to *time.Time) bool {
	if from != nil && at.Before(*from) {
		return false
	}
	if to != nil && !at.Before(*to) {
		return false
	}
	return true
}

func bucketFor(buckets map[string]*AggregateBucket, key string) *AggregateBucket {
	if key == "" {
		key = "unknown"
	}
	b := buckets[key]
	if b == nil {
		b = &AggregateBucket{Key: key}
		buckets[key] = b
	}
	return b
}

func addBucket(bucket *AggregateBucket, entry contracts.UsageEntry) {
	bucket.Requests++
	bucket.InputTokens += entry.InputTokens
	bucket.OutputTokens += entry.OutputTokens
	bucket.TotalTokens += entry.TotalTokens
	bucket.CostUSD += entry.CostUSD
}

func sortedBuckets(src map[string]*AggregateBucket) []AggregateBucket {
	out := make([]AggregateBucket, 0, len(src))
	for _, bucket := range src {
		out = append(out, *bucket)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CostUSD == out[j].CostUSD {
			return out[i].Key < out[j].Key
		}
		return out[i].CostUSD > out[j].CostUSD
	})
	return out
}

func sortedDays(src map[string]*DailyUsageSummary) []DailyUsageSummary {
	out := make([]DailyUsageSummary, 0, len(src))
	for _, day := range src {
		out = append(out, *day)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

func bucketTable(title string, buckets []AggregateBucket, totalCost float64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<h2>%s</h2><table><thead><tr><th>Name</th><th class=\"num\">Requests</th><th class=\"num\">Tokens</th><th class=\"num\">Cost</th><th>Share</th></tr></thead><tbody>", html.EscapeString(title))
	for _, bucket := range buckets {
		share := 0.0
		if totalCost > 0 {
			share = bucket.CostUSD / totalCost * 100
		}
		fmt.Fprintf(&b, "<tr><td>%s</td><td class=\"num\">%d</td><td class=\"num\">%d</td><td class=\"num\">$%.4f</td><td><div class=\"bar\"><div class=\"fill\" style=\"width: %.2f%%\"></div></div></td></tr>",
			html.EscapeString(bucket.Key), bucket.Requests, bucket.TotalTokens, bucket.CostUSD, share)
	}
	if len(buckets) == 0 {
		b.WriteString("<tr><td colspan=\"5\">No usage recorded.</td></tr>")
	}
	b.WriteString("</tbody></table>")
	return b.String()
}

func dayTable(days []DailyUsageSummary) string {
	var b strings.Builder
	b.WriteString("<h2>Daily Totals</h2><table><thead><tr><th>Date</th><th class=\"num\">Requests</th><th class=\"num\">Tokens</th><th class=\"num\">Cost</th></tr></thead><tbody>")
	for _, day := range days {
		fmt.Fprintf(&b, "<tr><td>%s</td><td class=\"num\">%d</td><td class=\"num\">%d</td><td class=\"num\">$%.4f</td></tr>",
			html.EscapeString(day.Date), day.Requests, day.TotalTokens, day.CostUSD)
	}
	if len(days) == 0 {
		b.WriteString("<tr><td colspan=\"4\">No usage recorded.</td></tr>")
	}
	b.WriteString("</tbody></table>")
	return b.String()
}
