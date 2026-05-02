package workflow

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// recordingExecutor logs each (provider, stepID) call and dispatches to a
// per-call handler so each test can shape a deterministic scenario.
type recordingExecutor struct {
	calls   []callRecord
	handler func(call callRecord) (string, error)
}

type callRecord struct {
	Provider string
	StepID   string
	Index    int
}

func (r *recordingExecutor) Execute(_ context.Context, provider string, step Step) (string, error) {
	rec := callRecord{Provider: provider, StepID: step.ID, Index: len(r.calls)}
	r.calls = append(r.calls, rec)
	if r.handler == nil {
		return "ok", nil
	}
	return r.handler(rec)
}

func newTestEngine(t *testing.T, exec StepExecutor) (*Engine, Checkpointer) {
	t.Helper()
	cp, err := NewFileCheckpointer(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileCheckpointer: %v", err)
	}
	eng := NewEngine(cp, exec)
	// Deterministic clock so timestamps stay stable across CI runners.
	base := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	tick := 0
	eng.SetClock(func() time.Time {
		tick++
		return base.Add(time.Duration(tick) * time.Second)
	})
	return eng, cp
}

func sampleWorkflow(providers ...string) Workflow {
	return Workflow{
		ID:   "wf-1",
		Name: "sample",
		Steps: []Step{
			{ID: "a", Name: "a", Prompt: "do a"},
			{ID: "b", Name: "b", Prompt: "do b"},
			{ID: "c", Name: "c", Prompt: "do c"},
		},
		Providers: providers,
	}
}

func TestEngineStartCompletesAllSteps(t *testing.T) {
	exec := &recordingExecutor{
		handler: func(c callRecord) (string, error) {
			return "out:" + c.StepID, nil
		},
	}
	eng, cp := newTestEngine(t, exec)

	run, err := eng.Start(context.Background(), sampleWorkflow("p1"), "run-happy")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if run.State != RunStateCompleted {
		t.Fatalf("State = %q, want completed", run.State)
	}
	if run.CurrentStep != 3 {
		t.Errorf("CurrentStep = %d, want 3", run.CurrentStep)
	}
	if len(run.Results) != 3 {
		t.Fatalf("Results len = %d, want 3", len(run.Results))
	}
	for i, want := range []string{"out:a", "out:b", "out:c"} {
		got, _ := run.Results[i].Output.(string)
		if got != want {
			t.Errorf("Results[%d].Output = %v, want %q", i, run.Results[i].Output, want)
		}
		if run.Results[i].Provider != "p1" {
			t.Errorf("Results[%d].Provider = %q, want p1", i, run.Results[i].Provider)
		}
	}

	loaded, err := cp.Load("run-happy")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.State != RunStateCompleted || len(loaded.Results) != 3 {
		t.Errorf("checkpoint did not capture completion: %+v", loaded)
	}
}

