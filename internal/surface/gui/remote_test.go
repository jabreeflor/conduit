package gui

import (
	"errors"
	"testing"
	"time"
)

func TestRemoteConnection_initialState(t *testing.T) {
	r := NewRemoteConnection("ws://localhost:7777", nil)
	if r.State() != StateDisconnected {
		t.Fatalf("initial state: want disconnected, got %s", r.State())
	}
	if r.Attempts() != 0 {
		t.Fatalf("initial attempts: want 0, got %d", r.Attempts())
	}
	if r.URL() != "ws://localhost:7777" {
		t.Fatalf("URL: %q", r.URL())
	}
}

func TestRemoteConnection_happyPathLifecycle(t *testing.T) {
	r := NewRemoteConnection("ws://x", nil)
	r.MarkConnecting()
	if r.State() != StateConnecting {
		t.Fatalf("after MarkConnecting: %s", r.State())
	}
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	r.MarkConnected(now)
	if r.State() != StateConnected {
		t.Fatalf("after MarkConnected: %s", r.State())
	}
	if !r.LastUp().Equal(now) {
		t.Fatalf("LastUp: want %v, got %v", now, r.LastUp())
	}
	if r.Attempts() != 0 {
		t.Fatal("Attempts should reset on MarkConnected")
	}
}

func TestRemoteConnection_disconnectSchedulesReconnect(t *testing.T) {
	r := NewRemoteConnection("ws://x", &ExponentialBackoff{
		Base: 100 * time.Millisecond,
		Cap:  time.Second,
		// no jitter for deterministic test
	})
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	r.MarkConnecting()
	r.MarkConnected(now)

	delay := r.MarkDisconnected(errors.New("conn reset"), now)
	if r.State() != StateReconnecting {
		t.Fatalf("after MarkDisconnected: want Reconnecting, got %s", r.State())
	}
	if r.Attempts() != 1 {
		t.Fatalf("Attempts: want 1, got %d", r.Attempts())
	}
	if delay <= 0 {
		t.Fatalf("delay should be positive, got %v", delay)
	}
	if r.LastError() == nil {
		t.Fatal("LastError should record the disconnect cause")
	}
	if !r.NextAttemptAt().Equal(now.Add(delay)) {
		t.Fatalf("NextAttemptAt: want %v, got %v", now.Add(delay), r.NextAttemptAt())
	}
}

func TestRemoteConnection_giveUpAfterMaxAttempts(t *testing.T) {
	r := NewRemoteConnection("ws://x", &ExponentialBackoff{
		Base:        10 * time.Millisecond,
		Cap:         time.Second,
		MaxAttempts: 2,
	})
	now := time.Now()
	if d := r.MarkDisconnected(errors.New("a"), now); d == 0 {
		t.Fatalf("attempt 1 should schedule retry, got %v", d)
	}
	if r.State() != StateReconnecting {
		t.Fatalf("after attempt 1: %s", r.State())
	}
	if d := r.MarkDisconnected(errors.New("b"), now); d != 0 {
		t.Fatalf("attempt 2 should give up (return 0), got %v", d)
	}
	if r.State() != StateFailed {
		t.Fatalf("expected StateFailed after MaxAttempts, got %s", r.State())
	}
}

func TestRemoteConnection_userDisconnectClearsTimer(t *testing.T) {
	r := NewRemoteConnection("ws://x", nil)
	r.MarkDisconnected(errors.New("network"), time.Now()) // schedules reconnect
	r.Disconnect()
	if r.State() != StateDisconnected {
		t.Fatalf("after Disconnect: %s", r.State())
	}
	if !r.NextAttemptAt().IsZero() {
		t.Fatal("Disconnect should clear NextAttemptAt")
	}
	if r.Attempts() != 0 {
		t.Fatalf("Disconnect should reset Attempts, got %d", r.Attempts())
	}
}

func TestRemoteConnection_setURLResetsState(t *testing.T) {
	r := NewRemoteConnection("ws://a", nil)
	r.MarkConnecting()
	r.SetURL("ws://b")
	if r.URL() != "ws://b" {
		t.Fatalf("URL: %q", r.URL())
	}
	if r.State() != StateDisconnected {
		t.Fatalf("SetURL should drop to Disconnected, got %s", r.State())
	}
}

func TestExponentialBackoff_growsAndCaps(t *testing.T) {
	b := &ExponentialBackoff{
		Base: 100 * time.Millisecond,
		Cap:  800 * time.Millisecond,
	}
	cases := []struct {
		attempt int
		min     time.Duration
		max     time.Duration
	}{
		{0, 100 * time.Millisecond, 100 * time.Millisecond},
		{1, 200 * time.Millisecond, 200 * time.Millisecond},
		{2, 400 * time.Millisecond, 400 * time.Millisecond},
		{3, 800 * time.Millisecond, 800 * time.Millisecond}, // capped
		{10, 800 * time.Millisecond, 800 * time.Millisecond},
	}
	for _, tc := range cases {
		got := b.Delay(tc.attempt)
		if got < tc.min || got > tc.max {
			t.Errorf("Delay(%d) = %v, want in [%v, %v]", tc.attempt, got, tc.min, tc.max)
		}
	}
}

func TestExponentialBackoff_jitterStaysWithinBounds(t *testing.T) {
	b := &ExponentialBackoff{
		Base:   100 * time.Millisecond,
		Cap:    time.Second,
		Jitter: 50 * time.Millisecond,
	}
	// Deterministic source so the test isn't flaky.
	calls := 0
	b.SetRandomSource(func() int64 {
		calls++
		return int64(calls) * 7
	})
	for i := 0; i < 10; i++ {
		d := b.Delay(0)
		if d < 100*time.Millisecond || d >= 150*time.Millisecond {
			t.Errorf("Delay with 50ms jitter on Base=100ms must be in [100,150)ms, got %v", d)
		}
	}
}

func TestExponentialBackoff_giveUpRespectsMax(t *testing.T) {
	b := &ExponentialBackoff{MaxAttempts: 0}
	if b.GiveUp(1000) {
		t.Fatal("MaxAttempts=0 means retry forever")
	}
	b.MaxAttempts = 3
	if b.GiveUp(2) {
		t.Fatal("attempt < max should not give up")
	}
	if !b.GiveUp(3) {
		t.Fatal("attempt >= max should give up")
	}
}

func TestConnState_string(t *testing.T) {
	for _, s := range []ConnState{
		StateDisconnected, StateConnecting, StateConnected, StateReconnecting, StateFailed,
	} {
		if s.String() == "unknown" {
			t.Errorf("state %d should have a name", s)
		}
	}
}
