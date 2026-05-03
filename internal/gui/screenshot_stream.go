package gui

import (
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// ScreenshotStep represents one computer-use step shown in the stream view.
// It mirrors the data emitted by computeruse.LogEntry so the GUI layer never
// imports the capture backend directly.
type ScreenshotStep struct {
	// StepID is the opaque identifier emitted by the capture layer.
	StepID string
	// Timestamp is when the step started.
	Timestamp time.Time
	// PrePath is the absolute path to the pre-action PNG. Empty if unavailable.
	PrePath string
	// PostPath is the absolute path to the post-action PNG. Empty if unavailable.
	PostPath string
	// Captured is true when both pre/post screenshots were successfully taken.
	Captured bool
	// DurationMS is the step wall-clock duration in milliseconds.
	DurationMS int64
	// Status is "success" or "error".
	Status string
	// Pinned marks the step for inclusion in the agent's context window.
	Pinned bool
}

// ActivePhase describes which screenshot is currently displayed.
type ActivePhase int

const (
	PhasePre  ActivePhase = iota // show the pre-action screenshot
	PhasePost                    // show the post-action screenshot
)

// ScreenshotStream is the view-model for the live computer-use screenshot
// stream shown in the GUI main content area when ViewScreenshot is active.
//
// It is safe for concurrent use: the capture backend and the UI event loop
// run on different goroutines.
type ScreenshotStream struct {
	mu sync.RWMutex

	steps      []*ScreenshotStep // ordered oldest → newest
	stepIndex  map[string]*ScreenshotStep
	activeStep string      // StepID of the currently displayed step
	phase      ActivePhase // which screenshot half is shown

	// maxSteps caps in-memory history. When exceeded, the oldest unpinned
	// step is evicted. 0 means unlimited.
	maxSteps int
}

// NewScreenshotStream creates an empty stream. maxSteps limits in-memory
// history; 0 is unlimited.
func NewScreenshotStream(maxSteps int) *ScreenshotStream {
	return &ScreenshotStream{
		stepIndex: make(map[string]*ScreenshotStep),
		maxSteps:  maxSteps,
		phase:     PhasePost, // default: show result screenshot
	}
}

// Push appends or updates a step. If a step with the same StepID already
// exists it is updated in place (e.g. the post-screenshot arrives after the
// pre was already emitted). Otherwise a new entry is appended.
//
// The most-recently pushed step becomes active automatically.
func (s *ScreenshotStream) Push(step ScreenshotStep) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.stepIndex[step.StepID]; ok {
		// Merge: update fields that have changed since the pre-screenshot.
		if step.PostPath != "" {
			existing.PostPath = step.PostPath
		}
		if step.PrePath != "" {
			existing.PrePath = step.PrePath
		}
		existing.Captured = step.Captured
		existing.DurationMS = step.DurationMS
		existing.Status = step.Status
	} else {
		cp := step
		s.steps = append(s.steps, &cp)
		s.stepIndex[step.StepID] = &cp
		s.evictIfNeeded()
	}

	s.activeStep = step.StepID
}

// Steps returns a snapshot of all current steps, oldest first.
func (s *ScreenshotStream) Steps() []ScreenshotStep {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]ScreenshotStep, len(s.steps))
	for i, p := range s.steps {
		out[i] = *p
	}
	return out
}

// ActiveStep returns the currently selected step or nil if the stream is empty.
func (s *ScreenshotStream) ActiveStep() *ScreenshotStep {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if p, ok := s.stepIndex[s.activeStep]; ok {
		cp := *p
		return &cp
	}
	return nil
}

// SelectStep makes stepID the active step.
func (s *ScreenshotStream) SelectStep(stepID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.stepIndex[stepID]; ok {
		s.activeStep = stepID
	}
}

// SelectPhase switches between the pre and post screenshot for the active step.
func (s *ScreenshotStream) SelectPhase(phase ActivePhase) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.phase = phase
}

// Phase returns the currently displayed phase.
func (s *ScreenshotStream) Phase() ActivePhase {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.phase
}

// ActiveImagePath returns the file path of the currently visible screenshot.
// Returns "" if the stream is empty or the image is unavailable.
func (s *ScreenshotStream) ActiveImagePath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	step, ok := s.stepIndex[s.activeStep]
	if !ok {
		return ""
	}
	switch s.phase {
	case PhasePre:
		return step.PrePath
	default:
		if step.PostPath != "" {
			return step.PostPath
		}
		return step.PrePath // fall back to pre if post not yet captured
	}
}

