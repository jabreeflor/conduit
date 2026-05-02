package contracts

// WorkflowActionKind discriminates the action variant carried by a
// WorkflowStep. Exactly one matching action field on WorkflowStep must be
// non-nil; the kind reflects which one.
type WorkflowActionKind string

const (
	// WorkflowActionTool runs a registered tool with a structured argument map.
	WorkflowActionTool WorkflowActionKind = "tool"
	// WorkflowActionModel invokes a model provider directly with a prompt.
	WorkflowActionModel WorkflowActionKind = "model"
	// WorkflowActionSubagent spawns a profile-bound subagent (issue #54).
	WorkflowActionSubagent WorkflowActionKind = "subagent"
	// WorkflowActionBranch dispatches to one of N step lists by condition.
	WorkflowActionBranch WorkflowActionKind = "branch"
)

// WorkflowStepStatus is the lifecycle state of a single step inside a
// running workflow. The execution engine (issue #33) writes these values;
// the parser/validator only references them as the legal token set for the
// `when` field.
type WorkflowStepStatus string

const (
	// WorkflowStepStatusPending means the step has not yet executed.
	WorkflowStepStatusPending WorkflowStepStatus = "pending"
	// WorkflowStepStatusRunning means the step is currently executing.
	WorkflowStepStatusRunning WorkflowStepStatus = "running"
	// WorkflowStepStatusSucceeded means the step completed without error.
	WorkflowStepStatusSucceeded WorkflowStepStatus = "succeeded"
	// WorkflowStepStatusFailed means the step terminated with an error.
	WorkflowStepStatusFailed WorkflowStepStatus = "failed"
	// WorkflowStepStatusSkipped means the step's `if` condition evaluated
	// to false and the engine bypassed execution.
	WorkflowStepStatusSkipped WorkflowStepStatus = "skipped"
)

// WorkflowDefinition is the static, parsed shape of a YAML workflow file.
//
// It is the schema parsed by internal/workflow.Parse; a separate execution
// engine consumes it. The runtime types Run / RunState / StepResult live in
// internal/workflow and are intentionally distinct: the definition is the
// authored contract, the run is its dynamic execution.
type WorkflowDefinition struct {
	// ID identifies the workflow definition. Required; must be non-empty.
	ID string `yaml:"id" json:"id"`
	// Name is a human-readable label.
	Name string `yaml:"name,omitempty" json:"name,omitempty"`
	// Description is a free-form summary surfaced in tooling.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Version is the authored schema/workflow revision. Required.
	Version string `yaml:"version" json:"version"`
	// Schedule is an optional 5- or 6-field cron expression. When present
	// the scheduler wakes the workflow at each fire time.
	Schedule string `yaml:"schedule,omitempty" json:"schedule,omitempty"`
	// Inputs declares typed parameters supplied at run time. Keys are
	// parameter names; values document the parameter (free-form, the parser
	// does not impose a JSON Schema yet).
	Inputs map[string]WorkflowInput `yaml:"inputs,omitempty" json:"inputs,omitempty"`
	// Env is an environment variable seed merged into every action.
	Env map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	// Steps are the ordered top-level work units. At least one is required.
	Steps []WorkflowStep `yaml:"steps" json:"steps"`
	// Outputs maps named workflow outputs to template references resolved
	// against completed steps. Optional.
	Outputs map[string]string `yaml:"outputs,omitempty" json:"outputs,omitempty"`
}

// WorkflowInput documents a single named workflow parameter.
type WorkflowInput struct {
	// Type is the declared parameter type (e.g. "string", "int", "bool",
	// "object"). The parser accepts any non-empty value; downstream
	// type-checking is the engine's responsibility.
	Type string `yaml:"type,omitempty" json:"type,omitempty"`
	// Description is a free-form explanation surfaced in tooling.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Required marks the input as mandatory at run time.
	Required bool `yaml:"required,omitempty" json:"required,omitempty"`
	// Default is the value used when the caller omits the input.
	Default any `yaml:"default,omitempty" json:"default,omitempty"`
}

