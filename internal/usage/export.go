package usage

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// ExportFormat selects the on-disk shape of an export.
type ExportFormat string

const (
	ExportFormatCSV  ExportFormat = "csv"
	ExportFormatJSON ExportFormat = "json"
)

// ExportOptions controls the date-range filter applied during export.
// Out is the destination writer; Format selects encoding.
type ExportOptions struct {
	From   time.Time
	To     time.Time
	Format ExportFormat
}

// csvHeader is the column order for CSV exports — stable for downstream tooling.
var csvHeader = []string{
	"timestamp", "session_id", "provider", "model",
	"tokens_in", "tokens_out", "total_tokens",
	"ttft_ms", "total_ms", "tokens_per_sec",
	"status", "error_type", "feature", "plugin",
	"cost_usd", "cost_currency", "cost_estimated", "cost_source",
}

// Export streams entries from dir matching opts to out.
func Export(dir string, opts ExportOptions, out io.Writer) error {
	switch opts.Format {
	case ExportFormatCSV:
		return exportCSV(dir, opts, out)
	case ExportFormatJSON:
		return exportJSON(dir, opts, out)
	default:
		return fmt.Errorf("usage export: unknown format %q (want csv or json)", opts.Format)
	}
}

func exportCSV(dir string, opts ExportOptions, out io.Writer) error {
	w := csv.NewWriter(out)
	if err := w.Write(csvHeader); err != nil {
		return err
	}
	err := ScanAll(dir, func(e contracts.UsageEntry) bool {
		if !inRange(e.Timestamp, opts) {
			return true
		}
		_ = w.Write(csvRow(e))
		return true
	})
	if err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func csvRow(e contracts.UsageEntry) []string {
	return []string{
		e.Timestamp.UTC().Format(time.RFC3339),
		e.SessionID,
		e.Provider,
		e.Model,
		strconv.Itoa(e.TokensIn),
		strconv.Itoa(e.TokensOut),
		strconv.Itoa(e.TotalTokens),
		strconv.FormatInt(e.TTFMS, 10),
		strconv.FormatInt(e.TotalMS, 10),
		strconv.FormatFloat(e.TokensPerSecond, 'f', 3, 64),
		e.Status,
		e.ErrorType,
		e.Feature,
		e.Plugin,
		strconv.FormatFloat(e.CostUSD, 'f', 6, 64),
		e.CostCurrency,
		strconv.FormatBool(e.CostEstimated),
		e.CostSource,
	}
}

func exportJSON(dir string, opts ExportOptions, out io.Writer) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if _, err := io.WriteString(out, "[\n"); err != nil {
		return err
	}
	first := true
	err := ScanAll(dir, func(e contracts.UsageEntry) bool {
		if !inRange(e.Timestamp, opts) {
			return true
		}
		if !first {
			_, _ = io.WriteString(out, ",\n")
		}
		first = false
		// Inline encode without trailing newline so commas land cleanly.
		buf, mErr := json.MarshalIndent(e, "  ", "  ")
		if mErr != nil {
			return true
		}
		_, _ = io.WriteString(out, "  ")
		_, _ = out.Write(buf)
		return true
	})
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, "\n]\n")
	_ = enc // reserved for future streaming variants
	return err
}

func inRange(ts time.Time, opts ExportOptions) bool {
	if !opts.From.IsZero() && ts.Before(opts.From) {
		return false
	}
	if !opts.To.IsZero() && !ts.Before(opts.To) {
		return false
	}
	return true
}

func runExport(args []string, stdout, stderr io.Writer, dir string) error {
	fs := flag.NewFlagSet("usage export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "csv", "output format: csv or json")
	from := fs.String("from", "", "include entries on/after this date (YYYY-MM-DD, UTC)")
	to := fs.String("to", "", "include entries strictly before this date (YYYY-MM-DD, UTC)")
	outPath := fs.String("out", "", "write to this file (default: stdout)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	opts := ExportOptions{Format: ExportFormat(*format)}
	if *from != "" {
		t, err := time.Parse("2006-01-02", *from)
		if err != nil {
			return fmt.Errorf("usage export: --from must be YYYY-MM-DD: %w", err)
		}
		opts.From = t.UTC()
	}
	if *to != "" {
		t, err := time.Parse("2006-01-02", *to)
		if err != nil {
			return fmt.Errorf("usage export: --to must be YYYY-MM-DD: %w", err)
		}
		opts.To = t.UTC()
	}

	out := stdout
	if *outPath != "" {
		f, err := os.Create(*outPath)
		if err != nil {
			return fmt.Errorf("usage export: create output: %w", err)
		}
		defer f.Close()
		out = f
	}
	return Export(dir, opts, out)
}
