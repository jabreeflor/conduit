// Package pluginapi defines the contract plugins use to drive the
// Conduit Video stack. The host wires concrete implementations to the
// interfaces here at plugin-runtime startup.
//
// PRD §13.5. Functions:
//
//	video.record.start / video.record.stop
//	video.screenshot
//	video.edit
//	video.export
//	video.caption
//	video.narrate
//	video.annotate
//	video.highlight
//
// Use cases the API enables:
//   - QA plugin records a test run, attaches annotated screenshots, and
//     emits an annotated bug report
//   - Documentation plugin records a how-to walkthrough on demand
//   - Social plugin generates platform clips at the end of any session
package pluginapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// SessionID is an opaque recording-session handle.
type SessionID string

// RecordOptions controls a recording session.
type RecordOptions struct {
	Width, Height int
	FPS           int
	IncludeAudio  bool
	IncludeWebcam bool
}

// EditRequest carries a natural-language editing intent against a session.
type EditRequest struct {
	Session SessionID
	Intent  string
}

// ExportRequest binds a session to a preset and an output path.
type ExportRequest struct {
	Session SessionID
	Preset  string
	OutPath string
}

// CaptionRequest asks the host to generate captions for a session via Whisper.
type CaptionRequest struct {
	Session  SessionID
	Language string // empty = auto
	Format   string // "srt" | "vtt" | "txt"
}

// NarrateRequest asks the host to add an AI-generated voiceover.
type NarrateRequest struct {
	Session SessionID
	Voice   string // host-defined voice name
	Script  string // empty = derive from steps
}

// AnnotateRequest layers an annotation onto a session timeline.
type AnnotateRequest struct {
	Session SessionID
	At      time.Duration
	Text    string
	X, Y    int
}

// HighlightRequest marks a timeline position as a highlight candidate.
type HighlightRequest struct {
	Session SessionID
	At      time.Duration
	Score   float64 // 0..1; planner uses this to assemble a reel
}

// Screenshot captures a still frame from a session.
type Screenshot struct {
	Session SessionID
	At      time.Duration
	PNG     []byte
}

// Service is the interface plugins call. Method names mirror the
// dotted-API surface in the PRD; the host wires this to the concrete
// recorder, editor, exporter, and voice services.
type Service interface {
	StartRecording(ctx context.Context, opts RecordOptions) (SessionID, error)
	StopRecording(ctx context.Context, id SessionID) error
	Screenshot(ctx context.Context, id SessionID, at time.Duration) (*Screenshot, error)
	Edit(ctx context.Context, req EditRequest) error
	Export(ctx context.Context, req ExportRequest) error
	Caption(ctx context.Context, req CaptionRequest) ([]byte, error)
	Narrate(ctx context.Context, req NarrateRequest) error
	Annotate(ctx context.Context, req AnnotateRequest) error
	Highlight(ctx context.Context, req HighlightRequest) error
}

// --- noop / in-memory test double -------------------------------------------

// Memory is an in-memory Service useful for tests and for the surface
// scaffolds shipped before the real backends are wired up.
type Memory struct {
	mu       sync.Mutex
	sessions map[SessionID]*memSession
	counter  int
}

type memSession struct {
	opts        RecordOptions
	startedAt   time.Time
	stoppedAt   time.Time
	annotations []AnnotateRequest
	highlights  []HighlightRequest
	exports     []ExportRequest
	captions    []CaptionRequest
	narrations  []NarrateRequest
	edits       []EditRequest
}

// NewMemory constructs an empty in-memory service.
func NewMemory() *Memory {
	return &Memory{sessions: map[SessionID]*memSession{}}
}

func (m *Memory) StartRecording(_ context.Context, opts RecordOptions) (SessionID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counter++
	id := SessionID(fmt.Sprintf("session-%d", m.counter))
	m.sessions[id] = &memSession{opts: opts, startedAt: time.Now()}
	return id, nil
}

func (m *Memory) StopRecording(_ context.Context, id SessionID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.sess(id)
	if err != nil {
		return err
	}
	if !s.stoppedAt.IsZero() {
		return errors.New("video: session already stopped")
	}
	s.stoppedAt = time.Now()
	return nil
}

func (m *Memory) Screenshot(_ context.Context, id SessionID, at time.Duration) (*Screenshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.sess(id); err != nil {
		return nil, err
	}
	// Single-pixel PNG placeholder; real backend captures from frame buffer.
	return &Screenshot{Session: id, At: at, PNG: placeholderPNG()}, nil
}

func (m *Memory) Edit(_ context.Context, req EditRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.sess(req.Session)
	if err != nil {
		return err
	}
	if strings.TrimSpace(req.Intent) == "" {
		return errors.New("video: empty edit intent")
	}
	s.edits = append(s.edits, req)
	return nil
}

func (m *Memory) Export(_ context.Context, req ExportRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.sess(req.Session)
	if err != nil {
		return err
	}
	if req.Preset == "" || req.OutPath == "" {
		return errors.New("video: export requires preset and out path")
	}
	s.exports = append(s.exports, req)
	return nil
}

func (m *Memory) Caption(_ context.Context, req CaptionRequest) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.sess(req.Session)
	if err != nil {
		return nil, err
	}
	s.captions = append(s.captions, req)
	return []byte("WEBVTT\n\n00:00.000 --> 00:01.000\nplaceholder caption\n"), nil
}

func (m *Memory) Narrate(_ context.Context, req NarrateRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.sess(req.Session)
	if err != nil {
		return err
	}
	s.narrations = append(s.narrations, req)
	return nil
}

func (m *Memory) Annotate(_ context.Context, req AnnotateRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.sess(req.Session)
	if err != nil {
		return err
	}
	if strings.TrimSpace(req.Text) == "" {
		return errors.New("video: annotation text required")
	}
	s.annotations = append(s.annotations, req)
	return nil
}

func (m *Memory) Highlight(_ context.Context, req HighlightRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.sess(req.Session)
	if err != nil {
		return err
	}
	if req.Score < 0 || req.Score > 1 {
		return errors.New("video: highlight score must be in [0,1]")
	}
	s.highlights = append(s.highlights, req)
	return nil
}

// Inspect returns a snapshot of a session's recorded interactions.
// Useful for tests; not part of the plugin-facing Service.
func (m *Memory) Inspect(id SessionID) (started bool, stopped bool, edits, exports, captions, narrations, annotations, highlights int, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, exists := m.sessions[id]
	if !exists {
		return
	}
	return !s.startedAt.IsZero(), !s.stoppedAt.IsZero(),
		len(s.edits), len(s.exports), len(s.captions), len(s.narrations),
		len(s.annotations), len(s.highlights), true
}

func (m *Memory) sess(id SessionID) (*memSession, error) {
	s, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("video: unknown session %q", id)
	}
	return s, nil
}

// placeholderPNG returns a 1x1 transparent PNG so callers always receive
// a valid image while the real capture backend is being wired up.
func placeholderPNG() []byte {
	// Hand-rolled minimal PNG. Decoder-friendly, displays as 1x1 transparent.
	return []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
		0x89, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
		0x42, 0x60, 0x82,
	}
}
