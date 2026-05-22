package eval

import (
	"context"
	"testing"
	"time"
)

func TestEvaluateSupportedAssertions(t *testing.T) {
	noInjection := true
	assertions := Evaluate(Expectations{
		ToolCallsInclude:          []string{"memory.read"},
		ToolCallsExclude:          []string{"file.delete"},
		ReplyContains:             "invoicing",
		ReplyContainsTag:          "[[canvas:html]]",
		ReplySentiment:            "affirmative",
		DurationMaxSeconds:        3,
		CostMaxUSD:                0.2,
		NoPromptInjectionDetected: &noInjection,
		WorkflowStepsCompleted:    "2/2",
		ContextRetained:           "Last Tuesday",
	}, ObservedTrace{
		Reply:              "Last Tuesday invoicing [[canvas:html]]",
		ToolCalls:          []string{"memory.read"},
		DurationSeconds:    1.5,
		CostUSD:            0.1,
		WorkflowStepsDone:  2,
		WorkflowStepsTotal: 2,
	})

	if len(assertions) != 10 {
		t.Fatalf("len(assertions) = %d, want 10", len(assertions))
	}
	for _, assertion := range assertions {
		if !assertion.Passed {
			t.Errorf("%s failed: %s", assertion.Name, assertion.Message)
		}
	}
}

func TestEvaluateReportsFailures(t *testing.T) {
	assertions := Evaluate(Expectations{
		ToolCallsInclude: []string{"memory.read"},
		ReplyContains:    "invoicing",
		CostMaxUSD:       0.01,
	}, ObservedTrace{
		Reply:   "hello",
		CostUSD: 0.02,
	})

	var failed int
	for _, assertion := range assertions {
		if !assertion.Passed {
			failed++
		}
	}
	if failed != 3 {
		t.Errorf("failed = %d, want 3", failed)
	}
}

func TestRunnerUsesModelOverrideAndObservedTrace(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	suites := []Suite{{
		Name: "suite",
		Cases: []Case{{
			Name:  "case",
			Input: "hello",
			Model: "case-model",
			Expect: Expectations{
				ReplyContains: "world",
			},
			Observed: &ObservedTrace{Reply: "hello world"},
		}},
	}}

	results, err := Runner{Now: func() time.Time { return now }}.Run(context.Background(), suites, "override-model")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if !results[0].Passed {
		t.Fatal("result should pass")
	}
	if results[0].Model != "override-model" {
		t.Errorf("Model = %q", results[0].Model)
	}
	if results[0].RunID == "" {
		t.Error("RunID should be populated")
	}
}
