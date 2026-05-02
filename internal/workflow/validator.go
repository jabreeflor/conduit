package workflow

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// ErrValidation is the sentinel root for every workflow validation failure.
// Callers can identify aggregated validation errors with errors.Is(err,
// ErrValidation).
var ErrValidation = errors.New("workflow: validation failed")

// reservedStepIDs are identifiers that must not be used as step IDs
// because the template language reserves them for top-level scopes.
var reservedStepIDs = map[string]struct{}{
	"inputs":  {},
	"env":     {},
	"step":    {},
	"outputs": {},
}

// stepIDPattern restricts step identifiers to a Unix-friendly token shape so
// they round-trip safely in template references like `step.<id>.output`.
var stepIDPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// templateRefPattern matches a `{{ ... }}` template expression. The captured
// group holds the inner expression with surrounding whitespace trimmed by
// the caller.
var templateRefPattern = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

// cronFieldPattern is the per-field charset allowed inside a cron
// expression. The issue spec deliberately keeps this minimal — semantic
// range checks are deferred to the scheduler subpackage.
var cronFieldPattern = regexp.MustCompile(`^[0-9*/,\-]+$`)

// fieldError pairs a dotted field path with a human-readable message.
type fieldError struct {
	Path    string
	Message string
}

// Error renders the field error in `path: message` form for stable
// aggregation in ValidationError.
func (f fieldError) Error() string {
	if f.Path == "" {
		return f.Message
	}
	return fmt.Sprintf("%s: %s", f.Path, f.Message)
}

// ValidationError aggregates every problem discovered by Validate. The
// caller can iterate Errors() for individual issues; errors.Is(err,
// ErrValidation) holds for any non-empty ValidationError.
type ValidationError struct {
	errs []fieldError
}

// Error renders the aggregated errors in newline-separated form, prefixed
// with the sentinel so log scrapers can recognise the family.
func (v *ValidationError) Error() string {
	if v == nil || len(v.errs) == 0 {
		return ErrValidation.Error()
	}
	parts := make([]string, 0, len(v.errs)+1)
	parts = append(parts, ErrValidation.Error())
	for _, e := range v.errs {
		parts = append(parts, "  - "+e.Error())
	}
	return strings.Join(parts, "\n")
}

// Is reports whether target is the validation sentinel.
func (v *ValidationError) Is(target error) bool {
	return target == ErrValidation
}

// Errors returns the aggregated field errors as plain strings in
// `path: message` form. The slice is freshly allocated on each call.
func (v *ValidationError) Errors() []string {
	if v == nil {
		return nil
	}
	out := make([]string, 0, len(v.errs))
	for _, e := range v.errs {
		out = append(out, e.Error())
	}
	return out
}

// add appends a field-scoped error to the accumulator.
func (v *ValidationError) add(path, msg string) {
	v.errs = append(v.errs, fieldError{Path: path, Message: msg})
}

// orNil returns the receiver as an error when at least one issue was
// recorded, or nil when the validator is empty.
func (v *ValidationError) orNil() error {
	if v == nil || len(v.errs) == 0 {
		return nil
	}
	return v
}

// Validate enforces the schema rules described in issue #32 against def.
//
// Validate collects every problem before returning so authors fix all
// issues in one round-trip. The returned error, when non-nil, is a
// *ValidationError that wraps ErrValidation.
func Validate(def contracts.WorkflowDefinition) error {
	v := &ValidationError{}

	if strings.TrimSpace(def.ID) == "" {
		v.add("id", "must not be empty")
	}
	if strings.TrimSpace(def.Version) == "" {
		v.add("version", "must not be empty")
	}
	if def.Schedule != "" {
		if err := validateCron(def.Schedule); err != nil {
			v.add("schedule", err.Error())
		}
	}
	if len(def.Steps) == 0 {
		v.add("steps", "must contain at least one step")
	}

	// Track top-level step IDs to validate template references.
	topLevelIDs := make(map[string]struct{}, len(def.Steps))
	validateStepList(v, "steps", def.Steps, topLevelIDs)

	// Outputs may reference {{ step.<id>.output }} or {{ inputs.<name> }}.
	inputNames := make(map[string]struct{}, len(def.Inputs))
	for name := range def.Inputs {
		inputNames[name] = struct{}{}
	}
	for name, expr := range def.Outputs {
		path := fmt.Sprintf("outputs.%s", name)
		validateTemplateString(v, path, expr, topLevelIDs, inputNames)
	}

	// Walk steps once more to validate template references inside their
	// scalar payloads, now that we know which top-level step IDs exist.
	for i, step := range def.Steps {
		validateStepTemplates(v, fmt.Sprintf("steps[%d]", i), step, topLevelIDs, inputNames)
	}

	return v.orNil()
}

