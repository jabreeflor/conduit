package eval

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreAppendReadAllAndSummarize(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	results := []CaseResult{
		{
			At:     now,
			RunID:  "run-1",
			Suite:  "suite",
			Case:   "a",
			Model:  "model-a",
			Passed: true,
			Observed: ObservedTrace{
				CostUSD:         0.10,
				DurationSeconds: 2,
			},
			Metrics: HarnessMetricEvent{
				InstructionFollowed: true,
				ToolSelectionOK:     true,
				HookCompliant:       true,
				ContextRetained:     true,
				StructuredTagOK:     true,
				WorkflowCompleted:   true,
				InjectionResistant:  true,
				CostUSD:             0.10,
				LatencySeconds:      2,
			},
		},
		{
			At:     now,
			RunID:  "run-1",
			Suite:  "suite",
			Case:   "b",
			Model:  "model-a",
			Passed: false,
			Observed: ObservedTrace{
				CostUSD:         0.30,
				DurationSeconds: 4,
			},
			Metrics: HarnessMetricEvent{
				HookCompliant:      true,
				InjectionResistant: true,
				CostUSD:            0.30,
				LatencySeconds:     4,
			},
		},
	}

	path, err := store.Append(results)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if filepath.Base(path) != "run-1.jsonl" {
		t.Errorf("path = %s", path)
	}

	read, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(read) != 2 {
		t.Fatalf("len(read) = %d, want 2", len(read))
	}

	summaries := Summarize(read)
	if len(summaries) != 1 {
		t.Fatalf("len(summaries) = %d, want 1", len(summaries))
	}
	s := summaries[0]
	if s.Passed != 1 || s.Total != 2 || s.ScorePercent != 50 {
		t.Errorf("summary score = %d/%d %.0f", s.Passed, s.Total, s.ScorePercent)
	}
	if s.AvgCostUSD != 0.20 {
		t.Errorf("AvgCostUSD = %f, want 0.20", s.AvgCostUSD)
	}
	if s.Metrics.HookCompliance != 100 {
		t.Errorf("HookCompliance = %f, want 100", s.Metrics.HookCompliance)
	}
}

func TestFilterResults(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	results := []CaseResult{
		{At: now.Add(-time.Hour), Model: "a"},
		{At: now.Add(-48 * time.Hour), Model: "a"},
		{At: now.Add(-time.Hour), Model: "b"},
	}

	filtered := FilterResults(results, "a", now.Add(-24*time.Hour))
	if len(filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1", len(filtered))
	}
	if filtered[0].Model != "a" {
		t.Errorf("Model = %q", filtered[0].Model)
	}
}
