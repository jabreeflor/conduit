package eval

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// RunCLI is the entry point for `conduit eval`.
func RunCLI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return usage(stdout)
	}
	switch args[0] {
	case "run":
		return runEval(ctx, args[1:], stdout)
	case "compare":
		return compareEval(ctx, args[1:], stdout)
	case "report":
		return reportEval(args[1:], stdout)
	case "replay":
		return replayEval(args[1:], stdout)
	default:
		return fmt.Errorf("unknown eval subcommand %q; try: run, compare, report, replay", args[0])
	}
}

func usage(stdout io.Writer) error {
	_, err := fmt.Fprintln(stdout, "usage: conduit eval run|compare|report|replay")
	return err
}

func runEval(ctx context.Context, args []string, stdout io.Writer) error {
	path, model, resultsDir, err := parseRunArgs(args)
	if err != nil {
		return err
	}
	suites, err := LoadSuites(path)
	if err != nil {
		return err
	}
	results, err := Runner{}.Run(ctx, suites, model)
	if err != nil {
		return err
	}
	store, err := NewStore(resultsDir)
	if err != nil {
		return err
	}
	resultsPath, err := store.Append(results)
	if err != nil {
		return err
	}
	printRun(stdout, results)
	fmt.Fprintf(stdout, "Results: %s\n", resultsPath)
	return nil
}

func compareEval(ctx context.Context, args []string, stdout io.Writer) error {
	models, suitePath, resultsDir, err := parseCompareArgs(args)
	if err != nil {
		return err
	}
	suites, err := LoadSuites(suitePath)
	if err != nil {
		return err
	}
	var all []CaseResult
	for _, model := range models {
		results, err := Runner{}.Run(ctx, suites, model)
		if err != nil {
			return err
		}
		all = append(all, results...)
	}
	store, err := NewStore(resultsDir)
	if err != nil {
		return err
	}
	path, err := store.Append(all)
	if err != nil {
		return err
	}
	printComparison(stdout, all)
	fmt.Fprintf(stdout, "Results: %s\n", path)
	return nil
}

func reportEval(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("eval report", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	model := fs.String("model", "", "model filter")
	last := fs.String("last", "", "window such as 30d, 12h, 90m")
	resultsDir := fs.String("results-dir", "", "results directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	since, err := parseLast(*last)
	if err != nil {
		return err
	}
	store, err := NewStore(*resultsDir)
	if err != nil {
		return err
	}
	results, err := store.ReadAll()
	if err != nil {
		return err
	}
	results = FilterResults(results, *model, since)
	printReport(stdout, Summarize(results))
	return nil
}

func replayEval(args []string, stdout io.Writer) error {
	sessionID, model, diff, err := parseReplayArgs(args)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Replay queued for session %s on %s\n", sessionID, model)
	if diff {
		fmt.Fprintln(stdout, "Diff output will be available once session transcript storage is connected.")
	}
	return nil
}

func parseRunArgs(args []string) (path, model, resultsDir string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--model":
			i++
			if i >= len(args) {
				return "", "", "", errors.New("eval run: --model requires a value")
			}
			model = args[i]
		case "--results-dir":
			i++
			if i >= len(args) {
				return "", "", "", errors.New("eval run: --results-dir requires a value")
			}
			resultsDir = args[i]
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", "", "", fmt.Errorf("eval run: unknown flag %s", args[i])
			}
			if path != "" {
				return "", "", "", errors.New("usage: conduit eval run <suite-file-or-dir> [--model model]")
			}
			path = args[i]
		}
	}
	if path == "" {
		return "", "", "", errors.New("usage: conduit eval run <suite-file-or-dir> [--model model]")
	}
	return path, model, resultsDir, nil
}

func parseCompareArgs(args []string) (models []string, suitePath, resultsDir string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--suite":
			i++
			if i >= len(args) {
				return nil, "", "", errors.New("eval compare: --suite requires a value")
			}
			suitePath = args[i]
		case "--results-dir":
			i++
			if i >= len(args) {
				return nil, "", "", errors.New("eval compare: --results-dir requires a value")
			}
			resultsDir = args[i]
		default:
			if strings.HasPrefix(args[i], "-") {
				return nil, "", "", fmt.Errorf("eval compare: unknown flag %s", args[i])
			}
			models = append(models, args[i])
		}
	}
	if len(models) < 2 || suitePath == "" {
		return nil, "", "", errors.New("usage: conduit eval compare <model-a> <model-b> [model-c...] --suite <suite-file-or-dir>")
	}
	return models, suitePath, resultsDir, nil
}

