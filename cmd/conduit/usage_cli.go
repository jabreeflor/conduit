package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
	"github.com/jabreeflor/conduit/internal/usage"
)

func runUsageCLI(_ context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return usageHelp(stderr)
	}
	logPath, err := usage.DefaultLogPath()
	if err != nil {
		return err
	}

	switch args[0] {
	case "purge":
		return runUsagePurge(args[1:], logPath, stdout, stderr)
	case "export":
		return runUsageExport(args[1:], logPath, stdout, stderr)
	case "report":
		return runUsageReport(args[1:], logPath, stdout, stderr)
	case "dashboard":
		return runUsageDashboard(args[1:], logPath, stdout, stderr)
	default:
		return fmt.Errorf("unknown usage command %q", args[0])
	}
}

func runUsagePurge(args []string, defaultLogPath string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("conduit usage purge", flag.ContinueOnError)
	fs.SetOutput(stderr)
	beforeRaw := fs.String("before", "", "delete usage entries before YYYY-MM-DD")
	model := fs.String("model", "", "delete usage entries for a model")
	logPath := fs.String("log", defaultLogPath, "usage log path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var before *time.Time
	if *beforeRaw != "" {
		t, err := parseUsageDate(*beforeRaw)
		if err != nil {
			return err
		}
		before = &t
	}
	removed, err := usage.Purge(*logPath, usage.PurgeOptions{Before: before, Model: *model})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Purged %d usage entr", removed)
	if removed == 1 {
		fmt.Fprintln(stdout, "y.")
	} else {
		fmt.Fprintln(stdout, "ies.")
	}
	return nil
}

func runUsageExport(args []string, defaultLogPath string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("conduit usage export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "json", "json or csv")
	outPath := fs.String("out", "", "output file path")
	logPath := fs.String("log", defaultLogPath, "usage log path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	entries, err := usage.ReadLog(*logPath)
	if err != nil {
		return err
	}
	return writeUsageOutput(*outPath, stdout, func(w io.Writer) error {
		return usage.ExportRaw(entries, usage.ExportFormat(*format), w)
	})
}

func runUsageReport(args []string, defaultLogPath string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("conduit usage report", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fromRaw := fs.String("from", "", "include entries on/after YYYY-MM-DD")
	toRaw := fs.String("to", "", "include entries before YYYY-MM-DD")
	outPath := fs.String("out", "", "output file path")
	logPath := fs.String("log", defaultLogPath, "usage log path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	entries, from, to, err := usageReportInputs(*logPath, *fromRaw, *toRaw)
	if err != nil {
		return err
	}
	report := usage.BuildAggregateReport(entries, from, to)
	return writeUsageOutput(*outPath, stdout, func(w io.Writer) error {
		return usage.WriteAggregateReport(report, w)
	})
}

func runUsageDashboard(args []string, defaultLogPath string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("conduit usage dashboard", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fromRaw := fs.String("from", "", "include entries on/after YYYY-MM-DD")
	toRaw := fs.String("to", "", "include entries before YYYY-MM-DD")
	outPath := fs.String("out", "", "output HTML file path")
	logPath := fs.String("log", defaultLogPath, "usage log path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	entries, from, to, err := usageReportInputs(*logPath, *fromRaw, *toRaw)
	if err != nil {
		return err
	}
	report := usage.BuildAggregateReport(entries, from, to)
	return writeUsageOutput(*outPath, stdout, func(w io.Writer) error {
		return usage.WriteDashboardHTML(report, w)
	})
}

func usageReportInputs(logPath, fromRaw, toRaw string) ([]contracts.UsageEntry, *time.Time, *time.Time, error) {
	entries, err := usage.ReadLog(logPath)
	if err != nil {
		return nil, nil, nil, err
	}
	var from, to *time.Time
	if fromRaw != "" {
		t, err := parseUsageDate(fromRaw)
		if err != nil {
			return nil, nil, nil, err
		}
		from = &t
	}
	if toRaw != "" {
		t, err := parseUsageDate(toRaw)
		if err != nil {
			return nil, nil, nil, err
		}
		to = &t
	}
	return entries, from, to, nil
}

func parseUsageDate(raw string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("usage: parse date %q as YYYY-MM-DD: %w", raw, err)
	}
	return t.UTC(), nil
}

func writeUsageOutput(outPath string, stdout io.Writer, write func(io.Writer) error) error {
	if outPath == "" {
		return write(stdout)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("usage: create output: %w", err)
	}
	defer f.Close()
	return write(f)
}

func usageHelp(w io.Writer) error {
	_, err := fmt.Fprintln(w, "usage: conduit usage purge|export|report|dashboard")
	return err
}
