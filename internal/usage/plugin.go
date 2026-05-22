package usage

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// PluginQuery filters the per-plugin usage rollup. Both bounds are optional;
// when zero, they mean "open" on that side. PluginID empty means "all
// plugins" — the typical surface filter is the dashboard's all-plugins view.
type PluginQuery struct {
	PluginID string
	From     time.Time
	To       time.Time
}

// PluginUsage is the per-(plugin, model) summary returned by Query. Model is
// empty for the "any model" rollup row that Query also emits when results
// span more than one model — see Query's comment for the layout.
type PluginUsage struct {
	PluginID         string  `json:"plugin_id"`
	Model            string  `json:"model,omitempty"`
	Provider         string  `json:"provider,omitempty"`
	Requests         int     `json:"requests"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	Errors           int     `json:"errors"`
	ErrorRate        float64 `json:"error_rate"`
	AverageLatencyMS float64 `json:"average_latency_ms"`
}

// AnomalyKind labels what was unusual about a window.
type AnomalyKind string

const (
	// AnomalyRequestsSpike fires when a plugin's request count in the most
	// recent window exceeds rolling-window mean + N*stddev.
	AnomalyRequestsSpike AnomalyKind = "requests_spike"
	// AnomalyCostSpike is the cost-USD analogue.
	AnomalyCostSpike AnomalyKind = "cost_spike"
	// AnomalyErrorRateSpike fires when error rate exceeds 2x rolling mean
	// AND is at least 5%.
	AnomalyErrorRateSpike AnomalyKind = "error_rate_spike"
)

// Anomaly is one detection result.
type Anomaly struct {
	PluginID  string      `json:"plugin_id"`
	Kind      AnomalyKind `json:"kind"`
	Window    string      `json:"window"`
	Observed  float64     `json:"observed"`
	BaseMean  float64     `json:"baseline_mean"`
	BaseStd   float64     `json:"baseline_stddev"`
	Threshold float64     `json:"threshold"`
}

// Query scans the usage log directory and returns per-(plugin, model) rows
// matching opts. Entries with an empty Plugin field are skipped — the goal
// here is plugin attribution, not session totals.
//
// Result layout: when more than one model is present for a single plugin, an
// extra row with Model="" is appended carrying the plugin-wide totals so
// dashboards can show "all models" without double-counting.
func Query(dir string, opts PluginQuery) ([]PluginUsage, error) {
	type key struct{ plugin, model, provider string }
	rows := map[key]*PluginUsage{}
	totalLatency := map[key]int64{}
	exportRange := ExportOptions{From: opts.From, To: opts.To}

	err := ScanAll(dir, func(e contracts.UsageEntry) bool {
		if e.Plugin == "" {
			return true
		}
		if opts.PluginID != "" && e.Plugin != opts.PluginID {
			return true
		}
		if !inRange(e.Timestamp, exportRange) {
			return true
		}
		k := key{plugin: e.Plugin, model: e.Model, provider: e.Provider}
		row, ok := rows[k]
		if !ok {
			row = &PluginUsage{PluginID: e.Plugin, Model: e.Model, Provider: e.Provider}
			rows[k] = row
		}
		row.Requests++
		row.InputTokens += e.TokensIn + e.InputTokens
		row.OutputTokens += e.TokensOut + e.OutputTokens
		row.TotalTokens += e.TotalTokens
		row.CostUSD += e.CostUSD
		if isErrorStatus(e.Status) {
			row.Errors++
		}
		totalLatency[k] += e.TotalMS
		return true
	})
	if err != nil {
		return nil, err
	}

	out := make([]PluginUsage, 0, len(rows))
	pluginRows := map[string][]*PluginUsage{}
	for k, row := range rows {
		if row.Requests > 0 {
			row.AverageLatencyMS = float64(totalLatency[k]) / float64(row.Requests)
			row.ErrorRate = float64(row.Errors) / float64(row.Requests)
		}
		out = append(out, *row)
		pluginRows[row.PluginID] = append(pluginRows[row.PluginID], row)
	}

	// Append rollup rows for plugins with > 1 model.
	for plugin, list := range pluginRows {
		if len(list) <= 1 {
			continue
		}
		roll := PluginUsage{PluginID: plugin}
		var totalMS int64
		for _, r := range list {
			roll.Requests += r.Requests
			roll.InputTokens += r.InputTokens
			roll.OutputTokens += r.OutputTokens
			roll.TotalTokens += r.TotalTokens
			roll.CostUSD += r.CostUSD
			roll.Errors += r.Errors
			totalMS += int64(r.AverageLatencyMS * float64(r.Requests))
		}
		if roll.Requests > 0 {
			roll.AverageLatencyMS = float64(totalMS) / float64(roll.Requests)
			roll.ErrorRate = float64(roll.Errors) / float64(roll.Requests)
		}
		out = append(out, roll)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].PluginID != out[j].PluginID {
			return out[i].PluginID < out[j].PluginID
		}
		// Empty model (rollup) sorts last per plugin.
		if (out[i].Model == "") != (out[j].Model == "") {
			return out[i].Model != ""
		}
		return out[i].Model < out[j].Model
	})
	return out, nil
}

// AnomalyOptions controls the rolling-window detector. Window is the bucket
// size, Lookback the number of historical buckets used to compute the
// baseline mean+stddev. Sigma is the multiplier (default 2.0).
type AnomalyOptions struct {
	PluginID string
	Window   time.Duration
	Lookback int
	Sigma    float64
	Now      time.Time
}

// Detect returns anomalies in the most recent window relative to the
// preceding Lookback buckets. The function is intentionally simple: a single
// rolling window per plugin per metric. Sophisticated detectors are the job
// of an outer dashboard, not the storage layer.
func Detect(dir string, opts AnomalyOptions) ([]Anomaly, error) {
	if opts.Window <= 0 {
		opts.Window = 24 * time.Hour
	}
	if opts.Lookback <= 0 {
		opts.Lookback = 7
	}
	if opts.Sigma <= 0 {
		opts.Sigma = 2.0
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	from := now.Add(-time.Duration(opts.Lookback+1) * opts.Window)

	// per plugin -> per bucket index (0 = oldest, last = most recent)
	histories := map[string][]anomalyBucket{}
	err := ScanAll(dir, func(e contracts.UsageEntry) bool {
		if e.Plugin == "" {
			return true
		}
		if opts.PluginID != "" && e.Plugin != opts.PluginID {
			return true
		}
		if e.Timestamp.Before(from) || e.Timestamp.After(now) {
			return true
		}
		idx := int(e.Timestamp.Sub(from) / opts.Window)
		if idx < 0 {
			idx = 0
		}
		if idx > opts.Lookback {
			idx = opts.Lookback
		}
		buckets, ok := histories[e.Plugin]
		if !ok {
			buckets = make([]anomalyBucket, opts.Lookback+1)
			histories[e.Plugin] = buckets
		}
		buckets[idx].Requests++
		buckets[idx].Cost += e.CostUSD
		if isErrorStatus(e.Status) {
			buckets[idx].Errors++
		}
		return true
	})
	if err != nil {
		return nil, err
	}

	var out []Anomaly
	windowLabel := fmt.Sprintf("last %s", opts.Window)
	for plugin, hist := range histories {
		current := hist[len(hist)-1]
		baseline := hist[:len(hist)-1]
		reqMean, reqStd := meanStd(intsRequests(baseline))
		costMean, costStd := meanStd(floatsCost(baseline))
		errMean, _ := meanStd(errorRates(baseline))

		if reqStd > 0 && float64(current.Requests) > reqMean+opts.Sigma*reqStd {
			out = append(out, Anomaly{
				PluginID: plugin, Kind: AnomalyRequestsSpike, Window: windowLabel,
				Observed: float64(current.Requests), BaseMean: reqMean, BaseStd: reqStd,
				Threshold: reqMean + opts.Sigma*reqStd,
			})
		}
		if costStd > 0 && current.Cost > costMean+opts.Sigma*costStd {
			out = append(out, Anomaly{
				PluginID: plugin, Kind: AnomalyCostSpike, Window: windowLabel,
				Observed: current.Cost, BaseMean: costMean, BaseStd: costStd,
				Threshold: costMean + opts.Sigma*costStd,
			})
		}
		curRate := 0.0
		if current.Requests > 0 {
			curRate = float64(current.Errors) / float64(current.Requests)
		}
		if curRate >= 0.05 && curRate > 2*errMean {
			out = append(out, Anomaly{
				PluginID: plugin, Kind: AnomalyErrorRateSpike, Window: windowLabel,
				Observed: curRate, BaseMean: errMean, Threshold: 2 * errMean,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PluginID != out[j].PluginID {
			return out[i].PluginID < out[j].PluginID
		}
		return out[i].Kind < out[j].Kind
	})
	return out, nil
}

type anomalyBucket struct {
	Requests int
	Cost     float64
	Errors   int
}

func intsRequests(b []anomalyBucket) []float64 {
	out := make([]float64, len(b))
	for i, v := range b {
		out[i] = float64(v.Requests)
	}
	return out
}

func floatsCost(b []anomalyBucket) []float64 {
	out := make([]float64, len(b))
	for i, v := range b {
		out[i] = v.Cost
	}
	return out
}

func errorRates(b []anomalyBucket) []float64 {
	out := make([]float64, len(b))
	for i, v := range b {
		if v.Requests > 0 {
			out[i] = float64(v.Errors) / float64(v.Requests)
		}
	}
	return out
}

func meanStd(xs []float64) (float64, float64) {
	if len(xs) == 0 {
		return 0, 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	mean := sum / float64(len(xs))
	var ss float64
	for _, x := range xs {
		ss += (x - mean) * (x - mean)
	}
	return mean, math.Sqrt(ss / float64(len(xs)))
}

func isErrorStatus(s string) bool {
	if s == "" {
		return false
	}
	switch strings.ToLower(s) {
	case "ok", "success":
		return false
	}
	return true
}

// runPluginsCLI handles `conduit usage plugins [--plugin <id>] [--from ...] [--to ...] [--json]`
// and `conduit usage plugins anomalies [...]`.
func runPluginsCLI(args []string, stdout, stderr io.Writer, dir string) error {
	if len(args) > 0 && args[0] == "anomalies" {
		return runAnomaliesCLI(args[1:], stdout, stderr, dir)
	}
	fs := flag.NewFlagSet("usage plugins", flag.ContinueOnError)
	fs.SetOutput(stderr)
	plugin := fs.String("plugin", "", "filter to a single plugin id")
	from := fs.String("from", "", "include entries on/after this date (YYYY-MM-DD, UTC)")
	to := fs.String("to", "", "include entries strictly before this date (YYYY-MM-DD, UTC)")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	q := PluginQuery{PluginID: *plugin}
	if *from != "" {
		t, err := time.Parse("2006-01-02", *from)
		if err != nil {
			return fmt.Errorf("usage plugins: --from must be YYYY-MM-DD: %w", err)
		}
		q.From = t.UTC()
	}
	if *to != "" {
		t, err := time.Parse("2006-01-02", *to)
		if err != nil {
			return fmt.Errorf("usage plugins: --to must be YYYY-MM-DD: %w", err)
		}
		q.To = t.UTC()
	}
	rows, err := Query(dir, q)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(stdout).Encode(rows)
	}
	for _, r := range rows {
		model := r.Model
		if model == "" {
			model = "*"
		}
		fmt.Fprintf(stdout, "%s\t%s\treq=%d\ttok=%d\terr=%.1f%%\tcost=$%.4f\n",
			r.PluginID, model, r.Requests, r.TotalTokens, r.ErrorRate*100, r.CostUSD)
	}
	return nil
}

func runAnomaliesCLI(args []string, stdout, stderr io.Writer, dir string) error {
	fs := flag.NewFlagSet("usage plugins anomalies", flag.ContinueOnError)
	fs.SetOutput(stderr)
	plugin := fs.String("plugin", "", "filter to a single plugin id")
	windowH := fs.Int("window-hours", 24, "rolling window size in hours")
	lookback := fs.Int("lookback", 7, "number of historical windows for baseline")
	sigma := fs.Float64("sigma", 2.0, "stddev multiplier triggering an anomaly")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	out, err := Detect(dir, AnomalyOptions{
		PluginID: *plugin,
		Window:   time.Duration(*windowH) * time.Hour,
		Lookback: *lookback,
		Sigma:    *sigma,
	})
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(stdout).Encode(out)
	}
	if len(out) == 0 {
		fmt.Fprintln(stdout, "no anomalies")
		return nil
	}
	for _, a := range out {
		fmt.Fprintf(stdout, "%s\t%s\tobserved=%.2f baseline=%.2f±%.2f threshold=%.2f\n",
			a.PluginID, a.Kind, a.Observed, a.BaseMean, a.BaseStd, a.Threshold)
	}
	return nil
}