func parseReplayArgs(args []string) (sessionID, model string, diff bool, err error) {
	model = "default"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--model":
			i++
			if i >= len(args) {
				return "", "", false, errors.New("eval replay: --model requires a value")
			}
			model = args[i]
		case "--diff":
			diff = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", "", false, fmt.Errorf("eval replay: unknown flag %s", args[i])
			}
			if sessionID != "" {
				return "", "", false, errors.New("usage: conduit eval replay <session-id> --model <model> [--diff]")
			}
			sessionID = args[i]
		}
	}
	if sessionID == "" {
		return "", "", false, errors.New("usage: conduit eval replay <session-id> --model <model> [--diff]")
	}
	return sessionID, model, diff, nil
}

func printRun(w io.Writer, results []CaseResult) {
	summaries := Summarize(results)
	for _, summary := range summaries {
		fmt.Fprintf(w, "Model: %s  Score: %d/%d (%.0f%%)  Avg cost: $%.4f  Avg latency: %.2fs\n",
			summary.Model, summary.Passed, summary.Total, summary.ScorePercent, summary.AvgCostUSD, summary.AvgLatencySecs)
	}
	for _, result := range results {
		status := "PASS"
		if !result.Passed {
			status = "FAIL"
		}
		fmt.Fprintf(w, "%s/%s [%s] %s\n", result.Suite, result.Case, result.Model, status)
		for _, assertion := range result.Assertions {
			if !assertion.Passed {
				fmt.Fprintf(w, "  - %s: %s\n", assertion.Name, assertion.Message)
			}
		}
	}
}

func printComparison(w io.Writer, results []CaseResult) {
	byCase := map[string]map[string]CaseResult{}
	var models []string
	seenModels := map[string]bool{}
	for _, result := range results {
		key := result.Suite + "/" + result.Case
		if byCase[key] == nil {
			byCase[key] = map[string]CaseResult{}
		}
		byCase[key][result.Model] = result
		if !seenModels[result.Model] {
			seenModels[result.Model] = true
			models = append(models, result.Model)
		}
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprint(tw, "CASE")
	for _, model := range models {
		fmt.Fprintf(tw, "\t%s", model)
	}
	fmt.Fprintln(tw)
	for key, byModel := range byCase {
		fmt.Fprint(tw, key)
		for _, model := range models {
			status := "MISS"
			if result, ok := byModel[model]; ok {
				status = "PASS"
				if !result.Passed {
					status = "FAIL"
				}
			}
			fmt.Fprintf(tw, "\t%s", status)
		}
		fmt.Fprintln(tw)
	}
	for _, summary := range Summarize(results) {
		fmt.Fprintf(tw, "Score %s\t%d/%d (%.0f%%)\n", summary.Model, summary.Passed, summary.Total, summary.ScorePercent)
	}
	_ = tw.Flush()
}

func printReport(w io.Writer, summaries []Summary) {
	if len(summaries) == 0 {
		fmt.Fprintln(w, "No eval results found.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "MODEL\tSCORE\tINSTR\tTOOLS\tHOOKS\tCONTEXT\tTAGS\tWORKFLOW\tINJECTION\tCOST\tLATENCY\tESCALATION")
	for _, s := range summaries {
		fmt.Fprintf(tw, "%s\t%d/%d %.0f%%\t%.0f%%\t%.0f%%\t%.0f%%\t%.0f%%\t%.0f%%\t%.0f%%\t%.0f%%\t$%.4f\t%.2fs\t%.0f%%\n",
			s.Model,
			s.Passed,
			s.Total,
			s.ScorePercent,
			s.Metrics.InstructionFollowRate,
			s.Metrics.ToolSelectionAccuracy,
			s.Metrics.HookCompliance,
			s.Metrics.ContextRetention,
			s.Metrics.StructuredTagFidelity,
			s.Metrics.WorkflowCompletion,
			s.Metrics.InjectionResistance,
			s.Metrics.CostEfficiencyUSD,
			s.Metrics.LatencySeconds,
			s.Metrics.EscalationTriggerRate,
		)
	}
	_ = tw.Flush()
}

func parseLast(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	unit := value[len(value)-1]
	n := strings.TrimSpace(value[:len(value)-1])
	count, err := strconv.Atoi(n)
	if err != nil || count <= 0 {
		return time.Time{}, fmt.Errorf("eval report: invalid --last %q", value)
	}
	var d time.Duration
	switch unit {
	case 'd':
		d = 24 * time.Hour
	case 'h':
		d = time.Hour
	case 'm':
		d = time.Minute
	default:
		return time.Time{}, fmt.Errorf("eval report: unsupported --last %q; use 30d, 12h, or 90m", value)
	}
	return time.Now().UTC().Add(-time.Duration(count) * d), nil
}
