package eval

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Harness runs one case against a model and returns normalized evidence.
type Harness interface {
	RunCase(context.Context, Case, string) (ObservedTrace, error)
}

// DeterministicHarness gives the CLI useful local behavior before live model
// adapters are attached. Suites can provide an observed block to replay traces.
type DeterministicHarness struct{}

// RunCase returns the case's observed trace, or a deterministic text-only trace.
func (DeterministicHarness) RunCase(_ context.Context, c Case, _ string) (ObservedTrace, error) {
	if c.Observed != nil {
		return *c.Observed, nil
	}
	reply := c.Input
	if c.Setup.InjectMemory != "" {
		reply += "\n" + c.Setup.InjectMemory
	}
	return ObservedTrace{Reply: reply}, nil
}

// Runner scores suites and writes no global state itself.
type Runner struct {
	Harness Harness
	Now     func() time.Time
}

// Run executes all cases in all suites. modelOverride applies to every case
// when non-empty.
func (r Runner) Run(ctx context.Context, suites []Suite, modelOverride string) ([]CaseResult, error) {
	harness := r.Harness
	if harness == nil {
		harness = DeterministicHarness{}
	}
	now := r.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	runID := fmt.Sprintf("eval-%d", now().UnixNano())
	var results []CaseResult
	for _, suite := range suites {
		for _, c := range suite.Cases {
			model := modelOverride
			if model == "" {
				model = c.Model
			}
			if model == "" {
				model = "default"
			}

			observed, err := harness.RunCase(ctx, c, model)
			if err != nil {
				return nil, fmt.Errorf("eval: %s/%s on %s: %w", suite.Name, c.Name, model, err)
			}
			assertions := Evaluate(c.Expect, observed)
			passed := true
			for _, a := range assertions {
				passed = passed && a.Passed
			}
			score := 0.0
			if len(assertions) > 0 {
				var ok int
				for _, a := range assertions {
					if a.Passed {
						ok++
					}
				}
				score = float64(ok) / float64(len(assertions))
			}
			results = append(results, CaseResult{
				At:         now(),
				RunID:      runID,
				Suite:      suite.Name,
				Case:       c.Name,
				Model:      model,
				Passed:     passed,
				Score:      score,
				Assertions: assertions,
				Observed:   observed,
				Metrics:    deriveMetrics(c.Expect, assertions, observed),
			})
		}
	}
	return results, nil
}

// Evaluate applies every supported assertion to an observed trace.
func Evaluate(expect Expectations, observed ObservedTrace) []AssertionResult {
	var out []AssertionResult
	for _, tool := range expect.ToolCallsInclude {
		out = append(out, assertion("tool_calls_include", contains(observed.ToolCalls, tool), "%s not called", tool))
	}
	for _, tool := range expect.ToolCallsExclude {
		out = append(out, assertion("tool_calls_exclude", !contains(observed.ToolCalls, tool), "%s called", tool))
	}
	if expect.ReplyContains != "" {
		out = append(out, assertion("reply_contains", strings.Contains(observed.Reply, expect.ReplyContains), "%q missing", expect.ReplyContains))
	}
	if expect.ReplyContainsTag != "" {
		out = append(out, assertion("reply_contains_tag", strings.Contains(observed.Reply, expect.ReplyContainsTag), "%q missing", expect.ReplyContainsTag))
	}
	if expect.ReplySentiment != "" {
		got := classifySentiment(observed.Reply)
		out = append(out, assertion("reply_sentiment", got == expect.ReplySentiment, "got %s", got))
	}
	if expect.DurationMaxSeconds > 0 {
		out = append(out, assertion("duration_max_seconds", observed.DurationSeconds <= expect.DurationMaxSeconds, "%.3fs > %.3fs", observed.DurationSeconds, expect.DurationMaxSeconds))
	}
	if expect.CostMaxUSD > 0 {
		out = append(out, assertion("cost_max_usd", observed.CostUSD <= expect.CostMaxUSD, "$%.6f > $%.6f", observed.CostUSD, expect.CostMaxUSD))
	}
	if expect.NoPromptInjectionDetected != nil {
		want := !*expect.NoPromptInjectionDetected
		out = append(out, assertion("no_prompt_injection_detected", observed.PromptInjectionDetected == want, "prompt injection detected=%t", observed.PromptInjectionDetected))
	}
	if expect.WorkflowStepsCompleted != "" {
		wantDone, wantTotal, ok := parseSteps(expect.WorkflowStepsCompleted)
		pass := ok && observed.WorkflowStepsDone >= wantDone
		if wantTotal > 0 {
			pass = pass && observed.WorkflowStepsTotal == wantTotal
		}
		out = append(out, assertion("workflow_steps_completed", pass, "got %d/%d", observed.WorkflowStepsDone, observed.WorkflowStepsTotal))
	}
	if expect.ContextRetained != "" {
		out = append(out, assertion("context_retained", strings.Contains(observed.Reply, expect.ContextRetained), "%q missing", expect.ContextRetained))
	}
	return out
}

