// Package workflow implements durable, resumable multi-step workflow runs.
//
// The package owns the minimal state machine required by PRD §6.7
// (workflow checkpointing & resume) and the associated provider failover
// chain. Conditional branching (issue #34) is implemented in
// condition.go; additional features (scheduling, etc.) extend these
// types in their own files.
package workflow

import "time"

// RunState is the lifecycle state of a single Run.
type RunState string

const (
	// RunStatePending means the Run has been created but no Step has executed.
	RunStatePending RunState = "pending"
	// RunStateRunning means at least one Step has executed and the Run is
	// neither completed nor failed.
	RunStateRunning RunState = "running"
	// RunStateCompleted means every Step finished successfully.
	RunStateCompleted RunState = "completed"
	// RunStateFailed means a Step exhausted its provider chain and the Run
	// cannot make further progress without intervention.
	RunStateFailed RunState = "failed"
)

// Step is one unit of work within a Workflow.
//
// The struct unifies the checkpointing fields needed by issue #33 with
// the conditional-branching fields contributed by issue #34. Additional
// features (scheduling, retries) should extend this struct rather than
// duplicate it.
type Step struct {
	// ID is a stable identifier for the Step within its Workflow.
	// IDs must be unique inside a single Workflow.
	ID string `json:"id" yaml:"id"`
	// Name is a human-readable label shown in surfaces and logs.
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// Prompt is the input handed to the StepExecutor when the Step runs.
	Prompt string `json:"prompt,omitempty" yaml:"prompt,omitempty"`

	// Next is the unconditional successor step ID. Used when Condition is
	// nil, or as a fallback when Condition's outcomes are empty. The
	// linear engine in this PR ignores Next; conditional branching uses
	// it (see condition.go).
	Next string `json:"next,omitempty" yaml:"next,omitempty"`
	// Condition, if non-nil, is evaluated against the previous step's
	// StepResult to decide which branch to take. See condition.go.
	Condition *Condition `json:"condition,omitempty" yaml:"condition,omitempty"`
	// OnTrue is the step ID taken when Condition evaluates to true.
	// Empty string means "stop the workflow".
	OnTrue string `json:"on_true,omitempty" yaml:"on_true,omitempty"`
	// OnFalse is the step ID taken when Condition evaluates to false.
	// Empty string means "stop the workflow".
	OnFalse string `json:"on_false,omitempty" yaml:"on_false,omitempty"`
}

// Workflow is the static definition of a multi-step task.
type Workflow struct {
	// ID identifies the Workflow definition. Two Runs of the same
	// definition share this ID.
	ID string `json:"id" yaml:"id"`
	// Name is a human-readable label.
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// Steps are executed in order. Empty Steps is a valid (no-op) workflow.
	Steps []Step `json:"steps" yaml:"steps"`
	// Providers is the ordered failover chain for model invocations.
	// When a Step's StepExecutor returns an error, the Engine advances
	// to the next provider and retries the Step before failing the Run.
	// An empty chain means a single attempt with the executor's default
	// provider (empty string).
	Providers []string `json:"providers,omitempty" yaml:"providers,omitempty"`
}

// StepResult is the persisted outcome of executing a Step.
type StepResult struct {
	// StepID matches Step.ID.
	StepID string `json:"step_id" yaml:"step_id"`
	// Provider is the entry from Workflow.Providers that produced Output,
	// or empty if no provider chain was configured.
	Provider string `json:"provider,omitempty" yaml:"provider,omitempty"`
	// Output is the structured value emitted by the step. May be a
	// string, number, bool, map[string]any, []any, or nil. The
	// branching evaluator (see condition.go) reads this value via
	// JSONPath; the StepExecutor used by this PR returns plain strings.
	Output any `json:"output,omitempty" yaml:"output,omitempty"`
	// Error is set when the Step ultimately failed after exhausting the
	// provider chain. A successful Step has Error == "".
	Error string `json:"error,omitempty" yaml:"error,omitempty"`
	// StartedAt and CompletedAt bracket the Step's wall-clock execution.
	StartedAt   time.Time `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
}

// Run is the durable state for one execution of a Workflow.
//
// The struct is JSON-encoded by the Checkpointer after every Step. Any
// new field added here must also survive a write/read roundtrip through
// encoding/json.
type Run struct {
	// ID uniquely identifies this Run across the host. Used as the
	// checkpoint filename.
	ID string `json:"id"`
	// WorkflowID matches Workflow.ID.
	WorkflowID string `json:"workflow_id"`
	// Workflow is the embedded definition so a Run can be resumed
	// without consulting an external definition store.
	Workflow Workflow `json:"workflow"`
	// State is the lifecycle position of the Run.
	State RunState `json:"state"`
	// CurrentStep is the index into Workflow.Steps of the next Step to
	// execute. After a successful Step at index i, CurrentStep is i+1.
	// When CurrentStep == len(Workflow.Steps), the Run is complete.
	CurrentStep int `json:"current_step"`
	// Results is the per-Step outcome history, one entry per executed
	// Step in execution order.
	Results []StepResult `json:"results"`
	// CreatedAt is when the Run was first checkpointed.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is the timestamp of the most recent checkpoint.
	UpdatedAt time.Time `json:"updated_at"`
	// LastError captures the failure reason when State == RunStateFailed.
	LastError string `json:"last_error,omitempty"`
}
