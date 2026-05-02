package workflow

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// ProviderError marks a StepExecutor failure as recoverable via provider
// failover. The Engine catches errors satisfying this interface (or wrapped
// with errors.As around *ProviderFailure) and advances to the next provider
// in the Workflow's chain before retrying the Step.
type ProviderError interface {
	error
	// IsProviderError is the marker. Implementations always return true.
	IsProviderError() bool
}

// ProviderFailure is the canonical ProviderError implementation. Callers can
// construct one with NewProviderFailure or by satisfying ProviderError on
// their own error type.
type ProviderFailure struct {
	// Provider is the failing provider identifier (often a model name).
	Provider string
	// Err is the underlying transport or upstream error.
	Err error
}

// Error implements error.
func (p *ProviderFailure) Error() string {
	if p == nil {
		return "<nil provider failure>"
	}
	if p.Err == nil {
		return fmt.Sprintf("provider %q failed", p.Provider)
	}
	return fmt.Sprintf("provider %q failed: %v", p.Provider, p.Err)
}

// Unwrap exposes the underlying error for errors.Is/As.
func (p *ProviderFailure) Unwrap() error { return p.Err }

// IsProviderError marks ProviderFailure as a recoverable failover error.
func (p *ProviderFailure) IsProviderError() bool { return true }

// NewProviderFailure wraps err as a ProviderFailure attributed to provider.
func NewProviderFailure(provider string, err error) *ProviderFailure {
	return &ProviderFailure{Provider: provider, Err: err}
}

// isProviderError reports whether err is recoverable via provider failover.
func isProviderError(err error) bool {
	if err == nil {
		return false
	}
	var pe ProviderError
	if errors.As(err, &pe) {
		return pe.IsProviderError()
	}
	return false
}

// StepExecutor invokes a model (or other backend) for a single Step.
//
// The Engine calls Execute once per provider in the failover chain until a
// call returns nil error or the chain is exhausted. Implementations should
// return a ProviderError (typically *ProviderFailure) for recoverable
// upstream failures and a plain error for terminal failures that should not
// trigger failover.
type StepExecutor interface {
	Execute(ctx context.Context, provider string, step Step) (string, error)
}

// StepExecutorFunc adapts an ordinary function to StepExecutor.
type StepExecutorFunc func(ctx context.Context, provider string, step Step) (string, error)

// Execute implements StepExecutor.
func (f StepExecutorFunc) Execute(ctx context.Context, provider string, step Step) (string, error) {
	return f(ctx, provider, step)
}

// nowFunc is the clock used by Engine. Tests may override it for
// deterministic timestamps.
type nowFunc func() time.Time

// Engine drives a Workflow to completion, persisting a checkpoint after
// every Step and handling provider failover on recoverable errors.
type Engine struct {
	checkpointer Checkpointer
	executor     StepExecutor
	now          nowFunc
}

// NewEngine returns an Engine that uses checkpointer for durability and
// executor for Step invocations.
func NewEngine(checkpointer Checkpointer, executor StepExecutor) *Engine {
	if checkpointer == nil {
		panic("workflow: NewEngine requires a non-nil Checkpointer")
	}
	if executor == nil {
		panic("workflow: NewEngine requires a non-nil StepExecutor")
	}
	return &Engine{
		checkpointer: checkpointer,
		executor:     executor,
		now:          func() time.Time { return time.Now().UTC() },
	}
}

// SetClock overrides the Engine's time source. Intended for tests.
func (e *Engine) SetClock(now func() time.Time) {
	if now != nil {
		e.now = now
	}
}

// Start begins a new Run for wf. If runID is empty, a fresh ID is generated.
// The newly-created Run is checkpointed before any Step executes so that an
// immediate crash leaves a resumable artifact behind.
func (e *Engine) Start(ctx context.Context, wf Workflow, runID string) (*Run, error) {
	if runID == "" {
		generated, err := newRunID()
		if err != nil {
			return nil, fmt.Errorf("workflow: generate run id: %w", err)
		}
		runID = generated
	}
	now := e.now()
	run := &Run{
		ID:          runID,
		WorkflowID:  wf.ID,
		Workflow:    wf,
		State:       RunStatePending,
		CurrentStep: 0,
		Results:     []StepResult{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := e.checkpointer.Save(run); err != nil {
		return nil, fmt.Errorf("workflow: save initial checkpoint: %w", err)
	}
	return e.run(ctx, run)
}

// Resume reloads the Run identified by runID and continues execution from
// the next unfinished Step. A Run that is already Completed or Failed is
// returned as-is without further execution.
func (e *Engine) Resume(ctx context.Context, runID string) (*Run, error) {
	run, err := e.checkpointer.Load(runID)
	if err != nil {
		return nil, err
	}
	switch run.State {
	case RunStateCompleted, RunStateFailed:
		return run, nil
	}
	return e.run(ctx, run)
}

// run executes Steps from run.CurrentStep until completion, failure, or
// context cancellation. The Run is checkpointed after every Step.
func (e *Engine) run(ctx context.Context, run *Run) (*Run, error) {
	for run.CurrentStep < len(run.Workflow.Steps) {
		if err := ctx.Err(); err != nil {
			return run, err
		}

		step := run.Workflow.Steps[run.CurrentStep]
		started := e.now()
		output, provider, err := e.executeWithFailover(ctx, run.Workflow.Providers, step)
		completed := e.now()

		result := StepResult{
			StepID:      step.ID,
			Provider:    provider,
			Output:      output,
			StartedAt:   started,
			CompletedAt: completed,
		}

		if err != nil {
			result.Error = err.Error()
			run.Results = append(run.Results, result)
			run.State = RunStateFailed
			run.LastError = err.Error()
			run.UpdatedAt = completed
			if saveErr := e.checkpointer.Save(run); saveErr != nil {
				return run, fmt.Errorf("workflow: save failure checkpoint: %w (original: %v)", saveErr, err)
			}
			return run, err
		}

		run.Results = append(run.Results, result)
		run.CurrentStep++
		run.State = RunStateRunning
		run.UpdatedAt = completed
		if run.CurrentStep == len(run.Workflow.Steps) {
			run.State = RunStateCompleted
		}
		if saveErr := e.checkpointer.Save(run); saveErr != nil {
			return run, fmt.Errorf("workflow: save step checkpoint: %w", saveErr)
		}
	}

	if run.State == RunStatePending {
		// A Workflow with no Steps short-circuits to Completed.
		run.State = RunStateCompleted
		run.UpdatedAt = e.now()
		if err := e.checkpointer.Save(run); err != nil {
			return run, fmt.Errorf("workflow: save completion checkpoint: %w", err)
		}
	}
	return run, nil
}

// executeWithFailover invokes step against each provider in turn, advancing
// on ProviderError until success or chain exhaustion. With an empty chain,
// the executor is invoked once with provider="" and any error is terminal.
func (e *Engine) executeWithFailover(ctx context.Context, providers []string, step Step) (string, string, error) {
	if len(providers) == 0 {
		out, err := e.executor.Execute(ctx, "", step)
		return out, "", err
	}

	var lastErr error
	for _, provider := range providers {
		if err := ctx.Err(); err != nil {
			return "", provider, err
		}
		out, err := e.executor.Execute(ctx, provider, step)
		if err == nil {
			return out, provider, nil
		}
		if !isProviderError(err) {
			return "", provider, err
		}
		lastErr = err
	}
	return "", providers[len(providers)-1], fmt.Errorf("workflow: provider chain exhausted: %w", lastErr)
}

// newRunID returns a 128-bit hex-encoded random identifier. Stdlib only.
func newRunID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}