func TestEngineResumePicksUpAtNextStep(t *testing.T) {
	// First Engine: panic on the third Step to simulate a crash.
	type stopErr struct{ error }
	stopper := &recordingExecutor{
		handler: func(c callRecord) (string, error) {
			if c.StepID == "c" {
				return "", stopErr{errors.New("simulated crash")}
			}
			return "out:" + c.StepID, nil
		},
	}
	eng1, cp := newTestEngine(t, stopper)

	_, err := eng1.Start(context.Background(), sampleWorkflow(), "run-resume")
	if err == nil {
		t.Fatal("Start should have returned the simulated error")
	}

	// Confirm the on-disk checkpoint sits exactly at the failed step.
	mid, err := cp.Load("run-resume")
	if err != nil {
		t.Fatalf("Load mid: %v", err)
	}
	if mid.State != RunStateFailed {
		t.Errorf("mid State = %q, want failed", mid.State)
	}
	if mid.CurrentStep != 2 {
		t.Errorf("mid CurrentStep = %d, want 2", mid.CurrentStep)
	}
	if len(mid.Results) != 3 { // 2 successes + 1 failure record
		t.Errorf("mid Results len = %d, want 3", len(mid.Results))
	}

	// To exercise Resume's actual continuation path, repair the checkpoint
	// to the running state that would exist after step b succeeded.
	mid.State = RunStateRunning
	mid.LastError = ""
	mid.Results = mid.Results[:2]
	if err := cp.Save(mid); err != nil {
		t.Fatalf("Save repaired: %v", err)
	}

	// Second Engine: a fresh executor that succeeds. Resume should only
	// invoke it for step c.
	good := &recordingExecutor{
		handler: func(c callRecord) (string, error) {
			return "resumed:" + c.StepID, nil
		},
	}
	eng2 := NewEngine(cp, good)

	resumed, err := eng2.Resume(context.Background(), "run-resume")
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if resumed.State != RunStateCompleted {
		t.Errorf("resumed State = %q, want completed", resumed.State)
	}
	if len(good.calls) != 1 {
		t.Fatalf("expected 1 executor call after resume, got %d (%v)", len(good.calls), good.calls)
	}
	if good.calls[0].StepID != "c" {
		t.Errorf("resume started at step %q, want c", good.calls[0].StepID)
	}
	if len(resumed.Results) != 3 {
		t.Errorf("Results len = %d, want 3", len(resumed.Results))
	}
	if got, _ := resumed.Results[2].Output.(string); got != "resumed:c" {
		t.Errorf("step c output = %v, want resumed:c", resumed.Results[2].Output)
	}
}

func TestEngineResumeAlreadyTerminal(t *testing.T) {
	exec := &recordingExecutor{}
	eng, cp := newTestEngine(t, exec)

	completed := &Run{
		ID:          "run-done",
		WorkflowID:  "wf",
		Workflow:    sampleWorkflow(),
		State:       RunStateCompleted,
		CurrentStep: 3,
	}
	if err := cp.Save(completed); err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}

	got, err := eng.Resume(context.Background(), "run-done")
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if got.State != RunStateCompleted {
		t.Errorf("State = %q, want completed", got.State)
	}
	if len(exec.calls) != 0 {
		t.Errorf("expected no executor calls, got %d", len(exec.calls))
	}
}

func TestEngineFailoverAdvancesProviderOnError(t *testing.T) {
	// p1 always fails with a recoverable ProviderError; p2 succeeds.
	exec := &recordingExecutor{
		handler: func(c callRecord) (string, error) {
			if c.Provider == "p1" {
				return "", NewProviderFailure("p1", fmt.Errorf("boom on %s", c.StepID))
			}
			return "out:" + c.StepID, nil
		},
	}
	eng, cp := newTestEngine(t, exec)

	wf := sampleWorkflow("p1", "p2")
	run, err := eng.Start(context.Background(), wf, "run-failover")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if run.State != RunStateCompleted {
		t.Fatalf("State = %q, want completed", run.State)
	}

	// Each step calls p1 then p2: 3 steps * 2 providers = 6 calls.
	if len(exec.calls) != 6 {
		t.Fatalf("expected 6 executor calls, got %d", len(exec.calls))
	}
	for i := 0; i < 6; i += 2 {
		if exec.calls[i].Provider != "p1" {
			t.Errorf("calls[%d].Provider = %q, want p1", i, exec.calls[i].Provider)
		}
		if exec.calls[i+1].Provider != "p2" {
			t.Errorf("calls[%d].Provider = %q, want p2", i+1, exec.calls[i+1].Provider)
		}
	}
	for i, r := range run.Results {
		if r.Provider != "p2" {
			t.Errorf("Results[%d].Provider = %q, want p2", i, r.Provider)
		}
	}

	loaded, err := cp.Load("run-failover")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.State != RunStateCompleted {
		t.Errorf("loaded state = %q, want completed", loaded.State)
	}
}

