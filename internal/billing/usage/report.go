package usage

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// AggregateGroup selects how the report buckets entries. Each value is a
// privacy-vs-utility trade-off — coarser groups leak less but are less useful.
type AggregateGroup string

const (
	GroupDay      AggregateGroup = "day"      // (date, provider, model)
	GroupMonth    AggregateGroup = "month"    // (year-month, provider, model)
	GroupModel    AggregateGroup = "model"    // (provider, model) only
	GroupProvider AggregateGroup = "provider" // (provider) only — coarsest
)

// AggregateOptions parameterises the aggregate report.
type AggregateOptions struct {
	From    time.Time
	To      time.Time
	GroupBy AggregateGroup
}

// Bucket is one row of the aggregate report. Bucket fields are intentionally
// limited to what is needed for expense reporting — no session IDs, no
// per-call latency, no feature/plugin attribution.
type Bucket struct {
	Period      string  `json:"period,omitempty"`   // YYYY-MM-DD or YYYY-MM, empty for non-time groups
	Provider    string  `json:"provider,omitempty"` // empty when grouping above provider
	Model       string  `json:"model,omitempty"`    // empty when grouping above model
	Calls       int     `json:"calls"`
	TotalTokens int     `json:"total_tokens"`
	CostUSD     float64 `json:"cost_usd"`
}

// Aggregate scans dir and returns sorted buckets per opts.GroupBy.
func Aggregate(dir string, opts AggregateOptions) ([]Bucket, error) {
	if opts.GroupBy == "" {
		opts.GroupBy = GroupDay
	}
	buckets := map[string]*Bucket{}
	exportRange := ExportOptions{From: opts.From, To: opts.To}

	err := ScanAll(dir, func(e contracts.UsageEntry) bool {
		if !inRange(e.Timestamp, exportRange) {
			return true
		}
		key, b := projectBucket(e, opts.GroupBy)
		existing, ok := buckets[key]
		if !ok {
			buckets[key] = &b
			existing = buckets[key]
		}
		existing.Calls++
		existing.TotalTokens += e.TotalTokens
		existing.CostUSD += e.CostUSD
		return true
	})
	if err != nil {
		return nil, err
	}

	out := make([]Bucket, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Period != out[j].Period {
			return out[i].Period < out[j].Period
		}
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Model < out[j].Model
	})
	return out, nil
}

// projectBucket maps one entry to the (key, fresh-bucket) pair for its group.
//
// PRIVACY NOTE: only fields explicitly carried forward end up in the report.
// Feature, plugin, session_id, error_type and per-call latency are dropped
// here on purpose — that is the "aggregate-only" guarantee for expense
// reporting. Adjust grouping with care; finer keys leak more about usage
// patterns. See README §usage for the rationale.
func projectBucket(e contracts.UsageEntry, group AggregateGroup) (string, Bucket) {
	day := e.Timestamp.UTC().Format("2006-01-02")
	month := e.Timestamp.UTC().Format("2006-01")
	switch group {
	case GroupMonth:
		return month + "|" + e.Provider + "|" + e.Model,
			Bucket{Period: month, Provider: e.Provider, Model: e.Model}
	case GroupModel:
		return e.Provider + "|" + e.Model,
			Bucket{Provider: e.Provider, Model: e.Model}
	case GroupProvider:
		return e.Provider,
			Bucket{Provider: e.Provider}
	default: // GroupDay
		return day + "|" + e.Provider + "|" + e.Model,
			Bucket{Period: day, Provider: e.Provider, Model: e.Model}
	}
}

func runReport(args []string, stdout, stderr io.Writer, dir string) error {
	fs := flag.NewFlagSet("usage report", flag.ContinueOnError)
	fs.SetOutput(stderr)
	group := fs.String("group", "day", "aggregation level: day, month, model, provider")
	from := fs.String("from", "", "include entries on/after this date (YYYY-MM-DD, UTC)")
	to := fs.String("to", "", "include entries strictly before this date (YYYY-MM-DD, UTC)")
	asJSON := fs.Bool("json", false, "emit JSON instead of a text table")
	if err := fs.Parse(args); err != nil {
		return err
	}

	opts := AggregateOptions{GroupBy: AggregateGroup(strings.ToLower(*group))}
	switch opts.GroupBy {
	case GroupDay, GroupMonth, GroupModel, GroupProvider:
	default:
		return fmt.Errorf("usage report: --group must be one of day,month,model,provider")
	}
	if *from != "" {
		t, err := time.Parse("2006-01-02", *from)
		if err != nil {
			return fmt.Errorf("usage report: --from must be YYYY-MM-DD: %w", err)
		}
		opts.From = t.UTC()
	}
	if *to != "" {
		t, err := time.Parse("2006-01-02", *to)
		if err != nil {
			return fmt.Errorf("usage report: --to must be YYYY-MM-DD: %w", err)
		}
		opts.To = t.UTC()
	}

	buckets, err := Aggregate(dir, opts)
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(buckets)
	}
	return printAggregateTable(stdout, buckets, opts.GroupBy)
}

func printAggregateTable(w io.Writer, buckets []Bucket, group AggregateGroup) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	header := headerFor(group)
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	var totalCalls, totalTokens int
	var totalCost float64
	for _, b := range buckets {
		fmt.Fprintln(tw, strings.Join(rowFor(b, group), "\t"))
		totalCalls += b.Calls
		totalTokens += b.TotalTokens
		totalCost += b.CostUSD
	}
	footer := append([]string{}, footerLabel(group)...)
	footer = append(footer,
		fmt.Sprintf("%d", totalCalls),
		fmt.Sprintf("%d", totalTokens),
		fmt.Sprintf("$%.4f", totalCost),
	)
	fmt.Fprintln(tw, strings.Join(footer, "\t"))
	return tw.Flush()
}

func headerFor(group AggregateGroup) []string {
	switch group {
	case GroupModel:
		return []string{"PROVIDER", "MODEL", "CALLS", "TOKENS", "COST_USD"}
	case GroupProvider:
		return []string{"PROVIDER", "CALLS", "TOKENS", "COST_USD"}
	default: // day, month
		return []string{"PERIOD", "PROVIDER", "MODEL", "CALLS", "TOKENS", "COST_USD"}
	}
}

func footerLabel(group AggregateGroup) []string {
	switch group {
	case GroupModel:
		return []string{"TOTAL", ""}
	case GroupProvider:
		return []string{"TOTAL"}
	default:
		return []string{"TOTAL", "", ""}
	}
}

func rowFor(b Bucket, group AggregateGroup) []string {
	cost := fmt.Sprintf("$%.4f", b.CostUSD)
	calls := fmt.Sprintf("%d", b.Calls)
	tokens := fmt.Sprintf("%d", b.TotalTokens)
	switch group {
	case GroupModel:
		return []string{b.Provider, b.Model, calls, tokens, cost}
	case GroupProvider:
		return []string{b.Provider, calls, tokens, cost}
	default:
		return []string{b.Period, b.Provider, b.Model, calls, tokens, cost}
	}
}