func assertion(name string, passed bool, format string, args ...any) AssertionResult {
	if passed {
		return AssertionResult{Name: name, Passed: true}
	}
	return AssertionResult{Name: name, Passed: false, Message: fmt.Sprintf(format, args...)}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func classifySentiment(reply string) string {
	text := strings.ToLower(reply)
	if strings.Contains(text, "can't") || strings.Contains(text, "cannot") || strings.Contains(text, "won't") || strings.Contains(text, "refuse") || strings.Contains(text, "sorry") {
		return "refusal"
	}
	if strings.Contains(text, "?") {
		return "question"
	}
	return "affirmative"
}

func parseSteps(s string) (int, int, bool) {
	done, total, ok := strings.Cut(strings.TrimSpace(s), "/")
	if !ok {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		return n, 0, err == nil
	}
	d, errDone := strconv.Atoi(strings.TrimSpace(done))
	t, errTotal := strconv.Atoi(strings.TrimSpace(total))
	return d, t, errDone == nil && errTotal == nil
}

func deriveMetrics(expect Expectations, assertions []AssertionResult, observed ObservedTrace) HarnessMetricEvent {
	passed := map[string]bool{}
	for _, a := range assertions {
		passed[a.Name] = a.Passed
	}
	toolOK := true
	if len(expect.ToolCallsInclude) > 0 {
		toolOK = toolOK && allAssertionsPassed(assertions, "tool_calls_include")
	}
	if len(expect.ToolCallsExclude) > 0 {
		toolOK = toolOK && allAssertionsPassed(assertions, "tool_calls_exclude")
	}
	return HarnessMetricEvent{
		InstructionFollowed: assertionsPassed(assertions),
		ToolSelectionOK:     toolOK,
		HookCompliant:       true,
		ContextRetained:     expect.ContextRetained == "" || passed["context_retained"],
		StructuredTagOK:     expect.ReplyContainsTag == "" || passed["reply_contains_tag"],
		WorkflowCompleted:   expect.WorkflowStepsCompleted == "" || passed["workflow_steps_completed"],
		InjectionResistant:  expect.NoPromptInjectionDetected == nil || passed["no_prompt_injection_detected"],
		CostUSD:             observed.CostUSD,
		LatencySeconds:      observed.DurationSeconds,
		Escalated:           false,
	}
}

func assertionsPassed(assertions []AssertionResult) bool {
	for _, a := range assertions {
		if !a.Passed {
			return false
		}
	}
	return true
}

func allAssertionsPassed(assertions []AssertionResult, name string) bool {
	found := false
	for _, a := range assertions {
		if a.Name != name {
			continue
		}
		found = true
		if !a.Passed {
			return false
		}
	}
	return found
}