func TestEngineFailoverChainExhaustionFailsRun(t *testing.T) {
	exec := &recordingExecutor{
		handler: func(c callRecord) (string, error) {
			return "", NewProviderFailure(c.Provider, errors.New("upstream down"))
		},
	}
	eng, cp := newTestEngine(t, exec)

	run, err := eng.Start(context.Background(), sampleWorkflow("p1", "p2"), "run-bust")
	if err == nil {
		t.Fatal("expected Start to return the chain-exhaustion error")
	}
	if run.State != RunStateFailed {
		t.Errorf("State = %q, want failed", run.State)
	}
	if run.LastError == "" {
		t.Error("LastError should be populated on failure")
	}
	// The first Step exhausts the chain; subsequent Steps must not run.
	if len(exec.calls) != 2 {
		t.Errorf("expected 2 calls (one per provider), got %d", len(exec.calls))
	}
	loaded, err := cp.Load("run-bust")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.State != RunStateFailed {
		t.Errorf("checkpoint state = %q, want failed", loaded.State)
	}
}

func TestEngineNonProviderErrorIsTerminal(t *testing.T) {
	// A plain error (not ProviderError) must NOT trigger failover.
	exec := &recordingExecutor{
		handler: func(c callRecord) (string, error) {
			return "", errors.New("hard stop")
		},
	}
	eng, _ := newTestEngine(t, exec)

	_, err := eng.Start(context.Background(), sampleWorkflow("p1", "p2"), "run-hard")
	if err == nil {
		t.Fatal("expected error")
	}
	if len(exec.calls) != 1 {
		t.Errorf("expected single call (no failover), got %d", len(exec.calls))
	}
}

func TestEngineNoProvidersStillRuns(t *testing.T) {
	exec := &recordingExecutor{
		handler: func(c callRecord) (string, error) { return "ok:" + c.StepID, nil },
	}
	eng, _ := newTestEngine(t, exec)

	run, err := eng.Start(context.Background(), sampleWorkflow(), "run-noprov")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if run.State != RunStateCompleted {
		t.Fatalf("State = %q, want completed", run.State)
	}
	for _, c := range exec.calls {
		if c.Provider != "" {
			t.Errorf("expected empty provider, got %q", c.Provider)
		}
	}
}

func TestEngineEmptyWorkflowCompletes(t *testing.T) {
	exec := &recordingExecutor{}
	eng, _ := newTestEngine(t, exec)

	wf := Workflow{ID: "empty", Name: "empty"}
	run, err := eng.Start(context.Background(), wf, "run-empty")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if run.State != RunStateCompleted {
		t.Errorf("State = %q, want completed", run.State)
	}
	if len(exec.calls) != 0 {
		t.Errorf("expected zero calls, got %d", len(exec.calls))
	}
}

func TestEngineStartGeneratesRunID(t *testing.T) {
	exec := &recordingExecutor{}
	eng, _ := newTestEngine(t, exec)

	run, err := eng.Start(context.Background(), Workflow{ID: "wf"}, "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(run.ID) != 32 { // 16 random bytes hex-encoded
		t.Errorf("generated ID has unexpected length %d: %q", len(run.ID), run.ID)
	}
}

func TestEngineStepExecutorFuncAdapter(t *testing.T) {
	// Make sure the function adapter type compiles into the StepExecutor
	// interface without ceremony.
	var _ StepExecutor = StepExecutorFunc(func(_ context.Context, _ string, _ Step) (string, error) {
		return "", nil
	})
}

func TestEngineContextCancelStopsBetweenSteps(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	exec := &recordingExecutor{
		handler: func(c callRecord) (string, error) {
			if c.StepID == "a" {
				cancel()
			}
			return "ok", nil
		},
	}
	eng, cp := newTestEngine(t, exec)

	_, err := eng.Start(ctx, sampleWorkflow(), "run-cancel")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	loaded, err := cp.Load("run-cancel")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.CurrentStep != 1 {
		t.Errorf("CurrentStep = %d, want 1 (after step a only)", loaded.CurrentStep)
	}
}
