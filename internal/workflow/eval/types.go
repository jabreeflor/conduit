// Package eval implements Conduit's custom eval suites, result storage, and
// scorecard reporting.
package eval

import "time"

// Suite is the YAML format accepted by `conduit eval run`.
type Suite struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Cases       []Case `yaml:"cases" json:"cases"`
}

// Case is one eval prompt plus the assertions that must hold.
type Case struct {
	Name     string         `yaml:"name" json:"name"`
	Input    string         `yaml:"input" json:"input"`
	Model    string         `yaml:"model,omitempty" json:"model,omitempty"`
	Setup    CaseSetup      `yaml:"setup,omitempty" json:"setup,omitempty"`
	Expect   Expectations   `yaml:"expect" json:"expect"`
	Observed *ObservedTrace `yaml:"observed,omitempty" json:"observed,omitempty"`
}

// CaseSetup injects deterministic harness state before the case runs.
type CaseSetup struct {
	InjectMemory string `yaml:"inject_memory,omitempty" json:"inject_memory,omitempty"`
}

// Expectations lists supported assertions from PRD section 6.23.
type Expectations struct {
	ToolCallsInclude          []string `yaml:"tool_calls_include,omitempty" json:"tool_calls_include,omitempty"`
	ToolCallsExclude          []string `yaml:"tool_calls_exclude,omitempty" json:"tool_calls_exclude,omitempty"`
	ReplyContains             string   `yaml:"reply_contains,omitempty" json:"reply_contains,omitempty"`
	ReplyContainsTag          string   `yaml:"reply_contains_tag,omitempty" json:"reply_contains_tag,omitempty"`
	ReplySentiment            string   `yaml:"reply_sentiment,omitempty" json:"reply_sentiment,omitempty"`
	DurationMaxSeconds        float64  `yaml:"duration_max_seconds,omitempty" json:"duration_max_seconds,omitempty"`
	CostMaxUSD                float64  `yaml:"cost_max_usd,omitempty" json:"cost_max_usd,omitempty"`
	NoPromptInjectionDetected *bool    `yaml:"no_prompt_injection_detected,omitempty" json:"no_prompt_injection_detected,omitempty"`
	WorkflowStepsCompleted    string   `yaml:"workflow_steps_completed,omitempty" json:"workflow_steps_completed,omitempty"`
	ContextRetained           string   `yaml:"context_retained,omitempty" json:"context_retained,omitempty"`
}

// ObservedTrace is the normalized evidence assertions run against.
type ObservedTrace struct {
	Reply                   string   `yaml:"reply,omitempty" json:"reply,omitempty"`
	ToolCalls               []string `yaml:"tool_calls,omitempty" json:"tool_calls,omitempty"`
	DurationSeconds         float64  `yaml:"duration_seconds,omitempty" json:"duration_seconds,omitempty"`
	CostUSD                 float64  `yaml:"cost_usd,omitempty" json:"cost_usd,omitempty"`
	PromptInjectionDetected bool     `yaml:"prompt_injection_detected,omitempty" json:"prompt_injection_detected,omitempty"`
	WorkflowStepsDone       int      `yaml:"workflow_steps_done,omitempty" json:"workflow_steps_done,omitempty"`
	WorkflowStepsTotal      int      `yaml:"workflow_steps_total,omitempty" json:"workflow_steps_total,omitempty"`
}

// CaseResult is one scored eval case stored as JSONL.
type CaseResult struct {
	At         time.Time          `json:"at"`
	RunID      string             `json:"run_id"`
	Suite      string             `json:"suite"`
	Case       string             `json:"case"`
	Model      string             `json:"model"`
	Passed     bool               `json:"passed"`
	Score      float64            `json:"score"`
	Assertions []AssertionResult  `json:"assertions"`
	Observed   ObservedTrace      `json:"observed"`
	Metrics    HarnessMetricEvent `json:"metrics"`
}

// AssertionResult captures the pass/fail reason for one assertion.
type AssertionResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

// HarnessMetricEvent stores the built-in metric signals collected per case.
type HarnessMetricEvent struct {
	InstructionFollowed bool    `json:"instruction_followed"`
	ToolSelectionOK     bool    `json:"tool_selection_ok"`
	HookCompliant       bool    `json:"hook_compliant"`
	ContextRetained     bool    `json:"context_retained"`
	StructuredTagOK     bool    `json:"structured_tag_ok"`
	WorkflowCompleted   bool    `json:"workflow_completed"`
	InjectionResistant  bool    `json:"injection_resistant"`
	CostUSD             float64 `json:"cost_usd"`
	LatencySeconds      float64 `json:"latency_seconds"`
	Escalated           bool    `json:"escalated"`
}

// Summary is the score for a model over a run or report window.
type Summary struct {
	Model          string
	Passed         int
	Total          int
	ScorePercent   float64
	AvgCostUSD     float64
	AvgLatencySecs float64
	Metrics        MetricSummary
}

// MetricSummary is the per-model harness scorecard.
type MetricSummary struct {
	InstructionFollowRate float64
	ToolSelectionAccuracy float64
	HookCompliance        float64
	ContextRetention      float64
	StructuredTagFidelity float64
	WorkflowCompletion    float64
	InjectionResistance   float64
	CostEfficiencyUSD     float64
	LatencySeconds        float64
	EscalationTriggerRate float64
}
