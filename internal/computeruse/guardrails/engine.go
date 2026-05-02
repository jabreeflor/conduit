package guardrails

import (
	"context"
	"errors"
	"time"
)

// DefaultConfirmationTimeout bounds how long the engine waits for the host
// to deliver a confirmation answer. On timeout the engine returns
// VerdictDeny so we never silently execute a destructive action because the
// user wandered away from the terminal.
const DefaultConfirmationTimeout = 60 * time.Second

// Engine is the safety guardrail layer. The MCP step dispatcher (issue #37)
// MUST call (*Engine).Evaluate on every proposed action before executing it.
//
// TODO(issue #37, issue #40): wire Engine.Evaluate into the MCP dispatcher
// pre-step hook. The dispatcher should treat (Decision.Verdict ==
// VerdictAllow) as the only green-light state — every other verdict means
// "do not call the OS for this step".
type Engine struct {
	classifier *Classifier
	policy     Policy
	confirmer  Confirmer
	audit      AuditSink
	sessionID  string
	timeout    time.Duration
	now        func() time.Time
}

// Option configures an Engine.
type Option func(*Engine)

// WithClassifier sets a custom classifier. If unset, DefaultClassifier()
// is used.
func WithClassifier(c *Classifier) Option {
	return func(e *Engine) {
		if c != nil {
			e.classifier = c
		}
	}
}

// WithPolicy overrides the default policy.
func WithPolicy(p Policy) Option {
	return func(e *Engine) {
		if p.Categories == nil {
			p.Categories = map[Category]Verdict{}
		}
		if p.Default == "" {
			p.Default = VerdictAllow
		}
		e.policy = p
	}
}

// WithConfirmer registers the host-side confirmation hook.
func WithConfirmer(c Confirmer) Option {
	return func(e *Engine) {
		if c != nil {
			e.confirmer = c
		}
	}
}

// WithAuditSink registers a JSONL audit writer.
func WithAuditSink(s AuditSink) Option {
	return func(e *Engine) {
		if s != nil {
			e.audit = s
		}
	}
}

// WithSessionID stamps every audit entry with the given session id (matches
// the per-session JSONL convention from PR #15).
func WithSessionID(id string) Option {
	return func(e *Engine) {
		e.sessionID = id
	}
}

// WithConfirmationTimeout bounds the host-side confirmation wait.
func WithConfirmationTimeout(d time.Duration) Option {
	return func(e *Engine) {
		if d > 0 {
			e.timeout = d
		}
	}
}

// WithClock injects a clock for tests.
func WithClock(now func() time.Time) Option {
	return func(e *Engine) {
		if now != nil {
			e.now = now
		}
	}
}

// New constructs an Engine with the supplied options. Sensible defaults are
// applied for any omitted option, INCLUDING fail-closed behaviour: if no
// confirmer is supplied, the engine treats every "require_confirmation"
// outcome as a denial.
func New(opts ...Option) *Engine {
	e := &Engine{
		classifier: DefaultClassifier(),
		policy:     DefaultPolicy(),
		confirmer:  denyConfirmer{},
		audit:      nopAuditSink{},
		timeout:    DefaultConfirmationTimeout,
		now:        func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Evaluate is the synchronous decision API. It runs in three phases:
//
//  1. Classify the action (no I/O).
//  2. Resolve the policy verdict for the dominant category.
//  3. If require_confirmation, call the host Confirmer with a bounded
//     timeout. Any error or timeout is treated as a denial.
//
// The final decision is written to the audit sink before return.
//
// The MCP dispatcher MUST treat anything other than VerdictAllow as
// "do not execute the underlying OS action".
func (e *Engine) Evaluate(ctx context.Context, action Action) (Decision, error) {
	cats := e.classifier.Classify(action)
	verdict, dominant := e.policy.verdictFor(cats)
	reason := reasonFor(verdict, dominant, action)

	dec := Decision{
		Verdict:    verdict,
		Categories: cats,
		Reason:     reason,
	}

	if verdict == VerdictRequireConfirmation {
		confirmed, confirmErr := e.runConfirmation(ctx, action, reason)
		dec.MatchedRule = "policy:require_confirmation"
		dec.Confirmed = confirmed
		if !confirmed {
			dec.Verdict = VerdictDeny
			if confirmErr != nil && !errors.Is(confirmErr, context.Canceled) &&
				!errors.Is(confirmErr, context.DeadlineExceeded) {
				dec.Reason = reason + " confirmation_error=" + confirmErr.Error()
			} else if confirmErr != nil {
				dec.Reason = reason + " confirmation=timeout_or_canceled"
			} else {
				dec.Reason = reason + " confirmation=denied"
			}
		} else {
			dec.Verdict = VerdictAllow
			dec.Reason = reason + " confirmation=granted"
		}
		e.writeAudit(action, dec)
		return dec, nil
	}

	if verdict == VerdictDeny {
		dec.MatchedRule = "policy:deny"
	} else if dominant != CategoryUnknown {
		dec.MatchedRule = "policy:allow:" + string(dominant)
	}
	e.writeAudit(action, dec)
	return dec, nil
}

// RequestConfirmation is the public confirmation hook documented in the
// PRD §6.8 brief. It exposes the engine's bounded-timeout machinery to
// callers that already know they need consent (e.g. checkpoint flows that
// don't run through Evaluate). Default-deny on timeout.
func (e *Engine) RequestConfirmation(ctx context.Context, action Action, reason string) (bool, error) {
	return e.runConfirmation(ctx, action, reason)
}

func (e *Engine) runConfirmation(ctx context.Context, action Action, reason string) (bool, error) {
	if e.confirmer == nil {
		return false, nil
	}
	if e.timeout <= 0 {
		return e.confirmer.Confirm(ctx, action, reason)
	}
	cctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	type result struct {
		ok  bool
		err error
	}
	ch := make(chan result, 1)
	go func() {
		ok, err := e.confirmer.Confirm(cctx, action, reason)
		ch <- result{ok: ok, err: err}
	}()

	select {
	case r := <-ch:
		return r.ok, r.err
	case <-cctx.Done():
		return false, cctx.Err()
	}
}

func (e *Engine) writeAudit(a Action, d Decision) {
	if e.audit == nil {
		return
	}
	cats := make([]string, len(d.Categories))
	for i, c := range d.Categories {
		cats[i] = string(c)
	}
	entry := AuditEntry{
		Timestamp:   e.now(),
		SessionID:   e.sessionID,
		Verdict:     d.Verdict,
		Confirmed:   d.Confirmed,
		Categories:  cats,
		MatchedRule: d.MatchedRule,
		Reason:      d.Reason,
		Verb:        a.Verb,
		BundleID:    a.BundleID,
		AppName:     a.AppName,
		URLHost:     hostFromURL(a.URL),
		Target:      a.Target,
		Path:        a.Path,
		Description: a.Description,
	}
	_ = e.audit.Write(entry)
}
