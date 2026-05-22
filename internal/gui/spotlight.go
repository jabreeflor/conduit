package gui

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// Spotlight is the view-model for the Conduit Spotlight overlay (PRD §11.4).
//
// It is summoned with a global hotkey (default ⌥Space) and renders a centered
// floating panel with a single input field. As the user types, ranked results
// from recent commands, named workflows, and memory entries appear inline.
// Selecting a result either runs it inline (simple commands) or hands off to
// the TUI / GUI surface (complex tasks).
//
// Spotlight is safe for concurrent use: the OS hotkey thread, the input event
// loop, and the result-source goroutines all touch it.
type Spotlight struct {
	mu sync.RWMutex

	visible bool
	query   string
	cursor  int                // index of the highlighted result; -1 if none
	results []SpotlightResult  // sorted, capped by maxResults
	sources []SpotlightSource  // result providers, queried on every keystroke
	recents []SpotlightCommand // most-recent-first, capped by maxRecents
	maxRes  int
	maxRec  int
}

// SpotlightKind classifies a result so the renderer can pick an icon and
// decide whether to run inline or hand off.
type SpotlightKind int

const (
	KindCommand  SpotlightKind = iota // built-in slash command (run inline)
	KindWorkflow                      // named workflow (hand off to GUI/TUI)
	KindMemory                        // memory entry (hand off to memory browser)
	KindRecent                        // recent command from history
	KindSession                       // resume a prior session
)

// SpotlightResult is one row in the result list.
type SpotlightResult struct {
	ID       string        // stable identifier for the result
	Kind     SpotlightKind // category
	Title    string        // primary text shown in the row
	Subtitle string        // secondary text (file path, last-run time, …)
	Score    float64       // ranking score; higher = better match
}

// SpotlightCommand is a recently executed command kept for the recents list.
type SpotlightCommand struct {
	ID       string
	Title    string
	Subtitle string
	RunAt    time.Time
}

// SpotlightSource provides results from one origin (workflows, memory, etc.).
// Sources are polled synchronously on every keystroke; expensive providers
// must implement their own caching.
type SpotlightSource interface {
	// Name identifies the source (used for debugging only).
	Name() string
	// Search returns ranked results for the current query. An empty query
	// should return the source's "default" suggestions (e.g. pinned workflows).
	Search(query string) []SpotlightResult
}

// NewSpotlight creates a hidden Spotlight overlay with sensible defaults.
func NewSpotlight() *Spotlight {
	return &Spotlight{
		cursor: -1,
		maxRes: 10,
		maxRec: 25,
	}
}

// AddSource registers a result provider. Duplicate names are not deduplicated;
// callers are expected to register each source once at startup.
func (s *Spotlight) AddSource(src SpotlightSource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sources = append(s.sources, src)
}

// Show summons the overlay. The query is preserved across summons so users
// can re-open Spotlight and continue where they left off.
func (s *Spotlight) Show() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.visible = true
	// Re-rank with the existing query (sources may have changed).
	s.refreshLocked()
}

// Hide dismisses the overlay without changing the query or recents.
func (s *Spotlight) Hide() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.visible = false
}

// Toggle flips visibility — used as the global-hotkey handler.
func (s *Spotlight) Toggle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.visible = !s.visible
	if s.visible {
		s.refreshLocked()
	}
}

// Visible reports whether the overlay should be rendered.
func (s *Spotlight) Visible() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.visible
}

// SetQuery replaces the input text and refreshes the ranked result list.
func (s *Spotlight) SetQuery(q string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.query = q
	s.refreshLocked()
}

// Query returns the current input text.
func (s *Spotlight) Query() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.query
}

// Results returns a snapshot of the currently ranked results.
func (s *Spotlight) Results() []SpotlightResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]SpotlightResult, len(s.results))
	copy(out, s.results)
	return out
}

// Cursor returns the index of the highlighted result, or -1 if no results.
func (s *Spotlight) Cursor() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cursor
}

// MoveCursor steps the cursor by delta (typically ±1 from arrow keys).
// Wraps at the ends so users can cycle through the list.
func (s *Spotlight) MoveCursor(delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.results) == 0 {
		s.cursor = -1
		return
	}
	n := len(s.results)
	s.cursor = ((s.cursor+delta)%n + n) % n
}

// Selected returns the highlighted result, or nil if none.
func (s *Spotlight) Selected() *SpotlightResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cursor < 0 || s.cursor >= len(s.results) {
		return nil
	}
	r := s.results[s.cursor]
	return &r
}

// Activate records the selected result in the recents list and returns it
// so the caller can dispatch it. Returns nil if no result is highlighted.
// The overlay is hidden as a side-effect, matching Spotlight UX.
func (s *Spotlight) Activate() *SpotlightResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cursor < 0 || s.cursor >= len(s.results) {
		return nil
	}
	r := s.results[s.cursor]
	s.pushRecentLocked(SpotlightCommand{
		ID:       r.ID,
		Title:    r.Title,
		Subtitle: r.Subtitle,
		RunAt:    time.Now(),
	})
	s.visible = false
	return &r
}

// Recents returns the recent-command list, newest first.
func (s *Spotlight) Recents() []SpotlightCommand {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]SpotlightCommand, len(s.recents))
	copy(out, s.recents)
	return out
}

// pushRecentLocked inserts a command at the head of the recents list,
// deduplicating by ID. Must be called with s.mu held.
func (s *Spotlight) pushRecentLocked(c SpotlightCommand) {
	out := make([]SpotlightCommand, 0, len(s.recents)+1)
	out = append(out, c)
	for _, prev := range s.recents {
		if prev.ID == c.ID {
			continue
		}
		out = append(out, prev)
	}
	if len(out) > s.maxRec {
		out = out[:s.maxRec]
	}
	s.recents = out
}

// refreshLocked re-queries every source and rebuilds the ranked list.
// Must be called with s.mu held.
func (s *Spotlight) refreshLocked() {
	q := strings.TrimSpace(s.query)

	var all []SpotlightResult
	for _, src := range s.sources {
		all = append(all, src.Search(q)...)
	}

	// When the query is empty, surface recents above provider defaults.
	if q == "" {
		for _, c := range s.recents {
			all = append([]SpotlightResult{{
				ID:       c.ID,
				Kind:     KindRecent,
				Title:    c.Title,
				Subtitle: c.Subtitle,
				Score:    1e9, // pin to top
			}}, all...)
		}
	}

	// Stable sort by score descending; ties keep insertion order.
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].Score > all[j].Score
	})

	if len(all) > s.maxRes {
		all = all[:s.maxRes]
	}
	s.results = all

	// Reset the cursor when the result set changes.
	if len(s.results) == 0 {
		s.cursor = -1
	} else {
		s.cursor = 0
	}
}

// Score is a small helper for SpotlightSource implementations that want a
// uniform scoring rule. It returns a higher number when query is a prefix or
// substring of title; 0 means no match.
//
// The scoring rule is intentionally simple — providers with specialised
// ranking (e.g. embedding search over memory) should ignore Score entirely.
func Score(query, title string) float64 {
	if query == "" {
		return 1.0
	}
	q := strings.ToLower(query)
	t := strings.ToLower(title)
	switch {
	case t == q:
		return 100
	case strings.HasPrefix(t, q):
		return 50
	case strings.Contains(t, q):
		return 10
	default:
		return 0
	}
}
