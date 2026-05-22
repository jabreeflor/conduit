package tui

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// fakeStep is a test double for the StepView interface.
type fakeStep struct {
	name     string
	status   StepStatus
	costUSD  float64
	duration time.Duration
}

func (s fakeStep) Name() string            { return s.name }
func (s fakeStep) Status() StepStatus      { return s.status }
func (s fakeStep) CostUSD() float64        { return s.costUSD }
func (s fakeStep) Duration() time.Duration { return s.duration }

// fakeWorkflow is a test double for the WorkflowStatus interface.
type fakeWorkflow struct {
	steps   []StepView
	current int
}

func (w fakeWorkflow) Steps() []StepView { return w.steps }
func (w fakeWorkflow) CurrentStep() int  { return w.current }

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, ""},
		{-1, ""},
		{45 * time.Millisecond, "45ms"},
		{999 * time.Millisecond, "999ms"},
		{1200 * time.Millisecond, "1.2s"},
		{59 * time.Second, "59.0s"},
		{75 * time.Second, "1m15s"},
		{2*time.Minute + 13*time.Second, "2m13s"},
		{time.Hour + 2*time.Minute, "1h2m"},
	}
	for _, tc := range cases {
		got := formatDuration(tc.in)
		if got != tc.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatCost(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, ""},
		{-0.5, ""},
		{0.0123, "$0.0123"},
		{1.5, "$1.5000"},
	}
	for _, tc := range cases {
		got := formatCost(tc.in)
		if got != tc.want {
			t.Errorf("formatCost(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// renderPlain returns the rendered output with ANSI codes stripped, so tests
// are stable across terminal capabilities.
func renderPlain(t *testing.T, w WorkflowStatus) string {
	t.Helper()
	var buf bytes.Buffer
	view := WorkflowStatusView{NoColor: true}
	if err := view.Render(&buf, w); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.String()
}

func TestRender_AllStatusKinds(t *testing.T) {
	wf := fakeWorkflow{
		current: 2,
		steps: []StepView{
			fakeStep{name: "fetch source", status: StepDone, costUSD: 0.0123, duration: 1200 * time.Millisecond},
			fakeStep{name: "lint", status: StepDone, costUSD: 0.0050, duration: 450 * time.Millisecond},
			fakeStep{name: "run tests", status: StepRunning, costUSD: 0, duration: 0},
			fakeStep{name: "deploy", status: StepPending},
			fakeStep{name: "smoke", status: StepFailed, costUSD: 0.0001, duration: 2*time.Minute + 13*time.Second},
		},
	}

	got := renderPlain(t, wf)
	want := "" +
		"1. ● fetch source $0.0123 1.2s\n" +
		"2. ● lint $0.0050 450ms\n" +
		"3. ▶ run tests\n" +
		"4. ○ deploy\n" +
		"5. ✕ smoke $0.0001 2m13s\n"

	if got != want {
		t.Fatalf("Render mismatch.\n got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRender_PendingOnly(t *testing.T) {
	wf := fakeWorkflow{
		current: -1,
		steps: []StepView{
			fakeStep{name: "step a", status: StepPending},
			fakeStep{name: "step b", status: StepPending},
		},
	}
	got := renderPlain(t, wf)
	want := "1. ○ step a\n2. ○ step b\n"
	if got != want {
		t.Fatalf("Render mismatch.\n got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRender_NilWorkflowIsNoop(t *testing.T) {
	var buf bytes.Buffer
	view := WorkflowStatusView{}
	if err := view.Render(&buf, nil); err != nil {
		t.Fatalf("Render(nil): %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no output for nil workflow, got %q", buf.String())
	}
}

func TestRender_EmptySteps(t *testing.T) {
	wf := fakeWorkflow{current: -1, steps: nil}
	got := renderPlain(t, wf)
	if got != "" {
		t.Fatalf("expected empty output for no steps, got %q", got)
	}
}

func TestRender_ColoredOutputContainsANSI(t *testing.T) {
	// When NoColor is false (the default), ANSI escapes must appear.
	wf := fakeWorkflow{
		current: 0,
		steps: []StepView{
			fakeStep{name: "build", status: StepRunning},
		},
	}
	var buf bytes.Buffer
	view := WorkflowStatusView{}
	if err := view.Render(&buf, wf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ANSI escape codes in colored output, got %q", out)
	}
	// The current running step name should be wrapped in bold+cyan.
	if !strings.Contains(out, ansiBold+ansiCyan+"build"+ansiReset) {
		t.Fatalf("expected current step name to be bold-cyan highlighted, got %q", out)
	}
	// And the running glyph should be present (highlighted).
	if !strings.Contains(out, "▶") {
		t.Fatalf("expected running glyph in output, got %q", out)
	}
}

func TestRender_FailedStepIsRed(t *testing.T) {
	wf := fakeWorkflow{
		current: -1,
		steps: []StepView{
			fakeStep{name: "broken", status: StepFailed, costUSD: 0.01, duration: 30 * time.Millisecond},
		},
	}
	var buf bytes.Buffer
	if err := (WorkflowStatusView{}).Render(&buf, wf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, ansiRed+"✕"+ansiReset) {
		t.Fatalf("expected failed glyph wrapped in red ANSI, got %q", out)
	}
}

func TestRender_CompletedStepIsGreen(t *testing.T) {
	wf := fakeWorkflow{
		current: -1,
		steps: []StepView{
			fakeStep{name: "ok", status: StepDone, costUSD: 0.005, duration: time.Second},
		},
	}
	var buf bytes.Buffer
	if err := (WorkflowStatusView{}).Render(&buf, wf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, ansiGreen+"●"+ansiReset) {
		t.Fatalf("expected completed glyph wrapped in green ANSI, got %q", out)
	}
}

func TestStripANSI(t *testing.T) {
	in := ansiBold + ansiCyan + "hello" + ansiReset + " " + ansiRed + "world" + ansiReset
	got := stripANSI(in)
	want := "hello world"
	if got != want {
		t.Fatalf("stripANSI = %q, want %q", got, want)
	}
}
