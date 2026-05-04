// Package recorder captures the screen and produces a stream of input
// events that the editor's auto-zoom, click-highlight, scroll-smoothing
// and Tango-style step generator turn into polished demos.
//
// PRD §13.2. Up to 4K 60fps; webcam PiP with background blur and auto-
// framing; AI-narrated voiceover for silent recordings.
//
// The capture backend is platform-specific and out of scope here; this
// package owns the input-event stream, zoom planner, and step extractor
// so the rest of the editor stays platform-independent.
package recorder

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// Resolution describes a recording target.
type Resolution struct {
	Width, Height int
	FPS           int
}

// Common presets.
var (
	Res4K60    = Resolution{Width: 3840, Height: 2160, FPS: 60}
	Res1080p60 = Resolution{Width: 1920, Height: 1080, FPS: 60}
	Res1080p30 = Resolution{Width: 1920, Height: 1080, FPS: 30}
)

// Settings configures a recording session.
type Settings struct {
	Resolution      Resolution
	IncludeAudio    bool
	IncludeWebcam   bool
	BlurBackground  bool // webcam background blur
	AutoFraming     bool // webcam auto-framing
	WindowFocusOnly bool // crop to currently focused window
	SilentMode      bool // generate AI voiceover later
}

// EventKind names a captured input event.
type EventKind string

const (
	EventClick     EventKind = "click"
	EventKeystroke EventKind = "keystroke"
	EventScroll    EventKind = "scroll"
	EventFocus     EventKind = "focus" // window focus change
	EventCursor    EventKind = "cursor"
)

// Event is one captured input event.
type Event struct {
	Kind    EventKind
	At      time.Duration
	X, Y    int    // cursor or click position
	Window  string // app or window title for focus events
	Scroll  int    // scroll delta in lines (positive = down)
	KeyText string // typed text for keystroke events
}

// Recorder owns the in-memory event stream and metadata for one session.
type Recorder struct {
	settings  Settings
	events    []Event
	started   bool
	startAt   time.Time
	stoppedAt time.Time
	now       func() time.Time
}

// New constructs a Recorder. Pass a clock for deterministic tests; nil
// uses time.Now.
func New(s Settings, clock func() time.Time) (*Recorder, error) {
	if s.Resolution.Width <= 0 || s.Resolution.Height <= 0 || s.Resolution.FPS <= 0 {
		return nil, errors.New("recorder: invalid resolution")
	}
	if clock == nil {
		clock = time.Now
	}
	return &Recorder{settings: s, now: clock}, nil
}

// Start begins a session. Subsequent calls return an error.
func (r *Recorder) Start() error {
	if r.started {
		return errors.New("recorder: already started")
	}
	r.started = true
	r.startAt = r.now()
	return nil
}

// Stop ends a session.
func (r *Recorder) Stop() error {
	if !r.started {
		return errors.New("recorder: not started")
	}
	r.stoppedAt = r.now()
	r.started = false
	return nil
}

// Record adds a captured event. The capture backend calls this.
func (r *Recorder) Record(e Event) error {
	if !r.started {
		return errors.New("recorder: not started")
	}
	if e.At == 0 {
		e.At = r.now().Sub(r.startAt)
	}
	r.events = append(r.events, e)
	return nil
}

// Events returns a copy of the recorded events.
func (r *Recorder) Events() []Event {
	out := make([]Event, len(r.events))
	copy(out, r.events)
	return out
}

// Duration returns the recording's wall-clock duration.
func (r *Recorder) Duration() time.Duration {
	if r.startAt.IsZero() {
		return 0
	}
	end := r.stoppedAt
	if end.IsZero() {
		end = r.now()
	}
	return end.Sub(r.startAt)
}

// --- post-processing -------------------------------------------------------

// ZoomPlan is a sequence of zoom-pan keyframes the editor injects into
// the EDL. Each entry centers the view on (X,Y) at the given Scale and
// holds for Hold; transitions between entries are interpolated.
type ZoomKeyframe struct {
	At    time.Duration
	X, Y  int
	Scale float64
	Hold  time.Duration
}

// PlanZoom builds a zoom plan from the recorded events. Clicks and
// keystrokes anchor zoom-in moments; idle stretches and scrolls return
// to fit. The default heuristics are conservative; callers can tune via
// PlanZoomOpts.
type PlanZoomOpts struct {
	IdleZoomOut time.Duration // zoom out after this much inactivity
	ClickScale  float64       // scale at click anchors
	HoldOnClick time.Duration // hold duration on each click anchor
}

