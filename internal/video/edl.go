// Package video implements the Conduit Video editor — a non-destructive
// edit-decision-list (EDL) timeline plus a natural-language editing
// surface that converts prose like "remove the ums" or "make a 60s
// highlight reel" into EDL operations.
//
// The package is renderer-agnostic. Concrete export (FFmpeg/AVFoundation)
// lives in subpackages under internal/video/export — see issue #110.
//
// PRD §13.1.
package video

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// Time is a position or duration on the timeline. We use time.Duration
// throughout for unit safety.
type Time = time.Duration

// TrackKind enumerates the timeline lanes.
type TrackKind string

const (
	TrackVideo   TrackKind = "video"
	TrackAudio   TrackKind = "audio"
	TrackOverlay TrackKind = "overlay"
	TrackCaption TrackKind = "caption"
)

// Clip is one media segment on the timeline.
type Clip struct {
	ID      string
	Kind    TrackKind
	Source  string  // file path or imported URL ID
	In, Out Time    // source-time range
	Start   Time    // timeline position
	Speed   float64 // 1.0 = normal; <1.0 slow-mo; >1.0 fast
	Volume  float64 // 0.0 mute, 1.0 unity
	Effects []Effect
}

// Effect is a non-destructive transformation attached to a clip.
type Effect struct {
	Kind   string         // "color-correct" | "zoom-pan" | "transition" | "intro" | "outro" | …
	Params map[string]any // kind-specific parameters
}

// Caption is a single timed text entry on the caption track.
type Caption struct {
	Start, End Time
	Text       string
}

// EDL is the non-destructive edit decision list.
type EDL struct {
	Clips    []Clip
	Captions []Caption
	Music    *MusicTrack // optional background music with auto-duck
	// Markers can be set by users or by AI helpers (silence detection,
	// highlight scorer, …) and inform later operations.
	Markers []Marker
}

// MusicTrack is the background-music layer.
type MusicTrack struct {
	Source     string
	Volume     float64
	AutoDuckdB float64 // negative dB to drop when speech is present
}

// Marker is a named timestamp.
type Marker struct {
	At    Time
	Label string
}

// New returns an empty EDL.
func New() *EDL { return &EDL{} }

// Duration returns the timeline length (max Start+Length across clips).
func (e *EDL) Duration() Time {
	var max Time
	for _, c := range e.Clips {
		end := c.Start + c.Length()
		if end > max {
			max = end
		}
	}
	return max
}

// Length returns the on-timeline duration of a clip after speed.
func (c Clip) Length() Time {
	src := c.Out - c.In
	if c.Speed <= 0 {
		return src
	}
	return time.Duration(float64(src) / c.Speed)
}

// AddClip appends a clip and returns its ID.
func (e *EDL) AddClip(c Clip) string {
	if c.ID == "" {
		c.ID = fmt.Sprintf("clip-%d", len(e.Clips)+1)
	}
	if c.Speed == 0 {
		c.Speed = 1.0
	}
	e.Clips = append(e.Clips, c)
	return c.ID
}

// Trim sets the In/Out range on a clip.
func (e *EDL) Trim(id string, in, out Time) error {
	c, idx := e.find(id)
	if c == nil {
		return notFound(id)
	}
	if out <= in {
		return errors.New("video: trim out must be > in")
	}
	c.In, c.Out = in, out
	e.Clips[idx] = *c
	return nil
}

// Split divides a clip at timeline-time at into two clips. The new
// second-half clip is returned.
func (e *EDL) Split(id string, at Time) (string, error) {
	c, idx := e.find(id)
	if c == nil {
		return "", notFound(id)
	}
	rel := at - c.Start
	if rel <= 0 || rel >= c.Length() {
		return "", errors.New("video: split position outside clip")
	}
	srcRel := time.Duration(float64(rel) * c.Speed)
	right := *c
	right.ID = ""
	right.In = c.In + srcRel
	right.Start = at
	c.Out = c.In + srcRel
	e.Clips[idx] = *c
	return e.AddClip(right), nil
}

// Cut removes a range of timeline-time across all clips that touch it.
// Used by silence removal and the "remove the ums" intent.
func (e *EDL) Cut(start, end Time) {
	if end <= start {
		return
	}
	out := make([]Clip, 0, len(e.Clips))
	for _, c := range e.Clips {
		cs, ce := c.Start, c.Start+c.Length()
		switch {
		case ce <= start || cs >= end:
			out = append(out, c) // outside cut
		case cs >= start && ce <= end:
			// fully inside cut — drop
		default:
			// straddle: keep the surviving portion(s)
			if cs < start {
				left := c
				left.Out = c.In + time.Duration(float64(start-cs)*c.Speed)
				out = append(out, left)
			}
			if ce > end {
				right := c
				right.In = c.In + time.Duration(float64(end-cs)*c.Speed)
				right.Start = end
				out = append(out, right)
			}
		}
	}
	// Shift everything after `end` left by (end-start).
	gap := end - start
	for i := range out {
		if out[i].Start >= end {
			out[i].Start -= gap
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	e.Clips = out
}

// AddTransition attaches a transition effect of the named kind between
// two adjacent clips. Length controls the cross-fade duration.
func (e *EDL) AddTransition(prevID, nextID, kind string, length Time) error {
	if _, idx := e.find(prevID); idx < 0 {
		return notFound(prevID)
	}
	if _, idx := e.find(nextID); idx < 0 {
		return notFound(nextID)
	}
	c, idx := e.find(prevID)
	c.Effects = append(c.Effects, Effect{
		Kind:   "transition",
		Params: map[string]any{"to": nextID, "kind": kind, "length": length},
	})
	e.Clips[idx] = *c
	return nil
}

// SetMusic attaches a background-music track with auto-duck.
func (e *EDL) SetMusic(m MusicTrack) { e.Music = &m }

// AddCaption appends a caption entry, keeping the slice ordered by Start.
func (e *EDL) AddCaption(c Caption) {
	e.Captions = append(e.Captions, c)
	sort.SliceStable(e.Captions, func(i, j int) bool {
		return e.Captions[i].Start < e.Captions[j].Start
	})
}

// AddMarker appends a marker, keeping the slice ordered.
func (e *EDL) AddMarker(m Marker) {
	e.Markers = append(e.Markers, m)
	sort.SliceStable(e.Markers, func(i, j int) bool {
		return e.Markers[i].At < e.Markers[j].At
	})
}

func (e *EDL) find(id string) (*Clip, int) {
	for i := range e.Clips {
		if e.Clips[i].ID == id {
			c := e.Clips[i]
			return &c, i
		}
	}
	return nil, -1
}

func notFound(id string) error { return fmt.Errorf("video: clip %q not found", id) }
