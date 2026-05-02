// Package workflow implements the conduit workflow engine described in
// PRD §6.7. This file defines the minimal step/result types needed by the
// branching evaluator. Issue #33 owns the canonical foundational types
// (Workflow, Step, Run, Engine); when those land, the fields below should
// be reconciled with whatever #33 ships and the duplicate definitions
// here removed.
package workflow

// Step is the unit of work in a workflow. Only the fields needed for
// conditional branching (PRD §6.7) are defined here; #33 will extend it
// with execution metadata (kind, inputs, retries, etc.).
type Step struct {
	// ID uniquely identifies the step within its workflow.
	ID string `json:"id" yaml:"id"`

	// Next is the unconditional successor step ID. Used when Condition is
	// nil, or as a fallback when Condition's outcomes are empty.
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

// StepResult captures the output of a step execution. The branching
// evaluator only reads Output and Error; #33 will extend with timings,
// stdout/stderr, exit codes, etc.
type StepResult struct {
	// StepID is the ID of the step that produced this result.
	StepID string `json:"step_id" yaml:"step_id"`

	// Output is the structured value emitted by the step. May be a
	// string, number, bool, map[string]any, []any, or nil.
	Output any `json:"output,omitempty" yaml:"output,omitempty"`

	// Error is the error string if the step failed, empty otherwise.
	Error string `json:"error,omitempty" yaml:"error,omitempty"`
}
