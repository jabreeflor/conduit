package guardrails

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEngine_BenignAllow(t *testing.T) {
	sink := &MemoryAuditSink{}
	e := New(WithAuditSink(sink), WithSessionID("sess-1"))

	dec, err := e.Evaluate(context.Background(), Action{Verb: "screenshot"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if dec.Verdict != VerdictAllow {
		t.Fatalf("expected allow, got %s", dec.Verdict)
	}

	entries := sink.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	if entries[0].Verdict != VerdictAllow {
		t.Fatalf("audit verdict = %s, want allow", entries[0].Verdict)
	}
	if entries[0].SessionID != "sess-1" {
		t.Fatalf("audit session id = %q, want sess-1", entries[0].SessionID)
	}
}

func TestEngine_FinancialDeniedByDefault(t *testing.T) {
	sink := &MemoryAuditSink{}
	// No confirmer supplied; engine should deny financial regardless.
	e := New(WithAuditSink(sink))

	dec, err := e.Evaluate(context.Background(), Action{
		Verb:   "click",
		URL:    "https://checkout.stripe.com/c/pay/abc",
		Target: "Pay $4500",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if dec.Verdict != VerdictDeny {
		t.Fatalf("expected deny on financial; got %s (reason=%s)", dec.Verdict, dec.Reason)
	}
	if len(dec.Categories) == 0 || dec.Categories[0] != CategoryFinancial {
		t.Fatalf("expected dominant Financial; got %v", dec.Categories)
	}
}

func TestEngine_CommunicationRequiresConfirmation_Granted(t *testing.T) {
	called := 0
	confirmer := ConfirmerFunc(func(ctx context.Context, a Action, reason string) (bool, error) {
		called++
		if !strings.Contains(reason, "communication") {
			t.Errorf("reason should include category; got %q", reason)
		}
		return true, nil
	})
	sink := &MemoryAuditSink{}
	e := New(WithAuditSink(sink), WithConfirmer(confirmer))

	dec, err := e.Evaluate(context.Background(), Action{
		Verb:     "click",
		BundleID: "com.apple.mail",
		Target:   "Send",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if dec.Verdict != VerdictAllow {
		t.Fatalf("verdict = %s, want allow after confirmation", dec.Verdict)
	}
	if !dec.Confirmed {
		t.Fatalf("expected Confirmed=true")
	}
	if called != 1 {
		t.Fatalf("confirmer called %d times, want 1", called)
	}
	if got := sink.Entries(); len(got) != 1 || !got[0].Confirmed {
		t.Fatalf("audit entries = %+v", got)
	}
}

func TestEngine_CommunicationRequiresConfirmation_Denied(t *testing.T) {
	confirmer := ConfirmerFunc(func(ctx context.Context, a Action, reason string) (bool, error) {
		return false, nil
	})
	sink := &MemoryAuditSink{}
	e := New(WithAuditSink(sink), WithConfirmer(confirmer))

	dec, err := e.Evaluate(context.Background(), Action{
		Verb:     "click",
		BundleID: "com.apple.mail",
		Target:   "Send",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if dec.Verdict != VerdictDeny {
		t.Fatalf("verdict = %s, want deny", dec.Verdict)
	}
	if dec.Confirmed {
		t.Fatalf("expected Confirmed=false")
	}
}

func TestEngine_DefaultDenyWithoutConfirmer(t *testing.T) {
	sink := &MemoryAuditSink{}
	e := New(WithAuditSink(sink)) // no confirmer

	dec, err := e.Evaluate(context.Background(), Action{
		Verb:     "click",
		BundleID: "com.apple.mail",
		Target:   "Send",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if dec.Verdict != VerdictDeny {
		t.Fatalf("verdict = %s, want deny when no confirmer registered", dec.Verdict)
	}
}

func TestEngine_ConfirmationTimeoutDenies(t *testing.T) {
	// Confirmer hangs forever — timeout must close the gap.
	confirmer := ConfirmerFunc(func(ctx context.Context, a Action, reason string) (bool, error) {
		<-ctx.Done()
		return false, ctx.Err()
	})
	sink := &MemoryAuditSink{}
	e := New(
		WithAuditSink(sink),
		WithConfirmer(confirmer),
		WithConfirmationTimeout(20*time.Millisecond),
	)

	dec, err := e.Evaluate(context.Background(), Action{
		Verb:     "click",
		BundleID: "com.apple.mail",
		Target:   "Send",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if dec.Verdict != VerdictDeny {
		t.Fatalf("verdict = %s, want deny on timeout", dec.Verdict)
	}
	if !strings.Contains(dec.Reason, "timeout") && !strings.Contains(dec.Reason, "canceled") {
		t.Fatalf("reason = %q, want timeout/canceled", dec.Reason)
	}
}

func TestEngine_ConfirmerError_Denies(t *testing.T) {
	wantErr := errors.New("confirmer broke")
	confirmer := ConfirmerFunc(func(ctx context.Context, a Action, reason string) (bool, error) {
		return false, wantErr
	})
	sink := &MemoryAuditSink{}
	e := New(WithAuditSink(sink), WithConfirmer(confirmer))

	dec, err := e.Evaluate(context.Background(), Action{
		Verb:     "click",
		BundleID: "com.apple.mail",
		Target:   "Send",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if dec.Verdict != VerdictDeny {
		t.Fatalf("verdict = %s, want deny on confirmer error", dec.Verdict)
	}
	if !strings.Contains(dec.Reason, "confirmer broke") {
		t.Fatalf("reason = %q, want confirmer error message", dec.Reason)
	}
}

func TestEngine_AuditFileJSONLPerSession(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "guardrails.jsonl")
	sink, err := NewFileAuditSink(path)
	if err != nil {
		t.Fatalf("sink: %v", err)
	}
	e := New(
		WithAuditSink(sink),
		WithSessionID("sess-42"),
		WithClock(func() time.Time { return time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC) }),
	)

	if _, err := e.Evaluate(context.Background(), Action{Verb: "screenshot"}); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if _, err := e.Evaluate(context.Background(), Action{
		Verb:   "click",
		URL:    "https://chase.com/transfer",
		Target: "Send Money",
	}); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), data)
	}
	for i, line := range lines {
		var entry AuditEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line %d invalid JSON: %v", i, err)
		}
		if entry.SessionID != "sess-42" {
			t.Errorf("line %d session id = %q", i, entry.SessionID)
		}
		if entry.Timestamp.IsZero() {
			t.Errorf("line %d missing timestamp", i)
		}
	}
}

func TestEngine_RequestConfirmation_ContextCancel(t *testing.T) {
	hung := make(chan struct{})
	confirmer := ConfirmerFunc(func(ctx context.Context, a Action, reason string) (bool, error) {
		<-ctx.Done()
		close(hung)
		return false, ctx.Err()
	})
	e := New(
		WithConfirmer(confirmer),
		WithConfirmationTimeout(time.Second),
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ok, err := e.RequestConfirmation(ctx, Action{Verb: "send"}, "test")
	if ok {
		t.Fatalf("expected denial on cancelled ctx")
	}
	if err == nil {
		t.Fatalf("expected error on cancelled ctx")
	}
	select {
	case <-hung:
		// goroutine cleaned up
	case <-time.After(time.Second):
		t.Fatalf("confirmer goroutine leaked")
	}
}

func TestPolicy_FinancialOverridesAllowDefault(t *testing.T) {
	p := DefaultPolicy()
	// Even if a misconfigured policy says allow for Financial, the
	// FinancialDefaultDeny safety floor stays disabled (intentionally:
	// explicit allow is explicit).
	p.Categories[CategoryFinancial] = VerdictAllow
	v, _ := p.verdictFor([]Category{CategoryFinancial})
	if v != VerdictAllow {
		t.Fatalf("explicit allow should not be upgraded; got %s", v)
	}

	// But require_confirmation upgrades to deny under the safety floor.
	p2 := DefaultPolicy()
	p2.Categories[CategoryFinancial] = VerdictRequireConfirmation
	v2, _ := p2.verdictFor([]Category{CategoryFinancial})
	if v2 != VerdictDeny {
		t.Fatalf("financial require_confirmation should upgrade to deny; got %s", v2)
	}
}
