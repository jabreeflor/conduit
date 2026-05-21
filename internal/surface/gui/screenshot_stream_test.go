package gui

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// helpers -----------------------------------------------------------------

func makeStep(id string, ts time.Time) ScreenshotStep {
	return ScreenshotStep{
		StepID:     id,
		Timestamp:  ts,
		PrePath:    fmt.Sprintf("/tmp/%s-pre.png", id),
		PostPath:   fmt.Sprintf("/tmp/%s-post.png", id),
		Captured:   true,
		DurationMS: 100,
		Status:     "success",
	}
}

var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// -------------------------------------------------------------------------

func TestScreenshotStream_PushAndSteps(t *testing.T) {
	s := NewScreenshotStream(0)

	s.Push(makeStep("step-1", epoch))
	s.Push(makeStep("step-2", epoch.Add(time.Second)))

	steps := s.Steps()
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if steps[0].StepID != "step-1" {
		t.Errorf("expected step-1 first, got %s", steps[0].StepID)
	}
	if steps[1].StepID != "step-2" {
		t.Errorf("expected step-2 second, got %s", steps[1].StepID)
	}
}

func TestScreenshotStream_PushUpdate(t *testing.T) {
	s := NewScreenshotStream(0)

	// Simulate: pre arrives, then post arrives via a second Push.
	pre := makeStep("step-1", epoch)
	pre.PostPath = ""
	pre.Captured = false
	s.Push(pre)

	post := makeStep("step-1", epoch)
	post.PrePath = "" // only updating post
	s.Push(post)

	steps := s.Steps()
	if len(steps) != 1 {
		t.Fatalf("expected 1 step after update, got %d", len(steps))
	}
	if steps[0].PrePath != pre.PrePath {
		t.Errorf("pre path should have been preserved; got %q", steps[0].PrePath)
	}
	if steps[0].PostPath != post.PostPath {
		t.Errorf("post path not updated; got %q", steps[0].PostPath)
	}
}

func TestScreenshotStream_ActiveStepAutoAdvances(t *testing.T) {
	s := NewScreenshotStream(0)

	s.Push(makeStep("step-1", epoch))
	if s.ActiveStep().StepID != "step-1" {
		t.Error("active should be step-1 after first push")
	}

	s.Push(makeStep("step-2", epoch.Add(time.Second)))
	if s.ActiveStep().StepID != "step-2" {
		t.Error("active should auto-advance to step-2")
	}
}

func TestScreenshotStream_ActiveImagePath(t *testing.T) {
	s := NewScreenshotStream(0)
	step := makeStep("s1", epoch)
	s.Push(step)

	// default phase is PhasePost
	if s.ActiveImagePath() != step.PostPath {
		t.Errorf("expected post path %q, got %q", step.PostPath, s.ActiveImagePath())
	}

	s.SelectPhase(PhasePre)
	if s.ActiveImagePath() != step.PrePath {
		t.Errorf("expected pre path %q, got %q", step.PrePath, s.ActiveImagePath())
	}
}

func TestScreenshotStream_ActiveImagePath_FallsBackToPre(t *testing.T) {
	s := NewScreenshotStream(0)
	step := makeStep("s1", epoch)
	step.PostPath = "" // post not yet available
	s.Push(step)

	s.SelectPhase(PhasePost)
	// Should fall back to pre path when post is missing.
	if s.ActiveImagePath() != step.PrePath {
		t.Errorf("expected fallback to pre path %q, got %q", step.PrePath, s.ActiveImagePath())
	}
}

func TestScreenshotStream_Pin(t *testing.T) {
	s := NewScreenshotStream(0)
	s.Push(makeStep("s1", epoch))
	s.Push(makeStep("s2", epoch.Add(time.Second)))

	s.Pin("s1")

	pinned := s.PinnedSteps()
	if len(pinned) != 1 || pinned[0].StepID != "s1" {
		t.Errorf("expected s1 pinned, got %v", pinned)
	}

	paths := s.PinnedPaths()
	if len(paths) != 1 {
		t.Fatalf("expected 1 pinned path, got %d", len(paths))
	}
	if paths[0] != "/tmp/s1-post.png" {
		t.Errorf("unexpected pinned path %q", paths[0])
	}
}

func TestScreenshotStream_Unpin(t *testing.T) {
	s := NewScreenshotStream(0)
	s.Push(makeStep("s1", epoch))
	s.Pin("s1")
	s.Unpin("s1")

	if len(s.PinnedSteps()) != 0 {
		t.Error("expected no pinned steps after Unpin")
	}
}

func TestScreenshotStream_Navigation(t *testing.T) {
	s := NewScreenshotStream(0)
	s.Push(makeStep("s1", epoch))
	s.Push(makeStep("s2", epoch.Add(time.Second)))
	s.Push(makeStep("s3", epoch.Add(2*time.Second)))

	// Active is s3 (newest).
	s.NavigatePrev()
	if s.ActiveStep().StepID != "s2" {
		t.Errorf("after NavigatePrev, expected s2, got %s", s.ActiveStep().StepID)
	}

	s.NavigatePrev()
	if s.ActiveStep().StepID != "s1" {
		t.Errorf("after 2× NavigatePrev, expected s1, got %s", s.ActiveStep().StepID)
	}

	// Already at oldest; NavigatePrev should be a no-op.
	s.NavigatePrev()
	if s.ActiveStep().StepID != "s1" {
		t.Errorf("NavigatePrev at oldest should not change; got %s", s.ActiveStep().StepID)
	}

	s.NavigateNext()
	if s.ActiveStep().StepID != "s2" {
		t.Errorf("after NavigateNext, expected s2, got %s", s.ActiveStep().StepID)
	}
}

