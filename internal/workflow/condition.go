package workflow

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ConditionMode selects which evaluator a Condition uses.
type ConditionMode string

const (
	// ModeCompare runs Operator on Left vs Right. Either operand may be a
	// JSONPath expression; see resolveOperand.
	ModeCompare ConditionMode = "compare"
	// ModeRegex matches Pattern against Left (which may itself be a
	// JSONPath expression).
	ModeRegex ConditionMode = "regex"
	// ModeJSONPath extracts a value via Path and tests for truthiness.
	// Combine with ModeCompare/ModeRegex by setting Left to the path
	// and using those modes directly when more than existence is needed.
	ModeJSONPath ConditionMode = "jsonpath"
)

// Operator enumerates the comparison operators supported by ModeCompare.
type Operator string

const (
	OpEq         Operator = "eq"
	OpNe         Operator = "ne"
	OpLt         Operator = "lt"
	OpLe         Operator = "le"
	OpGt         Operator = "gt"
	OpGe         Operator = "ge"
	OpContains   Operator = "contains"
	OpStartsWith Operator = "startsWith"
	OpEndsWith   Operator = "endsWith"
)

// Condition declares a boolean test against a previous step's StepResult.
// PRD §6.7 lists three modes: comparison, regex, and JSONPath; this struct
// serializes all three as a tagged union via Mode.
type Condition struct {
	// Mode selects the evaluator. Required.
	Mode ConditionMode `json:"mode" yaml:"mode"`

	// Left is the operand consumed by ModeCompare and ModeRegex. A value
	// starting with "$" is treated as a JSONPath expression evaluated
	// against the previous StepResult's Output (see resolveOperand);
	// otherwise it is used as a literal string.
	Left string `json:"left,omitempty" yaml:"left,omitempty"`

	// Right is the second operand for ModeCompare. Same JSONPath rules
	// as Left.
	Right string `json:"right,omitempty" yaml:"right,omitempty"`

	// Operator is the comparison operator for ModeCompare.
	Operator Operator `json:"operator,omitempty" yaml:"operator,omitempty"`

	// Pattern is the Go RE2 regex used by ModeRegex.
	Pattern string `json:"pattern,omitempty" yaml:"pattern,omitempty"`

	// Path is the JSONPath expression used by ModeJSONPath. The
	// condition evaluates true iff the path resolves to a truthy value.
	Path string `json:"path,omitempty" yaml:"path,omitempty"`
}

// ErrUnknownMode is returned when Condition.Mode is not one of the
// supported ConditionMode values.
var ErrUnknownMode = errors.New("workflow: unknown condition mode")

// ErrUnknownOperator is returned when an unsupported Operator is used in
// ModeCompare.
var ErrUnknownOperator = errors.New("workflow: unknown operator")

// Evaluate applies cond to prevOutput and returns whether the condition
// holds. A nil cond evaluates to true so that callers can treat
// unconditional steps uniformly.
func Evaluate(cond *Condition, prevOutput any) (bool, error) {
	if cond == nil {
		return true, nil
	}
	switch cond.Mode {
	case ModeCompare:
		return evalCompare(cond, prevOutput)
	case ModeRegex:
		return evalRegex(cond, prevOutput)
	case ModeJSONPath:
		return evalJSONPath(cond, prevOutput)
	default:
		return false, fmt.Errorf("%w: %q", ErrUnknownMode, cond.Mode)
	}
}

// NextStep returns the ID of the step that should run after step, given
// the result of step's execution. The empty string means "stop". The
// caller is responsible for honoring step.Next when Condition is nil.
func NextStep(step *Step, result *StepResult) (string, error) {
	if step == nil {
		return "", errors.New("workflow: nil step")
	}
	if step.Condition == nil {
		return step.Next, nil
	}
	var prev any
	if result != nil {
		prev = result.Output
	}
	ok, err := Evaluate(step.Condition, prev)
	if err != nil {
		return "", err
	}
	if ok {
		return step.OnTrue, nil
	}
	return step.OnFalse, nil
}

