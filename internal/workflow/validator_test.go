package workflow

import (
	"errors"
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// minimalDef returns a small but valid WorkflowDefinition the rule-specific
// tests can mutate without re-declaring every required field.
func minimalDef() contracts.WorkflowDefinition {
	return contracts.WorkflowDefinition{
		ID:      "wf",
		Version: "1.0",
		Steps: []contracts.WorkflowStep{
			{
				ID:   "first",
				Tool: &contracts.WorkflowToolAction{Name: "shell"},
			},
		},
	}
}

// TestValidate_HappyMinimal anchors the negative tests: the baseline must be
// accepted so any failure below points squarely at the mutation under test.
func TestValidate_HappyMinimal(t *testing.T) {
	t.Parallel()
	if err := Validate(minimalDef()); err != nil {
		t.Fatalf("Validate(minimalDef): %v", err)
	}
}

// TestValidate_Rules walks each rule with a single mutation per case and
// asserts the resulting message identifies the offending field.
func TestValidate_Rules(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mutate  func(*contracts.WorkflowDefinition)
		wantSub []string
	}{
		{
			name:    "missing id",
			mutate:  func(d *contracts.WorkflowDefinition) { d.ID = "" },
			wantSub: []string{"id"},
		},
		{
			name:    "missing version",
			mutate:  func(d *contracts.WorkflowDefinition) { d.Version = "" },
			wantSub: []string{"version"},
		},
		{
			name:    "no steps",
			mutate:  func(d *contracts.WorkflowDefinition) { d.Steps = nil },
			wantSub: []string{"steps"},
		},
		{
			name: "duplicate step ids",
			mutate: func(d *contracts.WorkflowDefinition) {
				d.Steps = append(d.Steps, contracts.WorkflowStep{
					ID:   "first",
					Tool: &contracts.WorkflowToolAction{Name: "shell"},
				})
			},
			wantSub: []string{"duplicate step id"},
		},
		{
			name: "step with no action",
			mutate: func(d *contracts.WorkflowDefinition) {
				d.Steps = []contracts.WorkflowStep{{ID: "first"}}
			},
			wantSub: []string{"exactly one of tool, model, subagent, or branch"},
		},
		{
			name: "step with two actions",
			mutate: func(d *contracts.WorkflowDefinition) {
				d.Steps[0].Model = &contracts.WorkflowModelAction{
					Provider: "anthropic", Model: "x", Prompt: "y",
				}
			},
			wantSub: []string{"declares 2 action variants"},
		},
		{
			name:    "empty step id",
			mutate:  func(d *contracts.WorkflowDefinition) { d.Steps[0].ID = "" },
			wantSub: []string{"id", "must not be empty"},
		},
		{
			name:    "reserved step id",
			mutate:  func(d *contracts.WorkflowDefinition) { d.Steps[0].ID = "inputs" },
			wantSub: []string{"reserved"},
		},
		{
			name:    "invalid step id charset",
			mutate:  func(d *contracts.WorkflowDefinition) { d.Steps[0].ID = "has space" },
			wantSub: []string{"not a valid identifier"},
		},
		{
			name:    "tool name empty",
			mutate:  func(d *contracts.WorkflowDefinition) { d.Steps[0].Tool.Name = "" },
			wantSub: []string{"tool.name"},
		},
		{
			name: "model action missing provider",
			mutate: func(d *contracts.WorkflowDefinition) {
				d.Steps[0].Tool = nil
				d.Steps[0].Model = &contracts.WorkflowModelAction{Model: "m", Prompt: "p"}
			},
			wantSub: []string{"model.provider"},
		},
		{
			name: "model action missing model",
			mutate: func(d *contracts.WorkflowDefinition) {
				d.Steps[0].Tool = nil
				d.Steps[0].Model = &contracts.WorkflowModelAction{Provider: "a", Prompt: "p"}
			},
			wantSub: []string{"model.model"},
		},
		{
			name: "model action missing prompt",
			mutate: func(d *contracts.WorkflowDefinition) {
				d.Steps[0].Tool = nil
				d.Steps[0].Model = &contracts.WorkflowModelAction{Provider: "a", Model: "m"}
			},
			wantSub: []string{"model.prompt"},
		},
		{
			name: "subagent missing profile",
			mutate: func(d *contracts.WorkflowDefinition) {
				d.Steps[0].Tool = nil
				d.Steps[0].Subagent = &contracts.WorkflowSubagentAction{Prompt: "p"}
			},
			wantSub: []string{"subagent.profile"},
		},
		{
			name: "subagent missing prompt",
			mutate: func(d *contracts.WorkflowDefinition) {
				d.Steps[0].Tool = nil
				d.Steps[0].Subagent = &contracts.WorkflowSubagentAction{Profile: "p"}
			},
			wantSub: []string{"subagent.prompt"},
		},
		{
			name: "branch with no cases",
			mutate: func(d *contracts.WorkflowDefinition) {
				d.Steps[0].Tool = nil
				d.Steps[0].Branch = &contracts.WorkflowBranchAction{}
			},
			wantSub: []string{"branch.cases"},
		},
		{
			name: "branch case missing when",
			mutate: func(d *contracts.WorkflowDefinition) {
				d.Steps[0].Tool = nil
				d.Steps[0].Branch = &contracts.WorkflowBranchAction{
					Cases: []contracts.WorkflowBranchCase{{
						Steps: []contracts.WorkflowStep{{
							ID:   "leaf",
							Tool: &contracts.WorkflowToolAction{Name: "shell"},
						}},
					}},
				}
			},
			wantSub: []string{"cases[0].when"},
		},
		{
			name: "schedule wrong field count",
			mutate: func(d *contracts.WorkflowDefinition) {
				d.Schedule = "0 9 * *"
			},
			wantSub: []string{"schedule", "5- or 6-field"},
		},
		{
			name: "schedule illegal char",
			mutate: func(d *contracts.WorkflowDefinition) {
				d.Schedule = "0 9 * * MON"
			},
			wantSub: []string{"schedule", "characters outside"},
		},
		{
			name: "schedule pure separator",
			mutate: func(d *contracts.WorkflowDefinition) {
				d.Schedule = "0 9 - , * *"
			},
			wantSub: []string{"numeric or wildcard"},
		},
		{
			name: "template references unknown step",
			mutate: func(d *contracts.WorkflowDefinition) {
				d.Steps[0].Tool.With = map[string]any{"x": "{{ step.ghost.output }}"}
			},
			wantSub: []string{"unknown step id", "ghost"},
		},
		{
			name: "template references unknown input",
			mutate: func(d *contracts.WorkflowDefinition) {
				d.Steps[0].Tool.With = map[string]any{"x": "{{ inputs.ghost }}"}
			},
			wantSub: []string{"unknown input", "ghost"},
		},
		{
			name: "template wrong shape",
			mutate: func(d *contracts.WorkflowDefinition) {
				d.Steps[0].Tool.With = map[string]any{"x": "{{ step.first }}"}
			},
			wantSub: []string{"step.<id>.output"},
		},
		{
			name: "template unknown root",
			mutate: func(d *contracts.WorkflowDefinition) {
				d.Steps[0].Tool.With = map[string]any{"x": "{{ env.HOME }}"}
			},
			wantSub: []string{"step.<id>.output or inputs.<name>"},
		},
		{
			name: "outputs reference unknown step",
			mutate: func(d *contracts.WorkflowDefinition) {
				d.Outputs = map[string]string{"r": "{{ step.ghost.output }}"}
			},
			wantSub: []string{"outputs.r", "unknown step id"},
		},
		{
			name: "if must not be whitespace",
			mutate: func(d *contracts.WorkflowDefinition) {
				d.Steps[0].If = "   "
			},
			wantSub: []string{".if"},
		},
		{
			name: "when must not be whitespace",
			mutate: func(d *contracts.WorkflowDefinition) {
				d.Steps[0].When = "   "
			},
			wantSub: []string{".when"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			def := minimalDef()
			tc.mutate(&def)
			err := Validate(def)
			if err == nil {
				t.Fatalf("expected validation error")
			}
			if !errors.Is(err, ErrValidation) {
				t.Errorf("errors.Is(err, ErrValidation) = false; want true")
			}
			msg := err.Error()
			for _, sub := range tc.wantSub {
				if !strings.Contains(msg, sub) {
					t.Errorf("err = %q\nwant substring %q", msg, sub)
				}
			}
		})
	}
}

