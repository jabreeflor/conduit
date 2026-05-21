package usage

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"os"
	"time"
)

// DashboardOptions controls the dashboard date range. Output is always a
// single self-contained HTML file with no external assets.
type DashboardOptions struct {
	From time.Time
	To   time.Time
}

// Dashboard renders aggregate buckets to a self-contained HTML page on out.
// The page embeds its data as inline JSON and uses no external scripts or
// styles — viewable offline, safe to attach to expense reports.
func Dashboard(dir string, opts DashboardOptions, out io.Writer) error {
	dayBuckets, err := Aggregate(dir, AggregateOptions{From: opts.From, To: opts.To, GroupBy: GroupDay})
	if err != nil {
		return err
	}
	modelBuckets, err := Aggregate(dir, AggregateOptions{From: opts.From, To: opts.To, GroupBy: GroupModel})
	if err != nil {
		return err
	}
	providerBuckets, err := Aggregate(dir, AggregateOptions{From: opts.From, To: opts.To, GroupBy: GroupProvider})
	if err != nil {
		return err
	}

	var totalCost float64
	var totalCalls, totalTokens int
	for _, b := range dayBuckets {
		totalCost += b.CostUSD
		totalCalls += b.Calls
		totalTokens += b.TotalTokens
	}

	dayJSON, _ := json.Marshal(dayBuckets)
	modelJSON, _ := json.Marshal(modelBuckets)
	providerJSON, _ := json.Marshal(providerBuckets)

	data := struct {
		Generated      string
		From, To       string
		TotalCost      string
		TotalCalls     int
		TotalTokens    int
		ByDayJSON      template.JS
		ByModelJSON    template.JS
		ByProviderJSON template.JS
	}{
		Generated:      time.Now().UTC().Format(time.RFC3339),
		From:           rangeLabel(opts.From),
		To:             rangeLabel(opts.To),
		TotalCost:      fmt.Sprintf("$%.4f", totalCost),
		TotalCalls:     totalCalls,
		TotalTokens:    totalTokens,
		ByDayJSON:      template.JS(dayJSON),
		ByModelJSON:    template.JS(modelJSON),
		ByProviderJSON: template.JS(providerJSON),
	}
	return dashboardTemplate.Execute(out, data)
}

func rangeLabel(t time.Time) string {
	if t.IsZero() {
		return "all"
	}
	return t.Format("2006-01-02")
}

