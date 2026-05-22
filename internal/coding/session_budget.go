package coding

import (
	"errors"
	"fmt"
	"sync"
)

// ErrBudgetExceeded is the sentinel returned by SessionBudget methods when any
// hard limit has been reached. Callers should treat this as a clean abort
// rather than a provider error.
var ErrBudgetExceeded = errors.New("session budget exceeded")

// SessionLimits holds the optional hard caps for a coding session (PRD §6.24.8).
// A nil pointer means "no limit". All limits are checked at the start of the
// operation they gate; exceeding one aborts the current run cleanly.
type SessionLimits struct {
	// MaxTotalTokens caps cumulative prompt + completion tokens for the run.
	MaxTotalTokens *int
	// MaxInputTokensPerCall caps the estimated input tokens passed per model call.
	MaxInputTokensPerCall *int
	// MaxOutputTokensPerCall caps the estimated output tokens received per model call.
	MaxOutputTokensPerCall *int
	// MaxReasoningTokensPerCall caps reasoning tokens per model call (provider-reported).
	MaxReasoningTokensPerCall *int
	// MaxBudgetUSD aborts the run when the estimated USD cost exceeds the threshold.
	MaxBudgetUSD *float64
	// MaxToolCalls is a hard cap on tool invocations per run.
	MaxToolCalls *int
	// MaxModelCalls is a hard cap on provider API calls per run.
	MaxModelCalls *int
	// MaxSessionTurns caps the total turns across resumed sessions.
	MaxSessionTurns *int
	// MaxDelegatedTasks limits nested agent spawning depth.
	MaxDelegatedTasks *int
}

// SessionBudgetSnapshot is a point-in-time read of SessionBudget counters.
// Feeds the Eval Framework (PRD §6.23) cost-efficiency scoring.
type SessionBudgetSnapshot struct {
	TotalInputTokens     int
	TotalOutputTokens    int
	TotalReasoningTokens int
	EstimatedCostUSD     float64
	ToolCallCount        int
	ModelCallCount       int
	SessionTurnCount     int
	DelegatedTaskCount   int
	Limits               SessionLimits
}

// SessionBudget tracks per-run usage against the limits in SessionLimits.
// All methods are safe for concurrent use from the REPL loop.
type SessionBudget struct {
	mu sync.Mutex

	limits SessionLimits

	totalInputTokens     int
	totalOutputTokens    int
	totalReasoningTokens int
	estimatedCostUSD     float64
	toolCallCount        int
	modelCallCount       int
	sessionTurnCount     int
	delegatedTaskCount   int
}

// NewSessionBudget returns a SessionBudget enforcing the given limits.
// Nil-pointer fields in limits mean "no limit" for that dimension.
func NewSessionBudget(limits SessionLimits) *SessionBudget {
	return &SessionBudget{limits: limits}
}

// RecordModelCall increments the model-call counter and checks MaxModelCalls.
// Returns ErrBudgetExceeded if the limit would be exceeded.
func (b *SessionBudget) RecordModelCall() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.limits.MaxModelCalls != nil && b.modelCallCount >= *b.limits.MaxModelCalls {
		return fmt.Errorf("%w: model calls (%d) reached limit (%d)",
			ErrBudgetExceeded, b.modelCallCount, *b.limits.MaxModelCalls)
	}
	b.modelCallCount++
	return nil
}

