package coding

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type recordingRunner struct {
	mu       sync.Mutex
	order    []string
	parallel atomic.Int32
	maxPar   atomic.Int32
	failOn   map[string]bool
	delay    time.Duration
}

func (r *recordingRunner) Run(ctx context.Context, t DelegateTask, lineage []DelegateLineageStep) (DelegateResult, error) {
	cur := r.parallel.Add(1)
	for {
		old := r.maxPar.Load()
		if cur <= old || r.maxPar.CompareAndSwap(old, cur) {
			break
		}
	}
	defer r.parallel.Add(-1)
	if r.delay > 0 {
		time.Sleep(r.delay)
	}
	r.mu.Lock()
	r.order = append(r.order, t.ID)
	r.mu.Unlock()
	if r.failOn[t.ID] {
		return DelegateResult{TaskID: t.ID, Lineage: lineage}, errors.New("forced failure")
	}
	return DelegateResult{
		TaskID:    t.ID,
		Agent:     t.Agent,
		SessionID: "child-" + t.ID,
		Output:    "out:" + t.Prompt,
		Lineage:   lineage,
	}, nil
}

func TestDelegateBatchRunsInTopologicalOrder(t *testing.T) {
	r := &recordingRunner{}
	m := NewAgentManager(r)
	results, err := m.DelegateBatch(context.Background(), "parent-1", []DelegateTask{
		{ID: "c", DependsOn: []string{"a", "b"}, Prompt: "C"},
		{ID: "a", Prompt: "A"},
		{ID: "b", Prompt: "B"},
		{ID: "d", DependsOn: []string{"c"}, Prompt: "D"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	// a and b come before c which comes before d.
	posOf := map[string]int{}
	for i, id := range r.order {
		posOf[id] = i
	}
	if !(posOf["a"] < posOf["c"] && posOf["b"] < posOf["c"] && posOf["c"] < posOf["d"]) {
		t.Errorf("topo order broken: %v", r.order)
	}
}

func TestDelegateBatchParallelInWave(t *testing.T) {
	r := &recordingRunner{delay: 50 * time.Millisecond}
	m := NewAgentManager(r)
	_, err := m.DelegateBatch(context.Background(), "parent", []DelegateTask{
		{ID: "a", Prompt: "A"},
		{ID: "b", Prompt: "B"},
		{ID: "c", Prompt: "C"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.maxPar.Load() < 2 {
		t.Errorf("expected parallel execution, max was %d", r.maxPar.Load())
	}
}

func TestDelegateBatchSkipsDownstreamOnFailure(t *testing.T) {
	r := &recordingRunner{failOn: map[string]bool{"a": true}}
	m := NewAgentManager(r)
	results, err := m.DelegateBatch(context.Background(), "parent", []DelegateTask{
		{ID: "a", Prompt: "A"},
		{ID: "b", DependsOn: []string{"a"}, Prompt: "B"},
		{ID: "c", DependsOn: []string{"b"}, Prompt: "C"},
		{ID: "d", Prompt: "D"}, // independent — should still run
	})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]DelegateResult{}
	for _, r := range results {
		byID[r.TaskID] = r
	}
	if byID["a"].Error == "" {
		t.Errorf("a should have failed")
	}
	if !strings.Contains(byID["b"].Error, "skipped") || !strings.Contains(byID["c"].Error, "skipped") {
		t.Errorf("b/c should be skipped: %+v %+v", byID["b"], byID["c"])
	}
	if byID["d"].Error != "" {
		t.Errorf("d should have run: %+v", byID["d"])
	}
}

func TestDelegateCycleDetected(t *testing.T) {
	m := NewAgentManager(&recordingRunner{})
	_, err := m.DelegateBatch(context.Background(), "p", []DelegateTask{
		{ID: "a", DependsOn: []string{"b"}},
		{ID: "b", DependsOn: []string{"a"}},
	})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected cycle error, got %v", err)
	}
}

func TestDelegateUnknownDependency(t *testing.T) {
	m := NewAgentManager(&recordingRunner{})
	_, err := m.DelegateBatch(context.Background(), "p", []DelegateTask{
		{ID: "a", DependsOn: []string{"missing"}},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Errorf("expected unknown-dep error, got %v", err)
	}
}

func TestDelegateLineageRecorded(t *testing.T) {
	m := NewAgentManager(&recordingRunner{})
	_, err := m.DelegateBatch(context.Background(), "parent-X", []DelegateTask{{ID: "a", Prompt: "p"}})
	if err != nil {
		t.Fatal(err)
	}
	lin := m.LineageOf("child-a")
	if len(lin) != 1 || lin[0].ParentSessionID != "parent-X" || lin[0].ParentTaskID != "a" {
		t.Errorf("lineage missing or wrong: %+v", lin)
	}
}

func TestSummarizeBatch(t *testing.T) {
	results := []DelegateResult{
		{TaskID: "a", Agent: "writer", Output: "first line\nsecond"},
		{TaskID: "b", Agent: "writer", Error: "boom"},
		{TaskID: "c", Agent: "writer", Error: "skipped: upstream dependency failed"},
	}
	out := SummarizeBatch(results)
	for _, want := range []string{"ok=1", "failed=1", "skipped=1", "[ok] a", "[fail] b", "[skip] c", "first line", "boom"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
}