// evalCompare resolves both operands then applies the operator.
func evalCompare(cond *Condition, prev any) (bool, error) {
	left, err := resolveOperand(cond.Left, prev)
	if err != nil {
		return false, fmt.Errorf("compare left: %w", err)
	}
	right, err := resolveOperand(cond.Right, prev)
	if err != nil {
		return false, fmt.Errorf("compare right: %w", err)
	}
	return applyOperator(cond.Operator, left, right)
}

// evalRegex matches Pattern against Left.
func evalRegex(cond *Condition, prev any) (bool, error) {
	left, err := resolveOperand(cond.Left, prev)
	if err != nil {
		return false, fmt.Errorf("regex value: %w", err)
	}
	re, err := regexp.Compile(cond.Pattern)
	if err != nil {
		return false, fmt.Errorf("regex pattern: %w", err)
	}
	return re.MatchString(toString(left)), nil
}

// evalJSONPath returns true when Path resolves to a truthy value.
func evalJSONPath(cond *Condition, prev any) (bool, error) {
	v, ok, err := jsonPath(cond.Path, prev)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return truthy(v), nil
}

// resolveOperand returns the operand as an any. Values starting with "$"
// are interpreted as JSONPath expressions evaluated against prev; the
// resolved value is returned. Other values are returned as the literal
// string. A missing JSONPath match returns nil with no error so that
// comparisons against absent data behave like comparisons against nil.
func resolveOperand(operand string, prev any) (any, error) {
	if strings.HasPrefix(operand, "$") {
		v, ok, err := jsonPath(operand, prev)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
		return v, nil
	}
	return operand, nil
}

// applyOperator runs op on left and right. Numeric operators coerce both
// operands to float64 when possible; string operators coerce via
// toString. Equality compares via string form when types differ.
func applyOperator(op Operator, left, right any) (bool, error) {
	switch op {
	case OpEq:
		return equal(left, right), nil
	case OpNe:
		return !equal(left, right), nil
	case OpLt, OpLe, OpGt, OpGe:
		lf, lok := toFloat(left)
		rf, rok := toFloat(right)
		if !lok || !rok {
			return false, fmt.Errorf("%w: non-numeric operand for %s", ErrUnknownOperator, op)
		}
		switch op {
		case OpLt:
			return lf < rf, nil
		case OpLe:
			return lf <= rf, nil
		case OpGt:
			return lf > rf, nil
		case OpGe:
			return lf >= rf, nil
		}
	case OpContains:
		return strings.Contains(toString(left), toString(right)), nil
	case OpStartsWith:
		return strings.HasPrefix(toString(left), toString(right)), nil
	case OpEndsWith:
		return strings.HasSuffix(toString(left), toString(right)), nil
	}
	return false, fmt.Errorf("%w: %q", ErrUnknownOperator, op)
}

// equal returns true when left and right represent the same value. It
// first tries numeric comparison; on failure it falls back to string
// equality. nil equals nil.
func equal(left, right any) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	if lf, lok := toFloat(left); lok {
		if rf, rok := toFloat(right); rok {
			return lf == rf
		}
	}
	if lb, lok := left.(bool); lok {
		if rb, rok := right.(bool); rok {
			return lb == rb
		}
	}
	return toString(left) == toString(right)
}

// toFloat returns v as a float64, true on success.
func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

// toString formats v for string operations. nil renders as the empty
// string; everything else uses fmt's default formatting.
func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// truthy mirrors the "falsy" set most scripting languages use: nil, "",
// false, and zero numeric values are false; everything else is true.
func truthy(v any) bool {
	if v == nil {
		return false
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0
	case int:
		return x != 0
	case int64:
		return x != 0
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	}
	if f, ok := toFloat(v); ok {
		return f != 0
	}
	return true
}
