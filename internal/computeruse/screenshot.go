// Package computeruse captures pre/post screenshots around computer-use steps
// and appends an audit entry to the per-session JSONL log.
//
// Storage layout (PRD §6.8 + §6.13):
//
//	~/.conduit/sessions/<session-id>/
//	  session.jsonl                      append-only audit log
//	  screenshots/
//	    <step-id>-pre.png
//	    <step-id>-post.png
//
// Capture strategy: macOS uses the built-in `screencapture -x` command (silent,
// no shutter sound, no permission prompts beyond Screen Recording). On every
// other GOOS the pipeline degrades to a no-op so the rest of the harness keeps
// working — the JSONL entry is still emitted with empty screenshot paths and a
// "captured: false" reason field so downstream replay tools can flag the gap.
//
// This package is the integration point for the MCP step dispatcher landing in
// issues #37 / #40. Until that dispatcher exists, callers wrap each step with
// Capturer.WithScreenshots(ctx, stepID, fn).
package computeruse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultEntryType is the JSONL "type" field for one wrapped step.
	DefaultEntryType = "computer-use-step"

	// DefaultMaxBytesPerSession caps screenshot disk usage per session before
	// the oldest pair is pruned. 256 MB ≈ ~500 step pairs at typical Retina PNG
	// sizes — enough for a long debug session without filling small disks.
	DefaultMaxBytesPerSession int64 = 256 * 1024 * 1024

	// DefaultMaxStepsPerSession caps the screenshot pair count, preventing a
	// runaway loop from generating millions of files even if each is small.
	DefaultMaxStepsPerSession = 2000

	defaultSessionsDir = ".conduit/sessions"
	screenshotsDir     = "screenshots"
	sessionLogFile     = "session.jsonl"
	screenshotExt      = ".png"
)

// ErrCaptureUnavailable is returned by capture backends that cannot run on the
// current platform. Callers treat it as a soft failure (log, continue).
var ErrCaptureUnavailable = errors.New("computeruse: screenshot capture not available on this platform")

// CaptureFunc takes a destination path and writes a PNG to it. Backends MUST
// either populate the file or return a non-nil error; partial writes are
// cleaned up by the caller.
type CaptureFunc func(ctx context.Context, dest string) error

// LogEntry is one record in the per-session JSONL log. The shape mirrors the
// usage tracker (#15) — flat JSON, lower-snake field names, timestamps in UTC.
type LogEntry struct {
	Timestamp   time.Time      `json:"timestamp"`
	SessionID   string         `json:"session_id"`
	StepID      string         `json:"step_id"`
	Type        string         `json:"type"`
	Screenshots ScreenshotPair `json:"screenshots"`
	DurationMS  int64          `json:"duration_ms"`
	Status      string         `json:"status"`
	ErrorType   string         `json:"error_type,omitempty"`
	ErrorMsg    string         `json:"error,omitempty"`
}

// ScreenshotPair holds the absolute paths of the pre/post captures plus a
// human-readable reason if either capture was skipped.
type ScreenshotPair struct {
	Pre      string `json:"pre,omitempty"`
	Post     string `json:"post,omitempty"`
	Captured bool   `json:"captured"`
	Reason   string `json:"reason,omitempty"`
}

// Options configure a Capturer. Zero values fall back to safe defaults.
type Options struct {
	// SessionsDir overrides the root sessions directory. Defaults to
	// ~/.conduit/sessions.
	SessionsDir string

	// Capture overrides the screenshot backend. Defaults to the platform
	// backend (macOS native; no-op elsewhere).
	Capture CaptureFunc

	// MaxBytesPerSession caps total screenshot disk usage per session.
	// <=0 disables the byte cap.
	MaxBytesPerSession int64

	// MaxStepsPerSession caps the number of step-pairs retained.
	// <=0 disables the count cap.
	MaxStepsPerSession int

	// Now overrides the clock (test seam).
	Now func() time.Time
}

// Capturer wraps computer-use steps with pre/post screenshot capture and
// appends a structured entry to the session log. All methods are safe for
// concurrent use.
type Capturer struct {
	mu sync.Mutex

	sessionID      string
	sessionDir     string
	screenshotsDir string
	logPath        string

	capture            CaptureFunc
	maxBytesPerSession int64
	maxStepsPerSession int
	now                func() time.Time
}

// New creates a Capturer rooted at ~/.conduit/sessions/<sessionID>.
func New(sessionID string) (*Capturer, error) {
	return NewWithOptions(sessionID, Options{})
}