// Pin marks a step so it is kept in the in-memory list even when the max-step
// cap is hit, and signals to the context assembler that the image should be
// embedded in the agent's context window.
func (s *ScreenshotStream) Pin(stepID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.stepIndex[stepID]; ok {
		p.Pinned = true
	}
}

// Unpin removes the pin from a step.
func (s *ScreenshotStream) Unpin(stepID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.stepIndex[stepID]; ok {
		p.Pinned = false
	}
}

// PinnedPaths returns the image paths (post preferred, pre fallback) for all
// pinned steps in chronological order. This is the list the context assembler
// passes to the model as vision attachments.
func (s *ScreenshotStream) PinnedPaths() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var paths []string
	for _, step := range s.steps {
		if !step.Pinned {
			continue
		}
		p := step.PostPath
		if p == "" {
			p = step.PrePath
		}
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// PinnedSteps returns copies of all pinned steps in chronological order.
func (s *ScreenshotStream) PinnedSteps() []ScreenshotStep {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []ScreenshotStep
	for _, step := range s.steps {
		if step.Pinned {
			out = append(out, *step)
		}
	}
	return out
}

// NavigatePrev selects the step immediately before the active one.
// Does nothing if already at the oldest step.
func (s *ScreenshotStream) NavigatePrev() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.navigateBy(-1)
}

// NavigateNext selects the step immediately after the active one.
// Does nothing if already at the newest step.
func (s *ScreenshotStream) NavigateNext() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.navigateBy(+1)
}

// navigateBy moves the active step by delta positions in the slice.
// Must be called with s.mu held.
func (s *ScreenshotStream) navigateBy(delta int) {
	if len(s.steps) == 0 {
		return
	}
	idx := s.activeIndex()
	next := idx + delta
	if next < 0 || next >= len(s.steps) {
		return
	}
	s.activeStep = s.steps[next].StepID
}

// activeIndex returns the slice index of the active step, or the last index
// if not found. Must be called with s.mu held.
func (s *ScreenshotStream) activeIndex() int {
	for i, step := range s.steps {
		if step.StepID == s.activeStep {
			return i
		}
	}
	return len(s.steps) - 1
}

// Len returns the number of steps currently held in the stream.
func (s *ScreenshotStream) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.steps)
}

// Clear removes all unpinned steps from the stream. Pinned steps are kept.
func (s *ScreenshotStream) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	kept := s.steps[:0]
	for _, step := range s.steps {
		if step.Pinned {
			kept = append(kept, step)
		} else {
			delete(s.stepIndex, step.StepID)
		}
	}
	s.steps = kept

	// Reset active to the newest remaining step.
	if len(s.steps) > 0 {
		s.activeStep = s.steps[len(s.steps)-1].StepID
	} else {
		s.activeStep = ""
	}
}

// evictIfNeeded removes the oldest unpinned step when the cap is exceeded.
// Must be called with s.mu held.
func (s *ScreenshotStream) evictIfNeeded() {
	if s.maxSteps <= 0 || len(s.steps) <= s.maxSteps {
		return
	}
	// Find the oldest unpinned step.
	for i, step := range s.steps {
		if !step.Pinned {
			delete(s.stepIndex, step.StepID)
			s.steps = append(s.steps[:i], s.steps[i+1:]...)
			return
		}
	}
	// All steps are pinned — allow the list to grow beyond maxSteps.
}

// StepsByTimestamp returns a snapshot sorted by Timestamp (oldest first).
// Useful for renderers that need stable ordering independent of insertion order.
func (s *ScreenshotStream) StepsByTimestamp() []ScreenshotStep {
	steps := s.Steps()
	sort.Slice(steps, func(i, j int) bool {
		return steps[i].Timestamp.Before(steps[j].Timestamp)
	})
	return steps
}

// ThumbnailPath returns the path of the post-action screenshot for a given
// step, falling back to pre if post is absent. Returns "" if neither exists.
// This is the image a thumbnail strip would display.
func ThumbnailPath(step ScreenshotStep) string {
	if p := step.PostPath; p != "" {
		return p
	}
	return step.PrePath
}

// StepBasename returns just the file base name (without directory) of a
// screenshot path, suitable for use as a label in narrow strip views.
func StepBasename(path string) string {
	return filepath.Base(path)
}
