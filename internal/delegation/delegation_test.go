package delegation_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/delegation"
)

// mockRunner records calls and returns canned responses.
type mockRunner struct {
	mu      interface{ Lock(); Unlock() }
	calls   []delegation.SubagentSpec
	outputs map[string]string
	errs    map[string]error
	delay   time.Duration
}

func newMockRunner() *mockRunner {
	return &mockRunner{
		outputs: map[string]string{},
		errs:    map[string]error{},
	}
}

func (m *mockRunner) Run(ctx context.Context, spec delegation.SubagentSpec) delegation.SubagentResult {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return delegation.SubagentResult{Name: spec.Name, Error: ctx.Err()}
		}
	}
	start := time.Now()
	out := m.outputs[spec.Name]
	if out == "" {
		out = "output:" + spec.Name
	}
	err := m.errs[spec.Name]
	return delegation.SubagentResult{
		Name:     spec.Name,
		Output:   out,
		Error:    err,
		Duration: time.Since(start),
	}
}

func specs(names ...string) []delegation.SubagentSpec {
	out := make([]delegation.SubagentSpec, len(names))
	for i, n := range names {
		out[i] = delegation.SubagentSpec{Name: n, Prompt: "do " + n}
	}
	return out
}

func TestRunParallel(t *testing.T) {
	runner := newMockRunner()
	o := delegation.NewOrchestrator(runner, 0)
	results, err := o.Run(context.Background(), specs("a", "b", "c"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("want 3 results, got %d", len(results))
	}
	for i, r := range results {
		want := specs("a", "b", "c")[i].Name
		if r.Name != want {
			t.Errorf("result[%d].Name = %q, want %q", i, r.Name, want)
		}
	}
}

func TestRunConcurrencyLimit(t *testing.T) {
	runner := newMockRunner()
	runner.delay = 10 * time.Millisecond
	o := delegation.NewOrchestrator(runner, 2)
	results, err := o.Run(context.Background(), specs("x", "y", "z"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("want 3, got %d", len(results))
	}
}

func TestRunReturnsErrorOnFailure(t *testing.T) {
	runner := newMockRunner()
	runner.errs["bad"] = errors.New("boom")
	o := delegation.NewOrchestrator(runner, 0)
	_, err := o.Run(context.Background(), specs("good", "bad"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Errorf("error should mention subagent name, got: %v", err)
	}
}

func TestRunSequential(t *testing.T) {
	runner := newMockRunner()
	o := delegation.NewOrchestrator(runner, 0)
	results, err := o.RunSequential(context.Background(), specs("p", "q"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("want 2, got %d", len(results))
	}
	if results[0].Name != "p" || results[1].Name != "q" {
		t.Errorf("wrong order: %v", results)
	}
}

func TestRunSequentialStopsOnError(t *testing.T) {
	runner := newMockRunner()
	runner.errs["first"] = fmt.Errorf("fail")
	o := delegation.NewOrchestrator(runner, 0)
	results, err := o.RunSequential(context.Background(), specs("first", "second"))
	if err == nil {
		t.Fatal("expected error")
	}
	if len(results) != 1 {
		t.Errorf("want 1 result before stop, got %d", len(results))
	}
}

func TestAggregate(t *testing.T) {
	o := delegation.NewOrchestrator(newMockRunner(), 0)
	results := []delegation.SubagentResult{
		{Name: "alpha", Output: "result-alpha"},
		{Name: "beta", Output: "result-beta"},
	}
	got := o.Aggregate(results)
	if !strings.Contains(got, "## alpha") || !strings.Contains(got, "result-alpha") {
		t.Errorf("aggregate missing alpha section: %s", got)
	}
	if !strings.Contains(got, "## beta") {
		t.Errorf("aggregate missing beta section: %s", got)
	}
}

func TestAggregateWithError(t *testing.T) {
	o := delegation.NewOrchestrator(newMockRunner(), 0)
	results := []delegation.SubagentResult{
		{Name: "ok", Output: "fine"},
		{Name: "fail", Error: errors.New("kaboom")},
	}
	got := o.Aggregate(results)
	if !strings.Contains(got, "kaboom") {
		t.Errorf("aggregate should include error text: %s", got)
	}
}

func TestParseTimeout(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", 5 * time.Minute},
		{"invalid", 5 * time.Minute},
		{"30s", 30 * time.Second},
		{"2m", 2 * time.Minute},
	}
	for _, c := range cases {
		got := delegation.ParseTimeout(c.in)
		if got != c.want {
			t.Errorf("ParseTimeout(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestRunEmpty(t *testing.T) {
	o := delegation.NewOrchestrator(newMockRunner(), 0)
	results, err := o.Run(context.Background(), nil)
	if err != nil || len(results) != 0 {
		t.Errorf("empty input should return nil, nil; got %v, %v", results, err)
	}
}
