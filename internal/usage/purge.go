package usage

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// PurgeOptions selects which usage entries to delete. At least one of Before
// or Model must be set — unscoped purges are intentionally unsupported.
type PurgeOptions struct {
	Before time.Time // delete entries strictly before this UTC instant
	Model  string    // case-insensitive exact match on entry.Model
	DryRun bool      // count what would be removed but write nothing
}

// PurgeReport summarizes a purge across all log files.
type PurgeReport struct {
	Files   []RewriteResult
	Kept    int
	Dropped int
}

// Purge applies opts across every log file in dir. Returns the rollup report.
func Purge(dir string, opts PurgeOptions) (PurgeReport, error) {
	if opts.Before.IsZero() && opts.Model == "" {
		return PurgeReport{}, fmt.Errorf("usage purge: --before or --model is required")
	}

	logs, err := ListLogs(dir)
	if err != nil {
		return PurgeReport{}, err
	}

	keep := matcher(opts)
	var report PurgeReport
	for _, log := range logs {
		if opts.DryRun {
			var kept, dropped int
			if err := ScanLog(log, func(e contracts.UsageEntry) bool {
				if keep(e) {
					kept++
				} else {
					dropped++
				}
				return true
			}); err != nil {
				return report, err
			}
			report.Files = append(report.Files, RewriteResult{Path: log.Path, Kept: kept, Dropped: dropped})
			report.Kept += kept
			report.Dropped += dropped
			continue
		}

		res, err := RewriteLog(log, keep)
		if err != nil {
			return report, err
		}
		report.Files = append(report.Files, res)
		report.Kept += res.Kept
		report.Dropped += res.Dropped
	}
	return report, nil
}

// matcher returns true for entries that should be kept (i.e. NOT purged).
func matcher(opts PurgeOptions) func(contracts.UsageEntry) bool {
	model := strings.ToLower(strings.TrimSpace(opts.Model))
	return func(e contracts.UsageEntry) bool {
		if !opts.Before.IsZero() && e.Timestamp.Before(opts.Before) {
			return false
		}
		if model != "" && strings.EqualFold(e.Model, model) {
			return false
		}
		return true
	}
}

func runPurge(args []string, stdout, stderr io.Writer, dir string) error {
	fs := flag.NewFlagSet("usage purge", flag.ContinueOnError)
	fs.SetOutput(stderr)
	beforeStr := fs.String("before", "", "delete entries before this date (YYYY-MM-DD, UTC)")
	model := fs.String("model", "", "delete entries matching this model exactly")
	dryRun := fs.Bool("dry-run", false, "report what would be deleted without changing files")
	yes := fs.Bool("yes", false, "skip confirmation prompt (required for non-dry-run)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	opts := PurgeOptions{Model: *model, DryRun: *dryRun}
	if *beforeStr != "" {
		t, err := time.Parse("2006-01-02", *beforeStr)
		if err != nil {
			return fmt.Errorf("usage purge: --before must be YYYY-MM-DD: %w", err)
		}
		opts.Before = t.UTC()
	}
	if opts.Before.IsZero() && opts.Model == "" {
		return fmt.Errorf("usage purge: at least one of --before or --model is required")
	}
	if !opts.DryRun && !*yes {
		return fmt.Errorf("usage purge: refusing destructive purge without --yes (run with --dry-run to preview)")
	}

	report, err := Purge(dir, opts)
	if err != nil {
		return err
	}
	printPurgeReport(stdout, report, opts)
	return nil
}

func printPurgeReport(w io.Writer, report PurgeReport, opts PurgeOptions) {
	verb := "removed"
	if opts.DryRun {
		verb = "would remove"
	}
	fmt.Fprintf(w, "%s %d entr%s across %d file(s); kept %d.\n",
		verb, report.Dropped, plural(report.Dropped), len(report.Files), report.Kept)
	for _, f := range report.Files {
		if f.Dropped == 0 && !f.Removed {
			continue
		}
		marker := ""
		if f.Removed {
			marker = " (file deleted)"
		}
		fmt.Fprintf(w, "  %s: -%d / kept %d%s\n", f.Path, f.Dropped, f.Kept, marker)
	}
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