func TestScreenshotStream_MaxStepsEviction(t *testing.T) {
	s := NewScreenshotStream(3)
	for i := 1; i <= 4; i++ {
		s.Push(makeStep(fmt.Sprintf("s%d", i), epoch.Add(time.Duration(i)*time.Second)))
	}

	// s1 should be evicted (oldest unpinned), leaving s2, s3, s4.
	if s.Len() != 3 {
		t.Fatalf("expected 3 steps after eviction, got %d", s.Len())
	}
	steps := s.Steps()
	if steps[0].StepID != "s2" {
		t.Errorf("expected s2 as oldest after eviction, got %s", steps[0].StepID)
	}
}

func TestScreenshotStream_MaxStepsPinnedNotEvicted(t *testing.T) {
	s := NewScreenshotStream(2)
	s.Push(makeStep("s1", epoch))
	s.Pin("s1")
	s.Push(makeStep("s2", epoch.Add(time.Second)))
	s.Push(makeStep("s3", epoch.Add(2*time.Second)))

	// s1 is pinned, so s2 should be evicted instead.
	if s.Len() != 2 {
		t.Fatalf("expected 2 steps, got %d", s.Len())
	}
	steps := s.Steps()
	ids := make([]string, len(steps))
	for i, step := range steps {
		ids[i] = step.StepID
	}
	for _, id := range ids {
		if id == "s2" {
			t.Errorf("s2 should have been evicted, but steps are %v", ids)
		}
	}
	// s1 (pinned) must still be present.
	found := false
	for _, id := range ids {
		if id == "s1" {
			found = true
		}
	}
	if !found {
		t.Errorf("pinned s1 was evicted; steps = %v", ids)
	}
}

func TestScreenshotStream_Clear(t *testing.T) {
	s := NewScreenshotStream(0)
	s.Push(makeStep("s1", epoch))
	s.Push(makeStep("s2", epoch.Add(time.Second)))
	s.Pin("s2")

	s.Clear()

	// Only s2 (pinned) should remain.
	if s.Len() != 1 {
		t.Fatalf("expected 1 step after Clear, got %d", s.Len())
	}
	if s.Steps()[0].StepID != "s2" {
		t.Errorf("expected s2 to survive Clear, got %s", s.Steps()[0].StepID)
	}
}

func TestScreenshotStream_EmptyStream(t *testing.T) {
	s := NewScreenshotStream(0)
	if s.ActiveStep() != nil {
		t.Error("empty stream should return nil ActiveStep")
	}
	if s.ActiveImagePath() != "" {
		t.Error("empty stream should return empty ActiveImagePath")
	}
	if s.Len() != 0 {
		t.Errorf("empty stream should have Len=0, got %d", s.Len())
	}
}

func TestScreenshotStream_ConcurrentPush(t *testing.T) {
	s := NewScreenshotStream(100)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			s.Push(makeStep(fmt.Sprintf("a%d", i), epoch.Add(time.Duration(i)*time.Millisecond)))
		}
		done <- struct{}{}
	}()
	for i := 0; i < 50; i++ {
		s.Push(makeStep(fmt.Sprintf("b%d", i), epoch.Add(time.Duration(i)*time.Millisecond)))
	}
	<-done
	// No panic = concurrent safety confirmed; just verify the count is sane.
	if s.Len() == 0 {
		t.Error("expected steps after concurrent push")
	}
}

func TestScreenshotStream_StepsByTimestamp(t *testing.T) {
	s := NewScreenshotStream(0)
	// Push out of order.
	s.Push(makeStep("s3", epoch.Add(3*time.Second)))
	s.Push(makeStep("s1", epoch.Add(1*time.Second)))
	s.Push(makeStep("s2", epoch.Add(2*time.Second)))

	sorted := s.StepsByTimestamp()
	for i := 1; i < len(sorted); i++ {
		if sorted[i].Timestamp.Before(sorted[i-1].Timestamp) {
			t.Errorf("not sorted at index %d", i)
		}
	}
}

func TestThumbnailPath(t *testing.T) {
	step := makeStep("s1", epoch)
	if ThumbnailPath(step) != step.PostPath {
		t.Errorf("expected post path as thumbnail")
	}
	step.PostPath = ""
	if ThumbnailPath(step) != step.PrePath {
		t.Errorf("expected pre fallback when post is missing")
	}
}

func TestStepBasename(t *testing.T) {
	path := "/some/long/dir/step-1-post.png"
	got := StepBasename(path)
	want := filepath.Base(path)
	if got != want {
		t.Errorf("StepBasename(%q) = %q, want %q", path, got, want)
	}
}