// RecordTokens accumulates input, output, and reasoning tokens for the turn
// and checks all token-based limits. costUSD should reflect the provider's
// reported cost for this call; pass 0 if cost tracking is not yet wired.
func (b *SessionBudget) RecordTokens(inputTokens, outputTokens, reasoningTokens int, costUSD float64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Per-call limits are checked against the incoming values (not cumulative)
	// so they act as per-turn guardrails.
	if b.limits.MaxInputTokensPerCall != nil && inputTokens > *b.limits.MaxInputTokensPerCall {
		return fmt.Errorf("%w: input tokens per call (%d) exceeded limit (%d)",
			ErrBudgetExceeded, inputTokens, *b.limits.MaxInputTokensPerCall)
	}
	if b.limits.MaxOutputTokensPerCall != nil && outputTokens > *b.limits.MaxOutputTokensPerCall {
		return fmt.Errorf("%w: output tokens per call (%d) exceeded limit (%d)",
			ErrBudgetExceeded, outputTokens, *b.limits.MaxOutputTokensPerCall)
	}
	if b.limits.MaxReasoningTokensPerCall != nil && reasoningTokens > *b.limits.MaxReasoningTokensPerCall {
		return fmt.Errorf("%w: reasoning tokens per call (%d) exceeded limit (%d)",
			ErrBudgetExceeded, reasoningTokens, *b.limits.MaxReasoningTokensPerCall)
	}

	b.totalInputTokens += inputTokens
	b.totalOutputTokens += outputTokens
	b.totalReasoningTokens += reasoningTokens
	b.estimatedCostUSD += costUSD

	total := b.totalInputTokens + b.totalOutputTokens
	if b.limits.MaxTotalTokens != nil && total > *b.limits.MaxTotalTokens {
		return fmt.Errorf("%w: total tokens (%d) exceeded limit (%d)",
			ErrBudgetExceeded, total, *b.limits.MaxTotalTokens)
	}
	if b.limits.MaxBudgetUSD != nil && b.estimatedCostUSD > *b.limits.MaxBudgetUSD {
		return fmt.Errorf("%w: estimated cost ($%.4f) exceeded limit ($%.4f)",
			ErrBudgetExceeded, b.estimatedCostUSD, *b.limits.MaxBudgetUSD)
	}
	return nil
}

// RecordToolCall increments the tool-call counter and checks MaxToolCalls.
// Returns ErrBudgetExceeded if the limit would be exceeded.
func (b *SessionBudget) RecordToolCall() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.limits.MaxToolCalls != nil && b.toolCallCount >= *b.limits.MaxToolCalls {
		return fmt.Errorf("%w: tool calls (%d) reached limit (%d)",
			ErrBudgetExceeded, b.toolCallCount, *b.limits.MaxToolCalls)
	}
	b.toolCallCount++
	return nil
}

// RecordTurn increments the session-turn counter and checks MaxSessionTurns.
// Returns ErrBudgetExceeded if the limit would be exceeded.
func (b *SessionBudget) RecordTurn() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.limits.MaxSessionTurns != nil && b.sessionTurnCount >= *b.limits.MaxSessionTurns {
		return fmt.Errorf("%w: session turns (%d) reached limit (%d)",
			ErrBudgetExceeded, b.sessionTurnCount, *b.limits.MaxSessionTurns)
	}
	b.sessionTurnCount++
	return nil
}

// RecordDelegatedTask increments the delegated-task counter and checks
// MaxDelegatedTasks. Returns ErrBudgetExceeded if the limit would be exceeded.
func (b *SessionBudget) RecordDelegatedTask() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.limits.MaxDelegatedTasks != nil && b.delegatedTaskCount >= *b.limits.MaxDelegatedTasks {
		return fmt.Errorf("%w: delegated tasks (%d) reached limit (%d)",
			ErrBudgetExceeded, b.delegatedTaskCount, *b.limits.MaxDelegatedTasks)
	}
	b.delegatedTaskCount++
	return nil
}

// Snapshot returns a consistent read of current usage counters plus the
// configured limits. The snapshot is suitable for emitting to the eval
// scorecard (PRD §6.23) or the GUI settings panel.
func (b *SessionBudget) Snapshot() SessionBudgetSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return SessionBudgetSnapshot{
		TotalInputTokens:     b.totalInputTokens,
		TotalOutputTokens:    b.totalOutputTokens,
		TotalReasoningTokens: b.totalReasoningTokens,
		EstimatedCostUSD:     b.estimatedCostUSD,
		ToolCallCount:        b.toolCallCount,
		ModelCallCount:       b.modelCallCount,
		SessionTurnCount:     b.sessionTurnCount,
		DelegatedTaskCount:   b.delegatedTaskCount,
		Limits:               b.limits,
	}
}

// intPtr returns a pointer to a copy of v, or nil if v <= 0.
// Used by CLI wiring to convert flag values into optional limits.
func IntPtr(v int) *int {
	if v <= 0 {
		return nil
	}
	cp := v
	return &cp
}

// float64Ptr returns a pointer to a copy of v, or nil if v <= 0.
func Float64Ptr(v float64) *float64 {
	if v <= 0 {
		return nil
	}
	cp := v
	return &cp
}
