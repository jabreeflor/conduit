package gui

import (
	"fmt"
	"testing"
)

func TestCanvasPanel_PushAndHTML(t *testing.T) {
	c := NewCanvasPanel()
	c.Push("<h1>Hello</h1>")
	if got := c.HTML(); got != "<h1>Hello</h1>" {
		t.Errorf("HTML() = %q, want <h1>Hello</h1>", got)
	}
}

func TestCanvasPanel_PushMakesVisible(t *testing.T) {
	c := NewCanvasPanel()
	if c.Visible() {
		t.Error("new panel should not be visible")
	}
	c.Push("<p>hi</p>")
	if !c.Visible() {
		t.Error("panel should become visible after first Push")
	}
}

func TestCanvasPanel_ShowHide(t *testing.T) {
	c := NewCanvasPanel()
	c.Push("<p>x</p>")
	c.Hide()
	if c.Visible() {
		t.Error("expected hidden after Hide()")
	}
	c.Show()
	if !c.Visible() {
		t.Error("expected visible after Show()")
	}
}

func TestCanvasPanel_Navigation(t *testing.T) {
	c := NewCanvasPanel()
	c.Push("<p>frame1</p>")
	c.Push("<p>frame2</p>")
	c.Push("<p>frame3</p>")

	// Active is frame3 (newest).
	if c.HTML() != "<p>frame3</p>" {
		t.Errorf("expected frame3, got %q", c.HTML())
	}

	c.Back()
	if c.HTML() != "<p>frame2</p>" {
		t.Errorf("after Back: expected frame2, got %q", c.HTML())
	}

	c.Back()
	if c.HTML() != "<p>frame1</p>" {
		t.Errorf("after 2× Back: expected frame1, got %q", c.HTML())
	}

	// Already at oldest; Back is a no-op.
	c.Back()
	if c.HTML() != "<p>frame1</p>" {
		t.Error("Back() at oldest frame should not change HTML")
	}

	c.Forward()
	if c.HTML() != "<p>frame2</p>" {
		t.Errorf("after Forward: expected frame2, got %q", c.HTML())
	}
}

func TestCanvasPanel_CanGoBackForward(t *testing.T) {
	c := NewCanvasPanel()
	if c.CanGoBack() || c.CanGoForward() {
		t.Error("empty panel should not be navigable")
	}

	c.Push("<p>a</p>")
	if c.CanGoBack() {
		t.Error("single frame: CanGoBack should be false")
	}
	if c.CanGoForward() {
		t.Error("single frame: CanGoForward should be false")
	}

	c.Push("<p>b</p>")
	if !c.CanGoBack() {
		t.Error("two frames at newest: CanGoBack should be true")
	}
	if c.CanGoForward() {
		t.Error("two frames at newest: CanGoForward should be false")
	}

	c.Back()
	if c.CanGoBack() {
		t.Error("at oldest of two: CanGoBack should be false")
	}
	if !c.CanGoForward() {
		t.Error("at oldest of two: CanGoForward should be true")
	}
}

func TestCanvasPanel_Reload(t *testing.T) {
	c := NewCanvasPanel()
	if c.Reload() != "" {
		t.Error("Reload on empty panel should return empty string")
	}

	c.Push("<div>reload me</div>")
	if c.Reload() != "<div>reload me</div>" {
		t.Errorf("Reload() = %q, want <div>reload me</div>", c.Reload())
	}
}

// TestCanvasPanel_PushTruncatesForwardHistory verifies browser-style
// navigation: when a new frame is pushed while the cursor is in the past,
// forward frames are discarded before appending.
//
// This is the key UX decision — see the implementation note in canvas.go.
// TODO: implement this test.
//
// Guidance: Set up a panel with 3 frames, navigate back to frame 1, push a
// new frame, then assert on Len() and the HTML sequence. Consider:
//   - What should Len() be after the push? (Was 3, cursor at 0, then push.)
//   - What HTML should Back()/Forward() reveal?
//   - Should the panel remain visible?
func TestCanvasPanel_PushTruncatesForwardHistory(t *testing.T) {
	c := NewCanvasPanel()
	c.Push("<p>frame1</p>")
	c.Push("<p>frame2</p>")
	c.Push("<p>frame3</p>")

	// Navigate back to frame1 (cursor = 0).
	c.Back()
	c.Back()
	if c.HTML() != "<p>frame1</p>" {
		t.Fatalf("expected frame1, got %q", c.HTML())
	}

	// Push a new frame while mid-history — forward frames (frame2, frame3)
	// must be discarded before appending.
	c.Push("<p>new</p>")

	// History should now be [frame1, new] — exactly 2 frames.
	if got := c.Len(); got != 2 {
		t.Errorf("Len() = %d, want 2 (frame1 + new, forward history discarded)", got)
	}
	if got := c.HTML(); got != "<p>new</p>" {
		t.Errorf("HTML() after push = %q, want <p>new</p>", got)
	}
	if c.CanGoForward() {
		t.Error("CanGoForward should be false after push truncates forward history")
	}
	if !c.CanGoBack() {
		t.Error("CanGoBack should be true (frame1 is still in history)")
	}
	c.Back()
	if got := c.HTML(); got != "<p>frame1</p>" {
		t.Errorf("after Back: HTML() = %q, want <p>frame1</p>", got)
	}
	// Panel should remain visible throughout.
	if !c.Visible() {
		t.Error("panel should remain visible after push-truncate")
	}
}

func TestCanvasPanel_Clear(t *testing.T) {
	c := NewCanvasPanel()
	c.Push("<p>a</p>")
	c.Push("<p>b</p>")
	c.Clear()

	if c.Len() != 0 {
		t.Errorf("after Clear: Len = %d, want 0", c.Len())
	}
	if c.Visible() {
		t.Error("after Clear: panel should not be visible")
	}
	if c.HTML() != "" {
		t.Errorf("after Clear: HTML() = %q, want empty", c.HTML())
	}
}

func TestCanvasPanel_EmptyPanel(t *testing.T) {
	c := NewCanvasPanel()
	if c.HTML() != "" {
		t.Error("empty panel should return empty HTML")
	}
	if c.Len() != 0 {
		t.Errorf("empty panel Len = %d, want 0", c.Len())
	}
	if c.Visible() {
		t.Error("empty panel should not be visible")
	}
	// Navigation on empty panel should not panic.
	c.Back()
	c.Forward()
}

func TestCanvasPanel_Len(t *testing.T) {
	c := NewCanvasPanel()
	for i := 0; i < 5; i++ {
		c.Push(fmt.Sprintf("<p>frame%d</p>", i))
	}
	if c.Len() != 5 {
		t.Errorf("Len() = %d, want 5", c.Len())
	}
}

func TestCanvasPanel_ConcurrentPush(t *testing.T) {
	c := NewCanvasPanel()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			c.Push(fmt.Sprintf("<p>a%d</p>", i))
		}
		done <- struct{}{}
	}()
	for i := 0; i < 50; i++ {
		c.Push(fmt.Sprintf("<p>b%d</p>", i))
	}
	<-done
	// No panic = concurrent safety confirmed.
	if c.Len() == 0 {
		t.Error("expected frames after concurrent push")
	}
}