// WorkflowStep is a single authored action inside a WorkflowDefinition.
//
// Exactly one of Tool, Model, Subagent, or Branch must be set; the validator
// rejects steps with zero or multiple actions.
type WorkflowStep struct {
	// ID is the step's stable identifier within its lexical scope.
	// Top-level step IDs must be unique across Steps; branch-case step IDs
	// must be unique within their case.
	ID string `yaml:"id" json:"id"`
	// Name is a human-readable label.
	Name string `yaml:"name,omitempty" json:"name,omitempty"`
	// If is a condition expression evaluated before the step runs. When
	// present and false, the step is skipped.
	If string `yaml:"if,omitempty" json:"if,omitempty"`
	// When is a guard against the immediately preceding step's status. The
	// parser accepts any non-empty string; the engine compares it against
	// WorkflowStepStatus values.
	When string `yaml:"when,omitempty" json:"when,omitempty"`

	// Tool is the tool-invocation action variant.
	Tool *WorkflowToolAction `yaml:"tool,omitempty" json:"tool,omitempty"`
	// Model is the direct model-invocation action variant.
	Model *WorkflowModelAction `yaml:"model,omitempty" json:"model,omitempty"`
	// Subagent is the subagent-spawn action variant (issue #54).
	Subagent *WorkflowSubagentAction `yaml:"subagent,omitempty" json:"subagent,omitempty"`
	// Branch is the conditional dispatch action variant.
	Branch *WorkflowBranchAction `yaml:"branch,omitempty" json:"branch,omitempty"`
}

// WorkflowToolAction invokes a tool by name with a structured argument map.
type WorkflowToolAction struct {
	// Name identifies the tool. Required, non-empty.
	Name string `yaml:"name" json:"name"`
	// With is the tool's argument map. Values may be any YAML scalar,
	// sequence, or mapping; template references inside string leaves are
	// resolved at execution time.
	With map[string]any `yaml:"with,omitempty" json:"with,omitempty"`
}

// WorkflowModelAction calls a model provider with a prompt.
type WorkflowModelAction struct {
	// Provider identifies the routing target (e.g. "anthropic", "local").
	// Required, non-empty.
	Provider string `yaml:"provider" json:"provider"`
	// Model is the model identifier within the provider. Required.
	Model string `yaml:"model" json:"model"`
	// Prompt is the user-side message. Required.
	Prompt string `yaml:"prompt" json:"prompt"`
	// System is the optional system message.
	System string `yaml:"system,omitempty" json:"system,omitempty"`
	// Tags annotate the request for special routing (destructive, etc.).
	Tags []TaskTag `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// WorkflowSubagentAction spawns a profile-bound subagent.
type WorkflowSubagentAction struct {
	// Profile names a registered subagent profile. Required.
	Profile string `yaml:"profile" json:"profile"`
	// Prompt is the subagent's input. Required.
	Prompt string `yaml:"prompt" json:"prompt"`
	// Inputs is an optional structured argument map.
	Inputs map[string]any `yaml:"inputs,omitempty" json:"inputs,omitempty"`
}

// WorkflowBranchAction dispatches to one of several step lists.
//
// At validation time, the first case whose `when` string is non-empty is the
// candidate; runtime selection is the engine's responsibility.
type WorkflowBranchAction struct {
	// Cases are the conditional branches in declaration order. At least
	// one case is required.
	Cases []WorkflowBranchCase `yaml:"cases" json:"cases"`
	// Default is the fallback step list when no case matches.
	Default []WorkflowStep `yaml:"default,omitempty" json:"default,omitempty"`
}

// WorkflowBranchCase is one arm of a WorkflowBranchAction.
type WorkflowBranchCase struct {
	// When is the condition expression. Required, non-empty.
	When string `yaml:"when" json:"when"`
	// Steps are the actions executed if When matches. Each case has its
	// own ID scope.
	Steps []WorkflowStep `yaml:"steps" json:"steps"`
}

// Action returns the kind of action carried by s and the count of action
// variants set. A correctly authored step has exactly one variant; the
// validator uses the count to surface "no action" / "multiple actions".
func (s WorkflowStep) Action() (WorkflowActionKind, int) {
	count := 0
	kind := WorkflowActionKind("")
	if s.Tool != nil {
		count++
		kind = WorkflowActionTool
	}
	if s.Model != nil {
		count++
		kind = WorkflowActionModel
	}
	if s.Subagent != nil {
		count++
		kind = WorkflowActionSubagent
	}
	if s.Branch != nil {
		count++
		kind = WorkflowActionBranch
	}
	return kind, count
}
