package gui

import (
	"errors"
	"sync"
	"time"
)

// ConnState is the lifecycle state of the GUI's connection to the Conduit
// core (PRD §11.2 + §17). The GUI reads this every render to decide whether
// to show the live status pill, the spinning reconnect indicator, or the
// "core unreachable" banner.
type ConnState int

const (
	StateDisconnected ConnState = iota // never connected, or user-initiated disconnect
	StateConnecting                    // first-attempt dial in flight
	StateConnected                     // WebSocket open, push pump healthy
	StateReconnecting                  // last connection dropped; backoff timer running
	StateFailed                        // gave up after MaxAttempts (if configured)
)

// String renders the state for status-bar / log output.
func (s ConnState) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateReconnecting:
		return "reconnecting"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// RemoteConnection is the view-model for the GUI ↔ core WebSocket. It holds
// the URL, the current ConnState, error history, ping latency, and the
// next-attempt deadline that drives the reconnect timer.
//
// Scope: the view-model OWNS the lifecycle state and the timer math. It does
// NOT own the actual WebSocket — the Tauri shell (or a future Go-side dial
// helper) calls Mark*ed methods to drive the state machine. This split
// keeps the Rust layer free of timer logic and matches ADR-003's "windowing
// host only" rule.
//
// Safe for concurrent use — both the dial goroutine and the GUI render
// loop touch this struct.
type RemoteConnection struct {
	mu sync.RWMutex

	url      string
	state    ConnState
	attempts int
	lastErr  error
	lastUp   time.Time
	lastDown time.Time
	pingMs   int

	policy BackoffPolicy
	nextAt time.Time
}

// BackoffPolicy decides how long to wait before the (attempt+1)th dial.
// attempt counts disconnect-triggered retries; 0 is the first reconnect
// after a healthy connection drops.
//
// The default policy is jittered exponential capped at 30s, which works
// well for typical home/office networks. Implementations that want
// constant-interval or aggressive-then-give-up can plug in here.
type BackoffPolicy interface {
	Delay(attempt int) time.Duration
	GiveUp(attempt int) bool
}

// ExponentialBackoff is the default BackoffPolicy.
//
// Delay grows as Base * 2^attempt, capped at Cap. Jitter (0..Jitter)
// is added on every call to avoid thundering-herd reconnects when many
// clients dropped at the same time.
//
// MaxAttempts of 0 means retry forever; a positive value caps retries
// and transitions the state machine to StateFailed.
type ExponentialBackoff struct {
	Base        time.Duration
	Cap         time.Duration
	Jitter      time.Duration
	MaxAttempts int

	// rand is injected for deterministic tests via SetRandomSource.
	rand func() int64
}

// NewExponentialBackoff returns a sensible default for production: 500 ms
// base, 30 s cap, 250 ms jitter, no attempt cap.
func NewExponentialBackoff() *ExponentialBackoff {
	return &ExponentialBackoff{
		Base:   500 * time.Millisecond,
		Cap:    30 * time.Second,
		Jitter: 250 * time.Millisecond,
	}
}

// SetRandomSource overrides the jitter source. Tests pass a deterministic
// generator; production leaves the default time-based source.
func (e *ExponentialBackoff) SetRandomSource(fn func() int64) { e.rand = fn }

// Delay returns the backoff for the next attempt.
func (e *ExponentialBackoff) Delay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	d := e.Base
	for i := 0; i < attempt && d < e.Cap; i++ {
		d *= 2
	}
	if d > e.Cap {
		d = e.Cap
	}
	if e.Jitter > 0 {
		var r int64
		if e.rand != nil {
			r = e.rand()
		} else {
			r = time.Now().UnixNano()
		}
		jitter := time.Duration(r%int64(e.Jitter)+int64(e.Jitter)) % e.Jitter
		d += jitter
	}
	return d
}

// GiveUp reports whether the policy has exhausted retries.
func (e *ExponentialBackoff) GiveUp(attempt int) bool {
	return e.MaxAttempts > 0 && attempt >= e.MaxAttempts
}

// NewRemoteConnection returns a disconnected view-model bound to url with
// the default exponential backoff. Pass policy=nil to keep the default.
func NewRemoteConnection(url string, policy BackoffPolicy) *RemoteConnection {
	if policy == nil {
		policy = NewExponentialBackoff()
	}
	return &RemoteConnection{
		url:    url,
		state:  StateDisconnected,
		policy: policy,
	}
}

// URL returns the configured server URL.
func (r *RemoteConnection) URL() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.url
}

// SetURL changes the target endpoint. Drops the connection state to
// Disconnected so the next StartDial uses the new URL.
func (r *RemoteConnection) SetURL(url string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.url = url
	r.state = StateDisconnected
	r.attempts = 0
	r.nextAt = time.Time{}
}

// State returns a snapshot of the current ConnState.
func (r *RemoteConnection) State() ConnState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state
}

// Attempts returns the number of reconnect attempts since the last
// successful connection. Resets to 0 on MarkConnected.
func (r *RemoteConnection) Attempts() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.attempts
}

// LastError returns the most recent dial or push-pump error, or nil.
func (r *RemoteConnection) LastError() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastErr
}

// LastUp returns the timestamp of the most recent successful connection,
// or the zero value if never connected.
func (r *RemoteConnection) LastUp() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastUp
}

// PingMs returns the most recent ping latency reported via SetPing.
func (r *RemoteConnection) PingMs() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.pingMs
}

// SetPing records a fresh ping observation (in milliseconds).
func (r *RemoteConnection) SetPing(ms int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pingMs = ms
}

// NextAttemptAt returns the deadline when the next dial should fire,
// or the zero value if no attempt is scheduled.
func (r *RemoteConnection) NextAttemptAt() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.nextAt
}

// MarkConnecting transitions to StateConnecting and clears the last error.
// Called by the dial goroutine immediately before opening the WebSocket.
func (r *RemoteConnection) MarkConnecting() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = StateConnecting
	r.lastErr = nil
	r.nextAt = time.Time{}
}

// MarkConnected transitions to StateConnected and resets the attempt
// counter. Called once the WebSocket handshake succeeds.
func (r *RemoteConnection) MarkConnected(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = StateConnected
	r.attempts = 0
	r.lastUp = now
	r.lastErr = nil
	r.nextAt = time.Time{}
}

// MarkDisconnected records a connection drop and either schedules the
// next reconnect or transitions to StateFailed when the policy gives up.
// Returns the scheduled delay (zero when giving up).
func (r *RemoteConnection) MarkDisconnected(err error, now time.Time) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastDown = now
	r.lastErr = err
	r.attempts++

	if r.policy.GiveUp(r.attempts) {
		r.state = StateFailed
		r.nextAt = time.Time{}
		return 0
	}
	d := r.policy.Delay(r.attempts)
	r.state = StateReconnecting
	r.nextAt = now.Add(d)
	return d
}

// Disconnect marks the connection closed by user action. The state
// machine transitions to StateDisconnected and no reconnect is scheduled.
func (r *RemoteConnection) Disconnect() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = StateDisconnected
	r.attempts = 0
	r.lastErr = nil
	r.nextAt = time.Time{}
}

// ErrNoURL is returned when a dial transition is requested before SetURL.
var ErrNoURL = errors.New("gui: no remote URL configured")
