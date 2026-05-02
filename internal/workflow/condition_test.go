package workflow

import (
	"errors"
	"strings"
	"testing"
)

func TestEvaluate_NilCondition(t *testing.T) {
	ok, err := Evaluate(nil, nil)
	if err != nil {
		t.Fatalf("Evaluate(nil) returned error: %v", err)
	}
	if !ok {
		t.Fatal("Evaluate(nil) = false, want true")
	}
}

func TestEvaluate_Compare(t *testing.T) {
	cases := []struct {
		name    string
		cond    Condition
		prev    any
		want    bool
		wantErr bool
	}{
		// numeric operators
		{"eq numeric equal", Condition{Mode: ModeCompare, Operator: OpEq, Left: "5", Right: "5"}, nil, true, false},
		{"eq numeric different", Condition{Mode: ModeCompare, Operator: OpEq, Left: "5", Right: "6"}, nil, false, false},
		{"ne numeric different", Condition{Mode: ModeCompare, Operator: OpNe, Left: "5", Right: "6"}, nil, true, false},
		{"ne numeric equal", Condition{Mode: ModeCompare, Operator: OpNe, Left: "5", Right: "5"}, nil, false, false},
		{"lt true", Condition{Mode: ModeCompare, Operator: OpLt, Left: "1", Right: "2"}, nil, true, false},
		{"lt false", Condition{Mode: ModeCompare, Operator: OpLt, Left: "3", Right: "2"}, nil, false, false},
		{"le boundary", Condition{Mode: ModeCompare, Operator: OpLe, Left: "2", Right: "2"}, nil, true, false},
		{"gt true", Condition{Mode: ModeCompare, Operator: OpGt, Left: "10", Right: "2"}, nil, true, false},
		{"gt false", Condition{Mode: ModeCompare, Operator: OpGt, Left: "2", Right: "10"}, nil, false, false},
		{"ge boundary", Condition{Mode: ModeCompare, Operator: OpGe, Left: "2", Right: "2"}, nil, true, false},

		// string operators
		{"eq string", Condition{Mode: ModeCompare, Operator: OpEq, Left: "abc", Right: "abc"}, nil, true, false},
		{"contains true", Condition{Mode: ModeCompare, Operator: OpContains, Left: "hello world", Right: "world"}, nil, true, false},
		{"contains false", Condition{Mode: ModeCompare, Operator: OpContains, Left: "hello world", Right: "xyz"}, nil, false, false},
		{"startsWith true", Condition{Mode: ModeCompare, Operator: OpStartsWith, Left: "hello world", Right: "hello"}, nil, true, false},
		{"startsWith false", Condition{Mode: ModeCompare, Operator: OpStartsWith, Left: "hello", Right: "world"}, nil, false, false},
		{"endsWith true", Condition{Mode: ModeCompare, Operator: OpEndsWith, Left: "hello.go", Right: ".go"}, nil, true, false},
		{"endsWith false", Condition{Mode: ModeCompare, Operator: OpEndsWith, Left: "hello.go", Right: ".rs"}, nil, false, false},

		// JSONPath operands
		{
			"eq via jsonpath",
			Condition{Mode: ModeCompare, Operator: OpEq, Left: "$.status", Right: "ok"},
			map[string]any{"status": "ok"},
			true, false,
		},
		{
			"gt via jsonpath",
			Condition{Mode: ModeCompare, Operator: OpGt, Left: "$.count", Right: "10"},
			map[string]any{"count": 42},
			true, false,
		},
		{
			"jsonpath missing returns false on equality",
			Condition{Mode: ModeCompare, Operator: OpEq, Left: "$.missing", Right: "ok"},
			map[string]any{"status": "ok"},
			false, false,
		},

		// error paths
		{"unknown operator", Condition{Mode: ModeCompare, Operator: Operator("nope"), Left: "a", Right: "b"}, nil, false, true},
		{"non-numeric lt", Condition{Mode: ModeCompare, Operator: OpLt, Left: "abc", Right: "def"}, nil, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Evaluate(&tc.cond, tc.prev)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("Evaluate = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEvaluate_Regex(t *testing.T) {
	cases := []struct {
		name    string
		cond    Condition
		prev    any
		want    bool
		wantErr bool
	}{
		{"literal match", Condition{Mode: ModeRegex, Pattern: "^hello", Left: "hello world"}, nil, true, false},
		{"literal no match", Condition{Mode: ModeRegex, Pattern: "^world", Left: "hello world"}, nil, false, false},
		{"digit match", Condition{Mode: ModeRegex, Pattern: `\d+`, Left: "abc 123"}, nil, true, false},
		{
			"jsonpath value match",
			Condition{Mode: ModeRegex, Pattern: `^https://`, Left: "$.url"},
			map[string]any{"url": "https://example.com"},
			true, false,
		},
		{
			"jsonpath value miss",
			Condition{Mode: ModeRegex, Pattern: `^https://`, Left: "$.url"},
			map[string]any{"url": "ftp://example.com"},
			false, false,
		},
		{"invalid pattern", Condition{Mode: ModeRegex, Pattern: "[unterminated", Left: "x"}, nil, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Evaluate(&tc.cond, tc.prev)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("Evaluate = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEvaluate_JSONPath(t *testing.T) {
	cases := []struct {
		name string
		cond Condition
		prev any
		want bool
	}{
		{
			"truthy string",
			Condition{Mode: ModeJSONPath, Path: "$.name"},
			map[string]any{"name": "alice"},
			true,
		},
		{
			"empty string is falsy",
			Condition{Mode: ModeJSONPath, Path: "$.name"},
			map[string]any{"name": ""},
			false,
		},
		{
			"missing field",
			Condition{Mode: ModeJSONPath, Path: "$.missing"},
			map[string]any{"name": "alice"},
			false,
		},
		{
			"nested",
			Condition{Mode: ModeJSONPath, Path: "$.user.active"},
			map[string]any{"user": map[string]any{"active": true}},
			true,
		},
		{
			"nested false",
			Condition{Mode: ModeJSONPath, Path: "$.user.active"},
			map[string]any{"user": map[string]any{"active": false}},
			false,
		},
		{
			"array index",
			Condition{Mode: ModeJSONPath, Path: "$.items[0]"},
			map[string]any{"items": []any{"first", "second"}},
			true,
		},
		{
			"array index out of range",
			Condition{Mode: ModeJSONPath, Path: "$.items[5]"},
			map[string]any{"items": []any{"first"}},
			false,
		},
		{
			"chained array+field",
			Condition{Mode: ModeJSONPath, Path: "$.items[1].id"},
			map[string]any{"items": []any{
				map[string]any{"id": "a"},
				map[string]any{"id": "b"},
			}},
			true,
		},
		{
			"bracket field access",
			Condition{Mode: ModeJSONPath, Path: `$["weird key"]`},
			map[string]any{"weird key": "x"},
			true,
		},
		{
			"zero is falsy",
			Condition{Mode: ModeJSONPath, Path: "$.count"},
			map[string]any{"count": 0},
			false,
		},
		{
			"non-zero is truthy",
			Condition{Mode: ModeJSONPath, Path: "$.count"},
			map[string]any{"count": 42},
			true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Evaluate(&tc.cond, tc.prev)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("Evaluate = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEvaluate_UnknownMode(t *testing.T) {
	_, err := Evaluate(&Condition{Mode: "bogus"}, nil)
	if err == nil {
		t.Fatal("expected error for unknown mode")
	}
	if !errors.Is(err, ErrUnknownMode) {
		t.Fatalf("expected ErrUnknownMode, got %v", err)
	}
}

func TestNextStep_Unconditional(t *testing.T) {
	step := &Step{ID: "a", Next: "b"}
	got, err := NextStep(step, &StepResult{StepID: "a"})
	if err != nil {
		t.Fatalf("NextStep err: %v", err)
	}
	if got != "b" {
		t.Fatalf("NextStep = %q, want %q", got, "b")
	}
}

func TestNextStep_Conditional(t *testing.T) {
	step := &Step{
		ID:      "check",
		OnTrue:  "approved",
		OnFalse: "rejected",
		Condition: &Condition{
			Mode:     ModeCompare,
			Operator: OpEq,
			Left:     "$.status",
			Right:    "ok",
		},
	}
	cases := []struct {
		name   string
		output any
		want   string
	}{
		{"true branch", map[string]any{"status": "ok"}, "approved"},
		{"false branch", map[string]any{"status": "fail"}, "rejected"},
		{"missing field falls through to false", map[string]any{}, "rejected"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NextStep(step, &StepResult{StepID: "check", Output: tc.output})
			if err != nil {
				t.Fatalf("NextStep err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("NextStep = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNextStep_NilStep(t *testing.T) {
	if _, err := NextStep(nil, nil); err == nil {
		t.Fatal("expected error for nil step")
	}
}

func TestNextStep_NilResult(t *testing.T) {
	step := &Step{
		ID:      "check",
		OnTrue:  "yes",
		OnFalse: "no",
		Condition: &Condition{
			Mode: ModeJSONPath,
			Path: "$.flag",
		},
	}
	got, err := NextStep(step, nil)
	if err != nil {
		t.Fatalf("NextStep err: %v", err)
	}
	if got != "no" {
		t.Fatalf("NextStep = %q, want %q (nil result -> falsy path)", got, "no")
	}
}

func TestNextStep_BubbleEvaluateError(t *testing.T) {
	step := &Step{
		ID:      "broken",
		OnTrue:  "yes",
		OnFalse: "no",
		Condition: &Condition{
			Mode:    ModeRegex,
			Pattern: "[unterminated",
			Left:    "anything",
		},
	}
	if _, err := NextStep(step, &StepResult{StepID: "broken"}); err == nil {
		t.Fatal("expected regex-compile error to bubble up from NextStep")
	}
}

func TestEqual_TypeCoercion(t *testing.T) {
	// Confirm equal() bridges string vs numeric forms when either side
	// is numeric, since JSONPath operands often arrive as float64 from
	// JSON decoding while the YAML literal arrives as a string.
	cases := []struct {
		left, right any
		want        bool
	}{
		{float64(5), "5", true},
		{int64(5), "5", true},
		{"5", float64(5), true},
		{"5", "5", true},
		{nil, nil, true},
		{nil, "x", false},
		{"true", true, true}, // string form coercion
		{true, true, true},
		{true, false, false},
	}
	for i, tc := range cases {
		if got := equal(tc.left, tc.right); got != tc.want {
			t.Errorf("case %d: equal(%v, %v) = %v, want %v", i, tc.left, tc.right, got, tc.want)
		}
	}
}

func TestUnknownOperatorError_Format(t *testing.T) {
	_, err := Evaluate(&Condition{Mode: ModeCompare, Operator: "wat", Left: "a", Right: "b"}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "wat") {
		t.Fatalf("error %q does not mention bad operator", err.Error())
	}
}