// TestValidate_AggregatesAllErrors verifies the validator collects every
// problem in one pass instead of stopping at the first.
func TestValidate_AggregatesAllErrors(t *testing.T) {
	t.Parallel()
	def := contracts.WorkflowDefinition{
		// Missing ID, missing Version, no steps — three independent problems.
	}
	err := Validate(def)
	if err == nil {
		t.Fatal("expected error")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("err type = %T, want *ValidationError", err)
	}
	if len(ve.Errors()) < 3 {
		t.Fatalf("len(Errors()) = %d, want >= 3 (id, version, steps)", len(ve.Errors()))
	}
}

// TestValidate_BranchCaseScopesAreIndependent confirms that two cases in
// the same branch may reuse a step ID — IDs are scoped per case.
func TestValidate_BranchCaseScopesAreIndependent(t *testing.T) {
	t.Parallel()
	def := contracts.WorkflowDefinition{
		ID:      "wf",
		Version: "1",
		Steps: []contracts.WorkflowStep{{
			ID: "router",
			Branch: &contracts.WorkflowBranchAction{
				Cases: []contracts.WorkflowBranchCase{
					{
						When: "true",
						Steps: []contracts.WorkflowStep{{
							ID:   "leaf",
							Tool: &contracts.WorkflowToolAction{Name: "noop"},
						}},
					},
					{
						When: "false",
						Steps: []contracts.WorkflowStep{{
							ID:   "leaf",
							Tool: &contracts.WorkflowToolAction{Name: "noop"},
						}},
					},
				},
			},
		}},
	}
	if err := Validate(def); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

// TestValidate_BranchDuplicateInsideOneCase rejects same-case duplicates.
func TestValidate_BranchDuplicateInsideOneCase(t *testing.T) {
	t.Parallel()
	def := contracts.WorkflowDefinition{
		ID:      "wf",
		Version: "1",
		Steps: []contracts.WorkflowStep{{
			ID: "router",
			Branch: &contracts.WorkflowBranchAction{
				Cases: []contracts.WorkflowBranchCase{{
					When: "true",
					Steps: []contracts.WorkflowStep{
						{ID: "leaf", Tool: &contracts.WorkflowToolAction{Name: "noop"}},
						{ID: "leaf", Tool: &contracts.WorkflowToolAction{Name: "noop"}},
					},
				}},
			},
		}},
	}
	err := Validate(def)
	if err == nil {
		t.Fatal("expected duplicate id error")
	}
	if !strings.Contains(err.Error(), "duplicate step id") {
		t.Errorf("err = %q, want %q", err.Error(), "duplicate step id")
	}
}

// TestValidate_TemplatesInsideBranchSteps walks the branch body to find
// template references that point at unknown step IDs.
func TestValidate_TemplatesInsideBranchSteps(t *testing.T) {
	t.Parallel()
	def := contracts.WorkflowDefinition{
		ID:      "wf",
		Version: "1",
		Steps: []contracts.WorkflowStep{{
			ID: "router",
			Branch: &contracts.WorkflowBranchAction{
				Cases: []contracts.WorkflowBranchCase{{
					When: "true",
					Steps: []contracts.WorkflowStep{{
						ID: "inner",
						Tool: &contracts.WorkflowToolAction{
							Name: "shell",
							With: map[string]any{"cmd": "{{ step.missing.output }}"},
						},
					}},
				}},
			},
		}},
	}
	err := Validate(def)
	if err == nil {
		t.Fatal("expected template error")
	}
	if !strings.Contains(err.Error(), "unknown step id") {
		t.Errorf("err = %q, want substring %q", err.Error(), "unknown step id")
	}
}

// TestValidate_NestedTemplateValuesInWith covers map-of-list-of-string
// recursion inside Tool.With.
func TestValidate_NestedTemplateValuesInWith(t *testing.T) {
	t.Parallel()
	def := contracts.WorkflowDefinition{
		ID:      "wf",
		Version: "1",
		Inputs:  map[string]contracts.WorkflowInput{"name": {Type: "string"}},
		Steps: []contracts.WorkflowStep{{
			ID: "first",
			Tool: &contracts.WorkflowToolAction{
				Name: "shell",
				With: map[string]any{
					"args": []any{"--name", "{{ inputs.name }}"},
					"env": map[string]any{
						"GREETING": "hello {{ inputs.name }}",
					},
				},
			},
		}},
	}
	if err := Validate(def); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

// TestValidationError_IsAndError asserts the sentinel + multi-error contract.
func TestValidationError_IsAndError(t *testing.T) {
	t.Parallel()
	v := &ValidationError{}
	v.add("id", "must not be empty")
	v.add("steps", "must contain at least one step")
	if !errors.Is(v, ErrValidation) {
		t.Error("errors.Is(v, ErrValidation) = false")
	}
	msg := v.Error()
	if !strings.Contains(msg, "id: must not be empty") {
		t.Errorf("err = %q, want id message", msg)
	}
	if !strings.Contains(msg, "steps: must contain at least one step") {
		t.Errorf("err = %q, want steps message", msg)
	}
	if got := v.Errors(); len(got) != 2 {
		t.Errorf("len(Errors()) = %d, want 2", len(got))
	}

	empty := &ValidationError{}
	if empty.orNil() != nil {
		t.Error("orNil on empty validator should be nil")
	}
	if empty.Error() != ErrValidation.Error() {
		t.Errorf("empty Error() = %q, want %q", empty.Error(), ErrValidation.Error())
	}
}

// TestValidate_ValidSixFieldCron accepts the seconds-prefixed cron form.
func TestValidate_ValidSixFieldCron(t *testing.T) {
	t.Parallel()
	def := minimalDef()
	def.Schedule = "*/10 0 9 * * 1"
	if err := Validate(def); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

// TestValidate_TemplateEdgeCases covers the empty-expression, malformed
// step shape, malformed inputs shape, and end-not-output paths.
func TestValidate_TemplateEdgeCases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		expr    string
		wantSub string
	}{
		{"empty expression", "{{ }}", "empty template expression"},
		{"step missing output suffix", "{{ step.first.foo }}", "must end in .output"},
		{"step ref too long", "{{ step.first.output.extra }}", "step.<id>.output"},
		{"inputs ref too long", "{{ inputs.foo.bar }}", "inputs.<name>"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			def := minimalDef()
			def.Inputs = map[string]contracts.WorkflowInput{"foo": {Type: "string"}}
			def.Steps[0].Tool.With = map[string]any{"x": tc.expr}
			err := Validate(def)
			if err == nil {
				t.Fatalf("expected error for %q", tc.expr)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

// TestValidate_FieldErrorWithoutPath exercises the bare-message path of
// fieldError.Error so the renderer's no-path branch stays covered.
func TestValidate_FieldErrorWithoutPath(t *testing.T) {
	t.Parallel()
	v := &ValidationError{}
	v.add("", "lonely message")
	if !strings.Contains(v.Error(), "lonely message") {
		t.Errorf("err = %q, want lonely message", v.Error())
	}
}

// TestValidationError_NilErrors guards the nil-receiver Errors() path.
func TestValidationError_NilErrors(t *testing.T) {
	t.Parallel()
	var v *ValidationError
	if got := v.Errors(); got != nil {
		t.Errorf("nil receiver Errors() = %v, want nil", got)
	}
}