// NewWithOptions creates a Capturer with explicit configuration, useful in
// tests and when the harness has a non-default storage root.
func NewWithOptions(sessionID string, opts Options) (*Capturer, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, errors.New("computeruse: session id is required")
	}

	root := opts.SessionsDir
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("computeruse: resolve home dir: %w", err)
		}
		root = filepath.Join(home, defaultSessionsDir)
	}

	sessionDir := filepath.Join(root, sessionID)
	shotsDir := filepath.Join(sessionDir, screenshotsDir)
	if err := os.MkdirAll(shotsDir, 0o755); err != nil {
		return nil, fmt.Errorf("computeruse: create screenshots dir: %w", err)
	}

	capture := opts.Capture
	if capture == nil {
		capture = platformCapture()
	}

	maxBytes := opts.MaxBytesPerSession
	if maxBytes == 0 {
		maxBytes = DefaultMaxBytesPerSession
	}
	maxSteps := opts.MaxStepsPerSession
	if maxSteps == 0 {
		maxSteps = DefaultMaxStepsPerSession
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	return &Capturer{
		sessionID:          sessionID,
		sessionDir:         sessionDir,
		screenshotsDir:     shotsDir,
		logPath:            filepath.Join(sessionDir, sessionLogFile),
		capture:            capture,
		maxBytesPerSession: maxBytes,
		maxStepsPerSession: maxSteps,
		now:                now,
	}, nil
}

// SessionID returns the session this Capturer is bound to.
func (c *Capturer) SessionID() string { return c.sessionID }

// LogPath returns the per-session JSONL log path.
func (c *Capturer) LogPath() string { return c.logPath }

// ScreenshotsDir returns the per-session screenshots directory.
func (c *Capturer) ScreenshotsDir() string { return c.screenshotsDir }

// WithScreenshots captures a pre-screenshot, runs fn, captures a post-
// screenshot, then appends one JSONL entry to the session log.
//
// fn's error is propagated unchanged. Capture failures are recorded in the log
// entry but never mask fn's error — pre/post audit must not break the dispatch
// path.
//
// TODO(#37, #40): Once the MCP step dispatcher lands, register
// Capturer.WithScreenshots as the Pre/Post step hook so every dispatched
// computer-use step is wrapped automatically.
func (c *Capturer) WithScreenshots(ctx context.Context, stepID string, fn func(context.Context) error) (LogEntry, error) {
	if strings.TrimSpace(stepID) == "" {
		return LogEntry{}, errors.New("computeruse: step id is required")
	}
	if fn == nil {
		return LogEntry{}, errors.New("computeruse: step function is required")
	}

	start := c.now()
	prePath, preReason := c.snap(ctx, stepID, "pre")
	fnErr := fn(ctx)
	postPath, postReason := c.snap(ctx, stepID, "post")
	end := c.now()

	pair := ScreenshotPair{Pre: prePath, Post: postPath}
	pair.Captured = prePath != "" && postPath != ""
	switch {
	case preReason != "" && postReason != "" && preReason == postReason:
		pair.Reason = preReason
	case preReason != "" && postReason != "":
		pair.Reason = "pre: " + preReason + "; post: " + postReason
	case preReason != "":
		pair.Reason = "pre: " + preReason
	case postReason != "":
		pair.Reason = "post: " + postReason
	}

	entry := LogEntry{
		Timestamp:   start.UTC(),
		SessionID:   c.sessionID,
		StepID:      stepID,
		Type:        DefaultEntryType,
		Screenshots: pair,
		DurationMS:  end.Sub(start).Milliseconds(),
		Status:      "success",
	}
	if fnErr != nil {
		entry.Status = "error"
		entry.ErrorMsg = fnErr.Error()
		entry.ErrorType = errorType(fnErr)
	}

	if err := c.appendLog(entry); err != nil {
		// Log append failure must surface so callers can decide whether to
		// abort — but never overwrite the original step error.
		if fnErr != nil {
			return entry, fnErr
		}
		return entry, err
	}

	c.enforceRetention()
	return entry, fnErr
}

// snap takes one screenshot and returns its absolute path on success, or an
// empty path plus a reason string on soft failure.
func (c *Capturer) snap(ctx context.Context, stepID, phase string) (string, string) {
	if c.capture == nil {
		return "", "no capture backend"
	}
	name := stepID + "-" + phase + screenshotExt
	dest := filepath.Join(c.screenshotsDir, name)

	if err := c.capture(ctx, dest); err != nil {
		_ = os.Remove(dest)
		if errors.Is(err, ErrCaptureUnavailable) {
			return "", "unavailable on " + runtime.GOOS
		}
		return "", err.Error()
	}
	if info, err := os.Stat(dest); err != nil || info.Size() == 0 {
		_ = os.Remove(dest)
		return "", "empty capture"
	}
	return dest, ""
}

