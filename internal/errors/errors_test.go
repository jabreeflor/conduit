package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestClassifyTypedErrorPassesThroughKind(t *testing.T) {
	for _, k := range []Kind{
		KindNetwork, KindTimeout, KindRateLimited, KindPermission,
		KindInvalidInput, KindModelUnavailable, KindUnknown,
	} {
		err := New(k, "some failure")
		if got := Classify(err); got != k {
			t.Errorf("Classify(New(%s)) = %s, want %s", k, got, k)
		}
	}
}

func TestClassifyWrappedErrorPreservesKind(t *testing.T) {
	inner := errors.New("connection refused")
	wrapped := Wrap(KindNetwork, inner)
	if got := Classify(wrapped); got != KindNetwork {
		t.Errorf("Classify(Wrap(network, err)) = %s, want network", got)
	}
}

func TestClassifyHeuristicsFromMessageSubstrings(t *testing.T) {
	cases := []struct {
		msg  string
		want Kind
	}{
		{"context deadline exceeded", KindTimeout},
		{"request timeout", KindTimeout},
		{"rate limit exceeded", KindRateLimited},
		{"too many requests: 429", KindRateLimited},
		{"permission denied: /etc/shadow", KindPermission},
		{"401 unauthorized", KindPermission},
		{"403 forbidden", KindPermission},
		{"connection refused", KindNetwork},
		{"no route to host", KindNetwork},
		{"model unavailable", KindModelUnavailable},
		{"model not found", KindModelUnavailable},
		{"invalid input: missing field", KindInvalidInput},
		{"bad request: 400", KindInvalidInput},
		{"unexpected failure", KindUnknown},
	}
	for _, c := range cases {
		err := fmt.Errorf("%s", c.msg)
		if got := Classify(err); got != c.want {
			t.Errorf("Classify(%q) = %s, want %s", c.msg, got, c.want)
		}
	}
}

func TestRecoveryForCoversAllKinds(t *testing.T) {
	table := map[Kind]Recovery{
		KindNetwork:          RecoveryExponentialBackoff,
		KindTimeout:          RecoveryRetryLongerThenSkip,
		KindRateLimited:      RecoveryBackoffOrFallback,
		KindPermission:       RecoveryEscalateToUser,
		KindInvalidInput:     RecoveryLogAndSkip,
		KindModelUnavailable: RecoveryNextFallback,
		KindUnknown:          RecoveryLogAndContinue,
	}
	for k, want := range table {
		if got := RecoveryFor(k); got != want {
			t.Errorf("RecoveryFor(%s) = %s, want %s", k, got, want)
		}
	}
}

func TestToolErrorRecoveryMatchesKind(t *testing.T) {
	err := New(KindRateLimited, "quota exceeded")
	if err.Recovery() != RecoveryBackoffOrFallback {
		t.Errorf("Recovery() = %s, want backoff_or_fallback", err.Recovery())
	}
}

func TestToolErrorUnwrapChain(t *testing.T) {
	sentinel := errors.New("root cause")
	wrapped := Wrap(KindNetwork, sentinel)
	if !errors.Is(wrapped, sentinel) {
		t.Error("errors.Is should find sentinel through Unwrap chain")
	}
}

func TestClassifyNilReturnsUnknown(t *testing.T) {
	if got := Classify(nil); got != KindUnknown {
		t.Errorf("Classify(nil) = %s, want unknown", got)
	}
}
