// Package delegation provides nested agent orchestration for conduit.
// Agents can spawn subagents for parallel or sequential task execution,
// enabling complex multi-step workflows to be broken into focused subtasks.
package delegation

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// SubagentSpec describes a task to be delegated to a subagent.
type SubagentSpec struct {
	// Name is a human-readable identifier for this subtask.
	Name string
	// Prompt is the task instruction sent to the subagent.
	Prompt string
	// Model overrides the parent's model for this subagent. Empty means inherit.
	Model string
	// MaxTokens caps token usage for this subagent. Zero means no limit.
	MaxTokens int
	// Tools restricts the subagent to a named subset of available tools.
	Tools []string
	// Timeout caps execution time. Zero means no timeout.
	Timeout time.Duration
}

// SubagentResult holds the outcome of a single delegated subagent run.
type SubagentResult struct {
	// Name matches SubagentSpec.Name.
	Name string
	// Output is the subagent's final text response.
	Output string
	// Error is non-nil if the subagent failed or timed out.
	Error error
	// TokensUsed is the number of tokens consumed, if available.
	TokensUsed int
	// Duration is wall-clock time the subagent ran.
	Duration time.Duration
}

// Runner executes a single SubagentSpec and returns its result.
// Implementations may call a real model API, a stub, or a sandboxed process.
type Runner interface {
	Run(ctx context.Context, spec SubagentSpec) SubagentResult
}

// Orchestrator manages concurrent and sequential subagent execution.
type Orchestrator struct {
	Runner         Runner
	MaxConcurrency int // 0 means unbounded
}

// NewOrchestrator returns an Orchestrator with the given runner.
// maxConcurrency controls how many subagents run in parallel; 0 = unbounded.
func NewOrchestrator(runner Runner, maxConcurrency int) *Orchestrator {
	return &Orchestrator{Runner: runner, MaxConcurrency: maxConcurrency}
}

// Run executes all specs concurrently (up to MaxConcurrency at a time) and
// returns results in the same order as the input specs.
func (o *Orchestrator) Run(ctx context.Context, specs []SubagentSpec) ([]SubagentResult, error) {
	if len(specs) == 0 {
		return nil, nil
	}

	results := make([]SubagentResult, len(specs))
	sem := make(chan struct{}, o.concurrencyLimit(len(specs)))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for i, spec := range specs {
		wg.Add(1)
		go func(idx int, s SubagentSpec) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			runCtx := ctx
			if s.Timeout > 0 {
				var cancel context.CancelFunc
				runCtx, cancel = context.WithTimeout(ctx, s.Timeout)
				defer cancel()
			}

			res := o.Runner.Run(runCtx, s)
			results[idx] = res

			if res.Error != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("subagent %q: %w", s.Name, res.Error)
				}
				mu.Unlock()
			}
		}(i, spec)
	}

	wg.Wait()
	return results, firstErr
}

// RunSequential executes specs one at a time in order, stopping on the first
// error.
func (o *Orchestrator) RunSequential(ctx context.Context, specs []SubagentSpec) ([]SubagentResult, error) {
	results := make([]SubagentResult, 0, len(specs))
	for _, spec := range specs {
		if ctx.Err() != nil {
			return results, ctx.Err()
		}
		runCtx := ctx
		if spec.Timeout > 0 {
			var cancel context.CancelFunc
			runCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
			defer cancel()
		}
		res := o.Runner.Run(runCtx, spec)
		results = append(results, res)
		if res.Error != nil {
			return results, fmt.Errorf("subagent %q: %w", spec.Name, res.Error)
		}
	}
	return results, nil
}

// Aggregate combines multiple SubagentResult outputs into a single string,
// prefixing each with a header that identifies the subagent by name.
func (o *Orchestrator) Aggregate(results []SubagentResult) string {
	var sb strings.Builder
	for i, r := range results {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("## ")
		sb.WriteString(r.Name)
		sb.WriteString("\n")
		if r.Error != nil {
			sb.WriteString("**Error:** ")
			sb.WriteString(r.Error.Error())
		} else {
			sb.WriteString(r.Output)
		}
		if r.Duration > 0 {
			sb.WriteString(fmt.Sprintf("\n\n_(%s, %d tokens)_", r.Duration.Round(time.Millisecond), r.TokensUsed))
		}
	}
	return sb.String()
}

// SpawnSubagentParams is the JSON-serialisable parameter block for the
// spawn_subagent virtual tool.
type SpawnSubagentParams struct {
	Name    string `json:"name"`
	Prompt  string `json:"prompt"`
	Model   string `json:"model,omitempty"`
	Timeout string `json:"timeout,omitempty"` // e.g. "30s", "2m"
}

// ParseTimeout converts the string timeout field to a time.Duration.
// Empty or invalid strings default to 5 minutes.
func ParseTimeout(s string) time.Duration {
	if s == "" {
		return 5 * time.Minute
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}

// concurrencyLimit returns the effective concurrency cap.
func (o *Orchestrator) concurrencyLimit(n int) int {
	if o.MaxConcurrency <= 0 || o.MaxConcurrency > n {
		return n
	}
	return o.MaxConcurrency
}
