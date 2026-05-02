package scheduler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Clock abstracts the time source so tests can drive the scheduler
// deterministically without sleeping.
type Clock interface {
	Now() time.Time
	// After returns a channel that receives the time after at least d has
	// elapsed. The implementation must not block the caller.
	After(d time.Duration) <-chan time.Time
}

// SystemClock is the production clock backed by the time package.
type SystemClock struct{}

// Now reports the current wall-clock time in UTC.
func (SystemClock) Now() time.Time { return time.Now().UTC() }

// After delegates to time.After.
func (SystemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// TriggerFunc is invoked when a schedule fires. It is called from the
// scheduler's run goroutine; implementations should hand work off promptly to
// avoid stalling other timers.
type TriggerFunc func(ctx context.Context, workflowID string)

// Schedule is the user-visible representation of a configured cron entry.
//
// The struct is intentionally JSON-friendly so TUI and GUI surfaces can
// render it directly without an additional adapter.
type Schedule struct {
	ID         string    `json:"id"`
	WorkflowID string    `json:"workflow_id"`
	Expression string    `json:"expression"`
	NextFire   time.Time `json:"next_fire"`
	Enabled    bool      `json:"enabled"`
}

// internalSchedule is the in-memory state kept alongside the public view.
type internalSchedule struct {
	Schedule
	parsed *cronSchedule
}

// Scheduler runs cron entries and invokes a callback when they fire.
//
// The zero value is not usable; construct via New.
type Scheduler struct {
	mu        sync.Mutex
	clock     Clock
	store     *Store
	trigger   TriggerFunc
	schedules map[string]*internalSchedule
	// wake is signalled when the next-fire ordering may have changed
	// (Add/Remove/Enable). The Run loop selects on it to recompute.
	wake chan struct{}
}

// Options configures a new Scheduler.
type Options struct {
	// Clock is the time source. Defaults to SystemClock.
	Clock Clock
	// Store persists schedules across restarts. May be nil for ephemeral use.
	Store *Store
	// Trigger receives schedule fires. Required.
	Trigger TriggerFunc
}

// New constructs a Scheduler. If opts.Store is set, persisted schedules are
// loaded immediately so callers can List() before Run() returns.
func New(opts Options) (*Scheduler, error) {
	if opts.Trigger == nil {
		return nil, errors.New("scheduler: Trigger callback is required")
	}
	clock := opts.Clock
	if clock == nil {
		clock = SystemClock{}
	}
	s := &Scheduler{
		clock:     clock,
		store:     opts.Store,
		trigger:   opts.Trigger,
		schedules: map[string]*internalSchedule{},
		wake:      make(chan struct{}, 1),
	}
	if opts.Store != nil {
		entries, err := opts.Store.Load()
		if err != nil {
			return nil, fmt.Errorf("scheduler: load store: %w", err)
		}
		for _, e := range entries {
			parsed, err := parseExpression(e.Expression)
			if err != nil {
				// Skip corrupt entries rather than failing startup; surfaces
				// can re-add or correct them.
				continue
			}
			next, err := parsed.next(clock.Now())
			if err != nil {
				continue
			}
			e.NextFire = next
			s.schedules[e.ID] = &internalSchedule{Schedule: e, parsed: parsed}
		}
	}
	return s, nil
}

// Add registers a new cron entry and returns the assigned schedule ID.
func (s *Scheduler) Add(workflowID, expression string) (string, error) {
	parsed, err := parseExpression(expression)
	if err != nil {
		return "", err
	}
	now := s.clock.Now()
	next, err := parsed.next(now)
	if err != nil {
		return "", err
	}
	id, err := newID()
	if err != nil {
		return "", err
	}
	entry := &internalSchedule{
		Schedule: Schedule{
			ID:         id,
			WorkflowID: workflowID,
			Expression: expression,
			NextFire:   next,
			Enabled:    true,
		},
		parsed: parsed,
	}
	s.mu.Lock()
	s.schedules[id] = entry
	s.mu.Unlock()
	if err := s.persist(); err != nil {
		return "", err
	}
	s.kick()
	return id, nil
}

// Remove deletes a schedule by ID. Removing an unknown ID is a no-op.
func (s *Scheduler) Remove(scheduleID string) error {
	s.mu.Lock()
	_, ok := s.schedules[scheduleID]
	if ok {
		delete(s.schedules, scheduleID)
	}
	s.mu.Unlock()
	if !ok {
		return nil
	}
	if err := s.persist(); err != nil {
		return err
	}
	s.kick()
	return nil
}

// SetEnabled toggles whether a schedule is active. Disabled schedules remain
// listed and persist their next-fire time, but Run does not invoke Trigger.
func (s *Scheduler) SetEnabled(scheduleID string, enabled bool) error {
	s.mu.Lock()
	entry, ok := s.schedules[scheduleID]
	if ok {
		entry.Enabled = enabled
	}
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("scheduler: unknown schedule %q", scheduleID)
	}
	if err := s.persist(); err != nil {
		return err
	}
	s.kick()
	return nil
}

