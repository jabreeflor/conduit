package computeruse

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeCapture writes a canned PNG payload of `size` bytes per call.
func fakeCapture(size int) (CaptureFunc, *int64) {
	var calls int64
	body := make([]byte, size)
	for i := range body {
		body[i] = byte(i % 251)
	}
	return func(_ context.Context, dest string) error {
		atomic.AddInt64(&calls, 1)
		return os.WriteFile(dest, body, 0o644)
	}, &calls
}

func newCapturer(t *testing.T, capture CaptureFunc, opts Options) *Capturer {
	t.Helper()
	dir := t.TempDir()
	opts.SessionsDir = dir
	if capture != nil {
		opts.Capture = capture
	}
	c, err := NewWithOptions("sess-test", opts)
	if err != nil {
		t.Fatalf("NewWithOptions: %v", err)
	}
	return c
}

func TestWithScreenshots_writesPairAndJSONL(t *testing.T) {
	cap, calls := fakeCapture(1024)
	c := newCapturer(t, cap, Options{
		Now: func() time.Time { return time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC) },
	})

	entry, err := c.WithScreenshots(context.Background(), "step-001", func(_ context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("WithScreenshots: %v", err)
	}

	if got := atomic.LoadInt64(calls); got != 2 {
		t.Errorf("capture calls = %d, want 2", got)
	}
	if entry.StepID != "step-001" {
		t.Errorf("StepID = %q, want step-001", entry.StepID)
	}
	if entry.Type != DefaultEntryType {
		t.Errorf("Type = %q, want %q", entry.Type, DefaultEntryType)
	}
	if entry.Status != "success" {
		t.Errorf("Status = %q, want success", entry.Status)
	}
	if !entry.Screenshots.Captured {
		t.Error("Captured should be true on a healthy capture")
	}
	if entry.Screenshots.Pre == "" || entry.Screenshots.Post == "" {
		t.Errorf("expected pre and post paths, got %+v", entry.Screenshots)
	}
	for _, p := range []string{entry.Screenshots.Pre, entry.Screenshots.Post} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected file at %s: %v", p, err)
		}
	}

	// JSONL log roundtrip.
	f, err := os.Open(c.LogPath())
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		t.Fatal("expected one JSONL line")
	}
	var got LogEntry
	if err := json.Unmarshal(sc.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SessionID != "sess-test" || got.StepID != "step-001" {
		t.Errorf("log entry = %+v", got)
	}
	if got.Timestamp.IsZero() {
		t.Error("timestamp not recorded")
	}
}

func TestWithScreenshots_propagatesStepError(t *testing.T) {
	cap, _ := fakeCapture(64)
	c := newCapturer(t, cap, Options{})

	stepErr := errors.New("boom")
	entry, err := c.WithScreenshots(context.Background(), "step-bad", func(_ context.Context) error {
		return stepErr
	})
	if !errors.Is(err, stepErr) {
		t.Fatalf("err = %v, want %v", err, stepErr)
	}
	if entry.Status != "error" {
		t.Errorf("Status = %q, want error", entry.Status)
	}
	if entry.ErrorMsg != "boom" {
		t.Errorf("ErrorMsg = %q", entry.ErrorMsg)
	}
	if entry.ErrorType == "" {
		t.Error("ErrorType should be populated on failure")
	}
	// Pair should still be on disk — audit captures both around the failure.
	if entry.Screenshots.Pre == "" || entry.Screenshots.Post == "" {
		t.Errorf("expected screenshots even on failure, got %+v", entry.Screenshots)
	}
}