// DefaultPlanZoomOpts returns the recommended defaults.
func DefaultPlanZoomOpts() PlanZoomOpts {
	return PlanZoomOpts{
		IdleZoomOut: 2 * time.Second,
		ClickScale:  1.6,
		HoldOnClick: 800 * time.Millisecond,
	}
}

// PlanZoom turns an event slice into zoom keyframes.
func PlanZoom(events []Event, opts PlanZoomOpts) []ZoomKeyframe {
	if opts.ClickScale == 0 {
		opts = DefaultPlanZoomOpts()
	}
	var out []ZoomKeyframe
	var lastActive time.Duration
	for _, e := range events {
		switch e.Kind {
		case EventClick, EventKeystroke:
			out = append(out, ZoomKeyframe{
				At:    e.At,
				X:     e.X,
				Y:     e.Y,
				Scale: opts.ClickScale,
				Hold:  opts.HoldOnClick,
			})
			lastActive = e.At
		case EventScroll, EventCursor, EventFocus:
			if e.At-lastActive > opts.IdleZoomOut {
				out = append(out, ZoomKeyframe{At: e.At, Scale: 1.0})
				lastActive = e.At
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out
}

// ClickHighlights builds a list of (At, X, Y) tuples the editor renders
// as Tango-style click rings.
type ClickHighlight struct {
	At   time.Duration
	X, Y int
}

// CollectClickHighlights filters events down to clicks.
func CollectClickHighlights(events []Event) []ClickHighlight {
	var out []ClickHighlight
	for _, e := range events {
		if e.Kind == EventClick {
			out = append(out, ClickHighlight{At: e.At, X: e.X, Y: e.Y})
		}
	}
	return out
}

// SmoothScroll returns interpolated cursor positions for the editor's
// scroll-smoothing pass. Raw scroll events become a stream of intermediate
// positions evenly spaced by Step.
func SmoothScroll(events []Event, step time.Duration) []Event {
	if step <= 0 {
		step = 16 * time.Millisecond // ~60fps
	}
	var out []Event
	for i := 0; i < len(events); i++ {
		out = append(out, events[i])
		if i+1 >= len(events) {
			continue
		}
		a, b := events[i], events[i+1]
		if a.Kind != EventScroll || b.Kind != EventScroll {
			continue
		}
		gap := b.At - a.At
		if gap <= step {
			continue
		}
		steps := int(gap / step)
		for s := 1; s < steps; s++ {
			t := a.At + time.Duration(s)*step
			frac := float64(s) / float64(steps)
			out = append(out, Event{
				Kind:   EventScroll,
				At:     t,
				X:      lerp(a.X, b.X, frac),
				Y:      lerp(a.Y, b.Y, frac),
				Scroll: int(float64(b.Scroll-a.Scroll) * frac),
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out
}

func lerp(a, b int, t float64) int {
	return a + int(float64(b-a)*t)
}

// Step is one Tango-style step in an auto-annotated walkthrough.
type Step struct {
	Index       int
	At          time.Duration
	Description string
	Window      string
}

// ExtractSteps groups events into walkthrough steps. New steps start on
// focus changes and on each click that is more than `gap` after the
// previous one.
func ExtractSteps(events []Event, gap time.Duration) []Step {
	if gap <= 0 {
		gap = 1500 * time.Millisecond
	}
	var steps []Step
	var lastClick time.Duration = -1
	currentWindow := ""
	idx := 0
	for _, e := range events {
		switch e.Kind {
		case EventFocus:
			currentWindow = e.Window
			idx++
			steps = append(steps, Step{
				Index:       idx,
				At:          e.At,
				Description: "Switched to " + e.Window,
				Window:      currentWindow,
			})
		case EventClick:
			if lastClick < 0 || e.At-lastClick > gap {
				idx++
				steps = append(steps, Step{
					Index:       idx,
					At:          e.At,
					Description: fmt.Sprintf("Clicked at (%d, %d)", e.X, e.Y),
					Window:      currentWindow,
				})
			}
			lastClick = e.At
		case EventKeystroke:
			idx++
			steps = append(steps, Step{
				Index:       idx,
				At:          e.At,
				Description: "Typed: " + e.KeyText,
				Window:      currentWindow,
			})
		}
	}
	return steps
}
