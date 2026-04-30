// Package errors defines typed tool and model failure categories with
// corresponding recovery strategies for the Conduit engine.
package errors

import (
	"errors"
	"strings"
)

// Kind classifies a tool or model failure for recovery routing.
type Kind string

const (
	KindNetwork          Kind = "network"
	KindTimeout          Kind = "timeout"
	KindRateLimited      Kind = "rate_limited"
	KindPermission       Kind = "permission"
	KindInvalidInput     Kind = "invalid_input"
	KindModelUnavailable Kind = "model_unavailable"
	KindUnknown          Kind = "unknown"
)

// Recovery describes the action the engine should take after a classified failure.
type Recovery string

const (
	// RecoveryExponentialBackoff retries up to 3 times with exponential delay.
	RecoveryExponentialBackoff Recovery = "exponential_backoff"
	// RecoveryRetryLongerThenSkip retries once with an extended timeout, then skips.
	RecoveryRetryLongerThenSkip Recovery = "retry_longer_then_skip"
	// RecoveryBackoffOrFallback waits for retry-after, then falls back to a secondary model.
	RecoveryBackoffOrFallback Recovery = "backoff_or_fallback"
	// RecoveryEscalateToUser halts and surfaces the error to the user for approval.
	RecoveryEscalateToUser Recovery = "escalate_to_user"
	// RecoveryLogAndSkip records the failure and skips the call without retrying.
	RecoveryLogAndSkip Recovery = "log_and_skip"
	// RecoveryNextFallback switches to the next available model in the fallback chain.
	RecoveryNextFallback Recovery = "next_fallback"
	// RecoveryLogAndContinue records the failure and continues the session.
	RecoveryLogAndContinue Recovery = "log_and_continue"
)

// recoveryTable maps each failure kind to its recovery action.
var recoveryTable = map[Kind]Recovery{
	KindNetwork:          RecoveryExponentialBackoff,
	KindTimeout:          RecoveryRetryLongerThenSkip,
	KindRateLimited:      RecoveryBackoffOrFallback,
	KindPermission:       RecoveryEscalateToUser,
	KindInvalidInput:     RecoveryLogAndSkip,
	KindModelUnavailable: RecoveryNextFallback,
	KindUnknown:          RecoveryLogAndContinue,
}

// RecoveryFor returns the recovery strategy for a given failure kind.
func RecoveryFor(k Kind) Recovery {
	if r, ok := recoveryTable[k]; ok {
		return r
	}
	return RecoveryLogAndContinue
}

// ToolError is a typed failure that wraps an underlying error and carries its
// classification so the engine can route recovery without re-inspecting the error.
type ToolError struct {
	kind Kind
	err  error
}

// New creates a typed ToolError with the given kind and message.
func New(k Kind, msg string) *ToolError {
	return &ToolError{kind: k, err: errors.New(msg)}
}

// Wrap classifies an existing error and wraps it in a ToolError.
func Wrap(k Kind, err error) *ToolError {
	return &ToolError{kind: k, err: err}
}

// Kind returns the failure classification.
func (e *ToolError) Kind() Kind { return e.kind }

// Recovery returns the prescribed recovery action for this error.
func (e *ToolError) Recovery() Recovery { return RecoveryFor(e.kind) }

// Error satisfies the error interface.
func (e *ToolError) Error() string { return e.err.Error() }

// Unwrap exposes the inner error for errors.Is/As chains.
func (e *ToolError) Unwrap() error { return e.err }

// Classify inspects an error and returns its Kind. If the error is already a
// ToolError its kind is returned directly; otherwise heuristics on the error
// message are applied.
func Classify(err error) Kind {
	if err == nil {
		return KindUnknown
	}

	var te *ToolError
	if errors.As(err, &te) {
		return te.kind
	}

	msg := strings.ToLower(err.Error())

	switch {
	case containsAny(msg, "timeout", "deadline exceeded", "context deadline"):
		return KindTimeout
	case containsAny(msg, "rate limit", "rate_limit", "too many requests", "429"):
		return KindRateLimited
	case containsAny(msg, "permission denied", "unauthorized", "forbidden", "403", "401"):
		return KindPermission
	case containsAny(msg, "connection refused", "no route to host", "network", "dial", "i/o timeout"):
		return KindNetwork
	case containsAny(msg, "model unavailable", "model not found", "overloaded", "529"):
		return KindModelUnavailable
	case containsAny(msg, "invalid input", "invalid argument", "bad request", "400", "validation"):
		return KindInvalidInput
	default:
		return KindUnknown
	}
}

func containsAny(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
