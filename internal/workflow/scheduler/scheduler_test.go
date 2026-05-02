package scheduler

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakeClock is a controllable Clock used in tests. Goroutines that call
// After(d) block until the test advances time past their wake instant.
type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	pending []*fakeTimer
}

type fakeTimer struct {
	when time.Time
	ch   chan time.Time
}

func newFakeClock(t time.Time) *fakeClock { return &fakeClock{now: t} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	timer := &fakeTimer{when: c.now.Add(d), ch: ch}
	if d <= 0 {
		ch <- c.now
		return ch
	}
	c.pending = append(c.pending, timer)
	return ch
}

// Advance moves the clock forward by d, firing any timers whose deadline is
// now reached.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	now := c.now
	remaining := c.pending[:0]
	for _, t := range c.pending {
		if !t.when.After(now) {
			t.ch <- now
		} else {
			remaining = append(remaining, t)
		}
	}
	c.pending = remaining
	c.mu.Unlock()
}

func TestScheduler_AddListRemove(t *testing.T) {
	t.Parallel()
	clock := newFakeClock(mustTime(t, "2025-05-02T10:00:00Z"))
	s, err := New(Options{
		Clock:   clock,
		Trigger: func(context.Context, string) {},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	id, err := s.Add("wf-1", "*/5 * * * *")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if id == "" {
		t.Fatalf("expected non-empty id")
	}
	got := s.List()
	if len(got) != 1 {
		t.Fatalf("expected 1 schedule, got %d", len(got))
	}
	want := mustTime(t, "2025-05-02T10:05:00Z")
	if !got[0].NextFire.Equal(want) {
		t.Fatalf("NextFire = %s, want %s", got[0].NextFire, want)
	}
	if got[0].WorkflowID != "wf-1" {
		t.Fatalf("WorkflowID = %q", got[0].WorkflowID)
	}
	if !got[0].Enabled {
		t.Fatalf("expected enabled by default")
	}

	if err := s.Remove(id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(s.List()) != 0 {
		t.Fatalf("expected empty after remove")
	}
	// Removing unknown id is a no-op.
	if err := s.Remove("nonexistent"); err != nil {
		t.Fatalf("Remove(unknown): %v", err)
	}
}

func TestScheduler_AddRejectsBadExpression(t *testing.T) {
	t.Parallel()
	s, err := New(Options{Trigger: func(context.Context, string) {}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := s.Add("wf", "not a cron"); err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestScheduler_FiresAtRightTime(t *testing.T) {
	t.Parallel()
	clock := newFakeClock(mustTime(t, "2025-05-02T10:00:00Z"))
	type fired struct {
		at time.Time
		id string
	}
	var (
		mu    sync.Mutex
		hits  []fired
		ready = make(chan struct{}, 8)
	)
	s, err := New(Options{
		Clock: clock,
		Trigger: func(_ context.Context, workflowID string) {
			mu.Lock()
			hits = append(hits, fired{at: clock.Now(), id: workflowID})
			mu.Unlock()
			ready <- struct{}{}
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := s.Add("wf-five", "*/5 * * * *"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan struct{})
	go func() {
		_ = s.Run(ctx)
		close(done)
	}()

	// Advance to the first firing instant (10:05).
	clock.Advance(5 * time.Minute)
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatalf("first fire timed out")
	}
	// And again to 10:10.
	clock.Advance(5 * time.Minute)
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatalf("second fire timed out")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(hits) != 2 {
		t.Fatalf("expected 2 fires, got %d", len(hits))
	}
	if hits[0].id != "wf-five" || hits[1].id != "wf-five" {
		t.Fatalf("workflow ids: %+v", hits)
	}
	if !hits[0].at.Equal(mustTime(t, "2025-05-02T10:05:00Z")) {
		t.Fatalf("first fire at %s", hits[0].at)
	}
	if !hits[1].at.Equal(mustTime(t, "2025-05-02T10:10:00Z")) {
		t.Fatalf("second fire at %s", hits[1].at)
	}
}

func TestScheduler_DisabledScheduleDoesNotFire(t *testing.T) {
	t.Parallel()
	clock := newFakeClock(mustTime(t, "2025-05-02T10:00:00Z"))
	var fires int
	var mu sync.Mutex
	s, err := New(Options{
		Clock: clock,
		Trigger: func(context.Context, string) {
			mu.Lock()
			fires++
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	id, err := s.Add("wf", "* * * * *")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.SetEnabled(id, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = s.Run(ctx) }()

	clock.Advance(10 * time.Minute)
	// Give the goroutine a moment to (not) fire.
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if fires != 0 {
		t.Fatalf("expected 0 fires while disabled, got %d", fires)
	}
}

func TestStore_RoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "schedules.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	// Loading a missing file returns no entries and no error.
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load (missing): %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}

	want := []Schedule{
		{ID: "abc", WorkflowID: "wf-1", Expression: "@hourly", NextFire: mustTime(t, "2025-05-02T11:00:00Z"), Enabled: true},
		{ID: "def", WorkflowID: "wf-2", Expression: "*/15 * * * *", NextFire: mustTime(t, "2025-05-02T10:15:00Z"), Enabled: false},
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err = store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("count = %d, want %d", len(got), len(want))
	}
	for i, e := range got {
		if e.ID != want[i].ID || e.WorkflowID != want[i].WorkflowID || e.Expression != want[i].Expression {
			t.Errorf("entry %d mismatch: got %+v, want %+v", i, e, want[i])
		}
		if !e.NextFire.Equal(want[i].NextFire) {
			t.Errorf("entry %d NextFire = %s, want %s", i, e.NextFire, want[i].NextFire)
		}
		if e.Enabled != want[i].Enabled {
			t.Errorf("entry %d Enabled = %v, want %v", i, e.Enabled, want[i].Enabled)
		}
	}
}

func TestStore_AtomicOverwrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "schedules.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	for i := 0; i < 3; i++ {
		entries := []Schedule{{ID: "a", WorkflowID: "wf", Expression: "@daily", NextFire: time.Now().UTC(), Enabled: true}}
		if err := store.Save(entries); err != nil {
			t.Fatalf("Save iter %d: %v", i, err)
		}
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry after repeated writes, got %d", len(got))
	}
}

func TestScheduler_PersistsAcrossRestart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "schedules.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	clock := newFakeClock(mustTime(t, "2025-05-02T10:00:00Z"))
	s, err := New(Options{Clock: clock, Store: store, Trigger: func(context.Context, string) {}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	id, err := s.Add("wf", "0 12 * * *")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Simulate restart: build a new scheduler using the same store.
	clock2 := newFakeClock(mustTime(t, "2025-05-02T11:00:00Z"))
	s2, err := New(Options{Clock: clock2, Store: store, Trigger: func(context.Context, string) {}})
	if err != nil {
		t.Fatalf("New (restart): %v", err)
	}
	got := s2.List()
	if len(got) != 1 {
		t.Fatalf("expected 1 schedule after restart, got %d", len(got))
	}
	if got[0].ID != id {
		t.Fatalf("ID = %q, want %q", got[0].ID, id)
	}
	want := mustTime(t, "2025-05-02T12:00:00Z")
	if !got[0].NextFire.Equal(want) {
		t.Fatalf("NextFire after restart = %s, want %s", got[0].NextFire, want)
	}
}
