package credential

import (
	"sync"
	"time"
)

// DefaultBackoff is used when New is called with backoff ≤ 0.
const DefaultBackoff = 30 * time.Second

// Pool holds a round-robin set of API keys for one provider.
// Concurrent calls to Next and MarkUnhealthy are safe.
type Pool struct {
	mu      sync.Mutex
	entries []entry
	cursor  int
	backoff time.Duration
}

type entry struct {
	key            string
	unhealthyUntil time.Time
}

// New creates a Pool from keys with the given backoff window.
// Pass 0 to use DefaultBackoff.
func New(keys []string, backoff time.Duration) *Pool {
	if backoff <= 0 {
		backoff = DefaultBackoff
	}
	entries := make([]entry, len(keys))
	for i, k := range keys {
		entries[i] = entry{key: k}
	}
	return &Pool{entries: entries, backoff: backoff}
}

// Next returns the next healthy key via round-robin.
// Returns ("", false) when every key is inside its backoff window.
func (p *Pool) Next() (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	n := len(p.entries)
	for range n {
		e := &p.entries[p.cursor%n]
		p.cursor++
		if now.After(e.unhealthyUntil) {
			return e.key, true
		}
	}
	return "", false
}

// MarkUnhealthy suspends key for the pool's backoff window.
// Calls with an unrecognised key are silently ignored.
func (p *Pool) MarkUnhealthy(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	until := time.Now().Add(p.backoff)
	for i := range p.entries {
		if p.entries[i].key == key {
			p.entries[i].unhealthyUntil = until
			return
		}
	}
}

// Len returns the total key count regardless of health state.
func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.entries)
}

// HealthyCount returns the number of keys currently available for use.
func (p *Pool) HealthyCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	n := 0
	for i := range p.entries {
		if now.After(p.entries[i].unhealthyUntil) {
			n++
		}
	}
	return n
}