func (c *Capturer) appendLog(entry LogEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("computeruse: marshal log entry: %w", err)
	}
	line = append(line, '\n')

	f, err := os.OpenFile(c.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("computeruse: open session log: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("computeruse: append session log: %w", err)
	}
	return nil
}

// enforceRetention prunes the oldest screenshot pair files when either the
// byte cap or step-count cap is exceeded. Pruning is best-effort; failures are
// swallowed so the dispatch path never aborts on housekeeping.
func (c *Capturer) enforceRetention() {
	c.mu.Lock()
	defer c.mu.Unlock()

	dirEntries, err := os.ReadDir(c.screenshotsDir)
	if err != nil {
		return
	}

	pairs := make(map[string][]shot)
	var totalBytes int64

	for _, de := range dirEntries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), screenshotExt) {
			continue
		}
		info, err := de.Info()
		if err != nil {
			continue
		}
		stepID := stepIDFromFilename(de.Name())
		if stepID == "" {
			continue
		}
		full := filepath.Join(c.screenshotsDir, de.Name())
		pairs[stepID] = append(pairs[stepID], shot{
			path:    full,
			size:    info.Size(),
			modTime: info.ModTime(),
			stepID:  stepID,
		})
		totalBytes += info.Size()
	}

	if len(pairs) == 0 {
		return
	}

	// Order step IDs by the oldest file mtime in each pair — oldest first.
	stepIDs := make([]string, 0, len(pairs))
	for id := range pairs {
		stepIDs = append(stepIDs, id)
	}
	sort.Slice(stepIDs, func(i, j int) bool {
		return oldest(pairs[stepIDs[i]]).Before(oldest(pairs[stepIDs[j]]))
	})

	overByCount := c.maxStepsPerSession > 0 && len(stepIDs) > c.maxStepsPerSession
	overByBytes := c.maxBytesPerSession > 0 && totalBytes > c.maxBytesPerSession

	for (overByCount || overByBytes) && len(stepIDs) > 0 {
		victim := stepIDs[0]
		stepIDs = stepIDs[1:]
		for _, s := range pairs[victim] {
			if err := os.Remove(s.path); err == nil {
				totalBytes -= s.size
			}
		}
		delete(pairs, victim)
		overByCount = c.maxStepsPerSession > 0 && len(pairs) > c.maxStepsPerSession
		overByBytes = c.maxBytesPerSession > 0 && totalBytes > c.maxBytesPerSession
	}
}

func oldest(shots []shot) time.Time {
	var t time.Time
	for i, s := range shots {
		if i == 0 || s.modTime.Before(t) {
			t = s.modTime
		}
	}
	return t
}

type shot struct {
	path    string
	size    int64
	modTime time.Time
	stepID  string
}

// stepIDFromFilename extracts the step id from "<stepID>-pre.png" /
// "<stepID>-post.png", returning "" if the suffix doesn't match.
func stepIDFromFilename(name string) string {
	base := strings.TrimSuffix(name, screenshotExt)
	switch {
	case strings.HasSuffix(base, "-pre"):
		return strings.TrimSuffix(base, "-pre")
	case strings.HasSuffix(base, "-post"):
		return strings.TrimSuffix(base, "-post")
	}
	return ""
}

func errorType(err error) string {
	if err == nil {
		return ""
	}
	// Keep the type label simple — the engine's error classifier (PRD §6.9)
	// owns the canonical taxonomy. We just record the concrete error type so
	// session log readers can group failures.
	return fmt.Sprintf("%T", err)
}

// platformCapture returns the default backend for the current GOOS.
func platformCapture() CaptureFunc {
	if runtime.GOOS == "darwin" {
		return MacOSScreencapture()
	}
	return UnavailableCapture()
}

// MacOSScreencapture invokes `screencapture -x <dest>` (silent, no shutter
// sound). Requires the Screen Recording permission — issue #38 owns the
// permission flow. If `screencapture` is missing or the command fails, an
// error is returned so the wrapper records the reason.
func MacOSScreencapture() CaptureFunc {
	return func(ctx context.Context, dest string) error {
		if runtime.GOOS != "darwin" {
			return ErrCaptureUnavailable
		}
		// -x silences the shutter sound, -t png forces PNG output regardless
		// of the file extension. We pass a context so the parent can cancel
		// long-running captures (rare, but defensive).
		cmd := exec.CommandContext(ctx, "screencapture", "-x", "-t", "png", dest)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("screencapture failed: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
}

// UnavailableCapture is the no-op backend used on non-macOS platforms.
func UnavailableCapture() CaptureFunc {
	return func(_ context.Context, _ string) error {
		return ErrCaptureUnavailable
	}
}