// List returns a snapshot of every schedule sorted by next-fire time. The
// result is safe for surfaces to mutate.
func (s *Scheduler) List() []Schedule {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Schedule, 0, len(s.schedules))
	for _, entry := range s.schedules {
		out = append(out, entry.Schedule)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].NextFire.Equal(out[j].NextFire) {
			return out[i].ID < out[j].ID
		}
		return out[i].NextFire.Before(out[j].NextFire)
	})
	return out
}

// Run drives the scheduler until ctx is cancelled. It blocks; callers
// typically launch it in a goroutine.
func (s *Scheduler) Run(ctx context.Context) error {
	for {
		entry, wait := s.nextDue()
		if entry == nil {
			// Nothing scheduled; wait for an Add or cancellation.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-s.wake:
				continue
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.wake:
			// Schedule set changed; recompute.
			continue
		case <-s.clock.After(wait):
			s.fire(ctx, entry.ID)
		}
	}
}

// nextDue returns the entry that fires soonest along with the wait duration
// from now. It returns (nil, 0) when no enabled schedules exist.
func (s *Scheduler) nextDue() (*internalSchedule, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var best *internalSchedule
	for _, entry := range s.schedules {
		if !entry.Enabled {
			continue
		}
		if best == nil || entry.NextFire.Before(best.NextFire) {
			best = entry
		}
	}
	if best == nil {
		return nil, 0
	}
	wait := best.NextFire.Sub(s.clock.Now())
	if wait < 0 {
		wait = 0
	}
	return best, wait
}

// fire invokes the trigger for the given ID and advances its next-fire time.
func (s *Scheduler) fire(ctx context.Context, id string) {
	s.mu.Lock()
	entry, ok := s.schedules[id]
	if !ok || !entry.Enabled {
		s.mu.Unlock()
		return
	}
	workflowID := entry.WorkflowID
	// Advance using the firing time as the reference so we never re-fire the
	// same minute even if the trigger callback runs long.
	fired := entry.NextFire
	next, err := entry.parsed.next(fired)
	if err == nil {
		entry.NextFire = next
	}
	s.mu.Unlock()

	s.trigger(ctx, workflowID)
	_ = s.persist()
	_ = fired
}

// persist writes the current schedule set to the configured store, if any.
func (s *Scheduler) persist() error {
	if s.store == nil {
		return nil
	}
	s.mu.Lock()
	out := make([]Schedule, 0, len(s.schedules))
	for _, entry := range s.schedules {
		out = append(out, entry.Schedule)
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return s.store.Save(out)
}

// kick wakes the Run loop if it is currently sleeping.
func (s *Scheduler) kick() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// newID returns a 16-hex-character random schedule identifier.
func newID() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}