// validateStepList enforces the rules that apply to an ordered list of
// steps in a single lexical scope: ID uniqueness, ID shape, action
// presence, and recursion into branch cases.
func validateStepList(v *ValidationError, basePath string, steps []contracts.WorkflowStep, ids map[string]struct{}) {
	seen := make(map[string]int, len(steps))
	for i, step := range steps {
		path := fmt.Sprintf("%s[%d]", basePath, i)
		validateStep(v, path, step, seen, i)
		if step.ID != "" {
			ids[step.ID] = struct{}{}
		}
	}
}

// validateStep enforces per-step rules and dispatches into action-specific
// validators. The seen map tracks IDs already used in the same lexical
// scope so duplicates can be reported at the duplicate site.
func validateStep(v *ValidationError, path string, step contracts.WorkflowStep, seen map[string]int, index int) {
	id := strings.TrimSpace(step.ID)
	switch {
	case id == "":
		v.add(path+".id", "must not be empty")
	case !stepIDPattern.MatchString(id):
		v.add(path+".id", fmt.Sprintf("%q is not a valid identifier", id))
	default:
		if _, reserved := reservedStepIDs[id]; reserved {
			v.add(path+".id", fmt.Sprintf("%q is reserved and cannot be used as a step id", id))
		}
		if first, dup := seen[id]; dup {
			v.add(path+".id", fmt.Sprintf("duplicate step id %q (first declared at index %d)", id, first))
		} else {
			seen[id] = index
		}
	}

	if step.If != "" && strings.TrimSpace(step.If) == "" {
		v.add(path+".if", "must be a non-empty expression when present")
	}
	if step.When != "" && strings.TrimSpace(step.When) == "" {
		v.add(path+".when", "must be a non-empty expression when present")
	}

	kind, count := step.Action()
	switch count {
	case 0:
		v.add(path, "must declare exactly one of tool, model, subagent, or branch")
	case 1:
		validateAction(v, path, kind, step)
	default:
		v.add(path, fmt.Sprintf("declares %d action variants; exactly one is required", count))
	}
}

// validateAction routes to the per-kind validator.
func validateAction(v *ValidationError, path string, kind contracts.WorkflowActionKind, step contracts.WorkflowStep) {
	switch kind {
	case contracts.WorkflowActionTool:
		if strings.TrimSpace(step.Tool.Name) == "" {
			v.add(path+".tool.name", "must not be empty")
		}
	case contracts.WorkflowActionModel:
		if strings.TrimSpace(step.Model.Provider) == "" {
			v.add(path+".model.provider", "must not be empty")
		}
		if strings.TrimSpace(step.Model.Model) == "" {
			v.add(path+".model.model", "must not be empty")
		}
		if strings.TrimSpace(step.Model.Prompt) == "" {
			v.add(path+".model.prompt", "must not be empty")
		}
	case contracts.WorkflowActionSubagent:
		if strings.TrimSpace(step.Subagent.Profile) == "" {
			v.add(path+".subagent.profile", "must not be empty")
		}
		if strings.TrimSpace(step.Subagent.Prompt) == "" {
			v.add(path+".subagent.prompt", "must not be empty")
		}
	case contracts.WorkflowActionBranch:
		validateBranch(v, path+".branch", step.Branch)
	}
}

// validateBranch enforces case minima and recurses into every case body
// and the default body. Each case carries its own ID scope.
func validateBranch(v *ValidationError, path string, branch *contracts.WorkflowBranchAction) {
	if len(branch.Cases) == 0 {
		v.add(path+".cases", "must contain at least one case")
	}
	for i, c := range branch.Cases {
		casePath := fmt.Sprintf("%s.cases[%d]", path, i)
		if strings.TrimSpace(c.When) == "" {
			v.add(casePath+".when", "must not be empty")
		}
		caseIDs := make(map[string]struct{}, len(c.Steps))
		validateStepList(v, casePath+".steps", c.Steps, caseIDs)
	}
	if len(branch.Default) > 0 {
		defaultIDs := make(map[string]struct{}, len(branch.Default))
		validateStepList(v, path+".default", branch.Default, defaultIDs)
	}
}