func runDashboard(args []string, stdout, stderr io.Writer, dir string) error {
	fs := flag.NewFlagSet("usage dashboard", flag.ContinueOnError)
	fs.SetOutput(stderr)
	from := fs.String("from", "", "include entries on/after this date (YYYY-MM-DD, UTC)")
	to := fs.String("to", "", "include entries strictly before this date (YYYY-MM-DD, UTC)")
	outPath := fs.String("out", "", "write to this HTML file (default: stdout)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	opts := DashboardOptions{}
	if *from != "" {
		t, err := time.Parse("2006-01-02", *from)
		if err != nil {
			return fmt.Errorf("usage dashboard: --from must be YYYY-MM-DD: %w", err)
		}
		opts.From = t.UTC()
	}
	if *to != "" {
		t, err := time.Parse("2006-01-02", *to)
		if err != nil {
			return fmt.Errorf("usage dashboard: --to must be YYYY-MM-DD: %w", err)
		}
		opts.To = t.UTC()
	}

	out := stdout
	if *outPath != "" {
		f, err := os.Create(*outPath)
		if err != nil {
			return fmt.Errorf("usage dashboard: create output: %w", err)
		}
		defer f.Close()
		out = f
	}
	if err := Dashboard(dir, opts, out); err != nil {
		return err
	}
	if *outPath != "" {
		fmt.Fprintf(stdout, "wrote dashboard to %s\n", *outPath)
	}
	return nil
}

var dashboardTemplate = template.Must(template.New("dashboard").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Conduit usage report</title>
<style>
  :root { color-scheme: light dark; }
  body { font: 14px/1.4 system-ui, sans-serif; margin: 2rem; max-width: 960px; }
  h1 { margin-bottom: .25rem; }
  .meta { color: #666; margin-bottom: 1.5rem; font-size: .9em; }
  .cards { display: grid; grid-template-columns: repeat(3, 1fr); gap: 1rem; margin-bottom: 2rem; }
  .card { padding: 1rem; border: 1px solid #8884; border-radius: 8px; }
  .card h2 { margin: 0 0 .25rem; font-size: .85em; color: #888; font-weight: 500; text-transform: uppercase; letter-spacing: .04em; }
  .card .value { font-size: 1.6em; font-weight: 600; }
  table { width: 100%; border-collapse: collapse; margin-bottom: 2rem; }
  th, td { text-align: left; padding: .35rem .5rem; border-bottom: 1px solid #8884; }
  th { font-size: .8em; text-transform: uppercase; letter-spacing: .04em; color: #888; font-weight: 500; }
  td.num { text-align: right; font-variant-numeric: tabular-nums; }
  section { margin-bottom: 2rem; }
  h2.section { font-size: 1em; text-transform: uppercase; letter-spacing: .04em; color: #888; margin-bottom: .5rem; }
  footer { color: #888; font-size: .8em; border-top: 1px solid #8884; padding-top: 1rem; }
</style>
</head>
<body>
<h1>Conduit usage</h1>
<div class="meta">Generated {{.Generated}} · range {{.From}} → {{.To}} · all data local, no telemetry</div>

<div class="cards">
  <div class="card"><h2>Total cost</h2><div class="value">{{.TotalCost}}</div></div>
  <div class="card"><h2>Calls</h2><div class="value">{{.TotalCalls}}</div></div>
  <div class="card"><h2>Tokens</h2><div class="value">{{.TotalTokens}}</div></div>
</div>

<section><h2 class="section">By provider</h2><table id="byProvider"><thead><tr><th>Provider</th><th class="num">Calls</th><th class="num">Tokens</th><th class="num">Cost (USD)</th></tr></thead><tbody></tbody></table></section>
<section><h2 class="section">By model</h2><table id="byModel"><thead><tr><th>Provider</th><th>Model</th><th class="num">Calls</th><th class="num">Tokens</th><th class="num">Cost (USD)</th></tr></thead><tbody></tbody></table></section>
<section><h2 class="section">By day</h2><table id="byDay"><thead><tr><th>Date</th><th>Provider</th><th>Model</th><th class="num">Calls</th><th class="num">Tokens</th><th class="num">Cost (USD)</th></tr></thead><tbody></tbody></table></section>

<footer>conduit usage dashboard · stored at ~/.conduit/logs/usage · run <code>conduit usage purge --before YYYY-MM-DD --yes</code> to delete history</footer>

<script>
const data = {
  byDay: {{.ByDayJSON}},
  byModel: {{.ByModelJSON}},
  byProvider: {{.ByProviderJSON}},
};
const usd = (n) => "$" + (n || 0).toFixed(4);
const num = (n) => (n || 0).toLocaleString();
function row(cells) {
  const tr = document.createElement("tr");
  cells.forEach(([text, isNum]) => {
    const td = document.createElement("td");
    td.textContent = text;
    if (isNum) td.className = "num";
    tr.appendChild(td);
  });
  return tr;
}
function fill(id, rows) {
  const tbody = document.querySelector("#" + id + " tbody");
  rows.forEach(r => tbody.appendChild(row(r)));
}
fill("byProvider", data.byProvider.map(b => [
  [b.provider || "(unknown)", false], [num(b.calls), true], [num(b.total_tokens), true], [usd(b.cost_usd), true],
]));
fill("byModel", data.byModel.map(b => [
  [b.provider || "", false], [b.model || "", false], [num(b.calls), true], [num(b.total_tokens), true], [usd(b.cost_usd), true],
]));
fill("byDay", data.byDay.map(b => [
  [b.period || "", false], [b.provider || "", false], [b.model || "", false], [num(b.calls), true], [num(b.total_tokens), true], [usd(b.cost_usd), true],
]));
</script>
</body>
</html>
`))