func TestWithScreenshots_unavailableBackend(t *testing.T) {
	c := newCapturer(t, UnavailableCapture(), Options{})

	entry, err := c.WithScreenshots(context.Background(), "step-noop", func(_ context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("WithScreenshots returned err: %v", err)
	}
	if entry.Screenshots.Captured {
		t.Error("Captured should be false when backend is unavailable")
	}
	if entry.Screenshots.Pre != "" || entry.Screenshots.Post != "" {
		t.Errorf("paths should be empty, got %+v", entry.Screenshots)
	}
	if !strings.Contains(entry.Screenshots.Reason, runtime.GOOS) {
		t.Errorf("reason should mention GOOS, got %q", entry.Screenshots.Reason)
	}
}

func TestRetention_byStepCount(t *testing.T) {
	// Disable retention during creation so we can stagger mtimes deterministically
	// and then trigger a single retention pass with stable ordering.
	cap, _ := fakeCapture(256)
	c := newCapturer(t, cap, Options{
		MaxStepsPerSession: -1, // disable during seeding
		MaxBytesPerSession: -1,
	})

	for i := 0; i < 5; i++ {
		stepID := fmt.Sprintf("step-%02d", i)
		if _, err := c.WithScreenshots(context.Background(), stepID, func(_ context.Context) error {
			return nil
		}); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		mtime := time.Now().Add(time.Duration(i) * time.Second)
		_ = os.Chtimes(filepath.Join(c.ScreenshotsDir(), stepID+"-pre.png"), mtime, mtime)
		_ = os.Chtimes(filepath.Join(c.ScreenshotsDir(), stepID+"-post.png"), mtime, mtime)
	}
	// Now apply the cap and run retention with stable mtimes in place.
	c.maxStepsPerSession = 2
	c.enforceRetention()

	entries, err := os.ReadDir(c.ScreenshotsDir())
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	pngCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".png") {
			pngCount++
		}
	}
	// 2 steps * 2 files = 4
	if pngCount != 4 {
		t.Errorf("png count = %d, want 4 (2 newest pairs); files: %v", pngCount, listNames(entries))
	}
	// Newest two stepIDs survive.
	for _, want := range []string{"step-03-pre.png", "step-03-post.png", "step-04-pre.png", "step-04-post.png"} {
		if _, err := os.Stat(filepath.Join(c.ScreenshotsDir(), want)); err != nil {
			t.Errorf("expected %s to survive retention: %v", want, err)
		}
	}
}

func TestRetention_byBytes(t *testing.T) {
	// Each capture writes ~1024 bytes, so a 3000-byte cap allows at most one
	// pair (2048 bytes) before the next pair pushes us over.
	cap, _ := fakeCapture(1024)
	c := newCapturer(t, cap, Options{
		MaxStepsPerSession: -1,
		MaxBytesPerSession: 3000,
	})

	for i := 0; i < 3; i++ {
		if _, err := c.WithScreenshots(context.Background(), fmt.Sprintf("s-%d", i), func(_ context.Context) error {
			return nil
		}); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}

	entries, _ := os.ReadDir(c.ScreenshotsDir())
	var total int64
	for _, e := range entries {
		info, _ := e.Info()
		total += info.Size()
	}
	if total > 3000 {
		t.Errorf("total bytes = %d, want <= 3000 (post-pruning)", total)
	}
}

func TestNewWithOptions_requiresSessionID(t *testing.T) {
	if _, err := NewWithOptions("", Options{SessionsDir: t.TempDir()}); err == nil {
		t.Fatal("expected error for empty session id")
	}
}

func TestWithScreenshots_validatesArgs(t *testing.T) {
	cap, _ := fakeCapture(64)
	c := newCapturer(t, cap, Options{})
	if _, err := c.WithScreenshots(context.Background(), "", func(_ context.Context) error { return nil }); err == nil {
		t.Error("expected error for empty step id")
	}
	if _, err := c.WithScreenshots(context.Background(), "step-x", nil); err == nil {
		t.Error("expected error for nil fn")
	}
}

func TestStepIDFromFilename(t *testing.T) {
	cases := map[string]string{
		"step-1-pre.png":  "step-1",
		"step-1-post.png": "step-1",
		"foo-pre.png":     "foo",
		"foo.png":         "",
		"foo.txt":         "foo.txt",
	}
	for name, want := range cases {
		// Only test names with .png suffix; the function only normalizes that.
		if !strings.HasSuffix(name, ".png") {
			continue
		}
		got := stepIDFromFilename(name)
		if got != want {
			t.Errorf("stepIDFromFilename(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestUnavailableCapture_returnsSentinel(t *testing.T) {
	err := UnavailableCapture()(context.Background(), filepath.Join(t.TempDir(), "x.png"))
	if !errors.Is(err, ErrCaptureUnavailable) {
		t.Errorf("expected ErrCaptureUnavailable, got %v", err)
	}
}

func listNames(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}