// validateStepTemplates recurses into every scalar payload of a step,
// extracting `{{ ... }}` references and verifying they target a known
// top-level step or input.
func validateStepTemplates(v *ValidationError, path string, step contracts.WorkflowStep, ids, inputs map[string]struct{}) {
	if step.If != "" {
		validateTemplateString(v, path+".if", step.If, ids, inputs)
	}
	if step.When != "" {
		validateTemplateString(v, path+".when", step.When, ids, inputs)
	}
	switch kind, count := step.Action(); {
	case count != 1:
		// Already reported by validateStep; skip template checks.
		return
	default:
		switch kind {
		case contracts.WorkflowActionTool:
			validateAnyTemplates(v, path+".tool.with", step.Tool.With, ids, inputs)
		case contracts.WorkflowActionModel:
			validateTemplateString(v, path+".model.prompt", step.Model.Prompt, ids, inputs)
			validateTemplateString(v, path+".model.system", step.Model.System, ids, inputs)
		case contracts.WorkflowActionSubagent:
			validateTemplateString(v, path+".subagent.prompt", step.Subagent.Prompt, ids, inputs)
			validateAnyTemplates(v, path+".subagent.inputs", step.Subagent.Inputs, ids, inputs)
		case contracts.WorkflowActionBranch:
			for i, c := range step.Branch.Cases {
				casePath := fmt.Sprintf("%s.branch.cases[%d]", path, i)
				validateTemplateString(v, casePath+".when", c.When, ids, inputs)
				for j, sub := range c.Steps {
					validateStepTemplates(v, fmt.Sprintf("%s.steps[%d]", casePath, j), sub, ids, inputs)
				}
			}
			for i, sub := range step.Branch.Default {
				validateStepTemplates(v, fmt.Sprintf("%s.branch.default[%d]", path, i), sub, ids, inputs)
			}
		}
	}
}

// validateAnyTemplates walks an arbitrary YAML value (map / slice / scalar)
// and applies validateTemplateString to every string leaf.
func validateAnyTemplates(v *ValidationError, path string, value any, ids, inputs map[string]struct{}) {
	switch x := value.(type) {
	case nil:
		return
	case string:
		validateTemplateString(v, path, x, ids, inputs)
	case map[string]any:
		for k, vv := range x {
			validateAnyTemplates(v, fmt.Sprintf("%s.%s", path, k), vv, ids, inputs)
		}
	case []any:
		for i, vv := range x {
			validateAnyTemplates(v, fmt.Sprintf("%s[%d]", path, i), vv, ids, inputs)
		}
	}
}

// validateTemplateString extracts `{{ ... }}` references from s and checks
// each one against the legal forms `step.<id>.output` and `inputs.<name>`.
// Unknown referents and malformed expressions are reported at path.
func validateTemplateString(v *ValidationError, path, s string, ids, inputs map[string]struct{}) {
	if s == "" {
		return
	}
	matches := templateRefPattern.FindAllStringSubmatch(s, -1)
	for _, m := range matches {
		ref := strings.TrimSpace(m[1])
		if ref == "" {
			v.add(path, "empty template expression")
			continue
		}
		segments := strings.Split(ref, ".")
		switch segments[0] {
		case "step":
			if len(segments) != 3 {
				v.add(path, fmt.Sprintf("template %q must be of the form step.<id>.output", m[0]))
				continue
			}
			if segments[2] != "output" {
				v.add(path, fmt.Sprintf("template %q must end in .output", m[0]))
				continue
			}
			if _, ok := ids[segments[1]]; !ok {
				v.add(path, fmt.Sprintf("template %q references unknown step id %q", m[0], segments[1]))
			}
		case "inputs":
			if len(segments) != 2 {
				v.add(path, fmt.Sprintf("template %q must be of the form inputs.<name>", m[0]))
				continue
			}
			if _, ok := inputs[segments[1]]; !ok {
				v.add(path, fmt.Sprintf("template %q references unknown input %q", m[0], segments[1]))
			}
		default:
			v.add(path, fmt.Sprintf("template %q must reference step.<id>.output or inputs.<name>", m[0]))
		}
	}
}

// validateCron enforces the lightweight cron shape required by the issue
// spec: a 5- or 6-field expression where every field matches the charset
// `[0-9*/,\-]`. Semantic range checks are intentionally deferred — the
// scheduler subpackage owns those.
func validateCron(expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return errors.New("must not be empty")
	}
	fields := strings.Fields(expr)
	if len(fields) != 5 && len(fields) != 6 {
		return fmt.Errorf("must be a 5- or 6-field cron expression, got %d fields", len(fields))
	}
	for i, f := range fields {
		if !cronFieldPattern.MatchString(f) {
			return fmt.Errorf("field %d %q contains characters outside [0-9*/,-]", i+1, f)
		}
		// A field of just "-" or "," is syntactically wrong even if every
		// character is in the legal charset; reject the obvious cases by
		// requiring at least one digit or asterisk somewhere in the field.
		if !strings.ContainsAny(f, "0123456789*") {
			return fmt.Errorf("field %d %q must contain at least one numeric or wildcard token", i+1, f)
		}
	}
	return nil
}
