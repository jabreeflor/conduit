package gui

import "sync"

// CanvasPanel is the view-model for the Canvas HTML panel shown in the GUI
// main content area. Agents push HTML frames via [[canvas: html]] reply tags;
// the panel maintains a navigation history so users can step back through
// previous renders. The WKWebView rendering layer reads HTML() and Visible()
// to decide what to inject and whether to show the panel.
//
// Sandboxing note: the WKWebView must be configured with a restrictive
// WKWebpagePreferences — disable JavaScript access to the file system,
// restrict navigation to about:blank origins only, and set
// allowsContentJavaScript=false unless the agent explicitly requests
// scripting. See PRD §11.2.
//
// CanvasPanel is safe for concurrent use: the replytag dispatcher and the
// UI event loop run on different goroutines.
type CanvasPanel struct {
	mu sync.RWMutex

	history []string // HTML frames, oldest → newest
	cursor  int      // index of the displayed frame; -1 when empty
	visible bool     // whether this panel is the active main content view
}

// NewCanvasPanel returns an empty, hidden canvas panel.
func NewCanvasPanel() *CanvasPanel {
	return &CanvasPanel{cursor: -1}
}

// Push appends an HTML frame emitted by a [[canvas: html]] tag event and
// makes it the active frame. If the cursor is not at the newest frame (the
// user navigated back), frames ahead of the cursor are discarded before
// appending — consistent with browser-style navigation. The panel becomes
// visible automatically on the first push.
func (c *CanvasPanel) Push(html string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Discard forward history when a new frame arrives mid-navigation.
	if c.cursor >= 0 && c.cursor < len(c.history)-1 {
		c.history = c.history[:c.cursor+1]
	}
	c.history = append(c.history, html)
	c.cursor = len(c.history) - 1
	c.visible = true
}

// HTML returns the HTML of the currently displayed frame, or "" if the panel
// has no frames yet.
func (c *CanvasPanel) HTML() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentHTML()
}

// Visible reports whether the canvas panel is the active main content view.
// The rendering layer uses this to switch the main content area.
func (c *CanvasPanel) Visible() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.visible
}

// Show makes the canvas panel the active main content view.
func (c *CanvasPanel) Show() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.visible = true
}

// Hide deactivates the canvas panel, returning the main content area to the
// previous view (screenshot stream, workflow DAG, etc.).
func (c *CanvasPanel) Hide() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.visible = false
}

// Back navigates to the previous HTML frame. Does nothing if already at the
// oldest frame.
func (c *CanvasPanel) Back() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cursor > 0 {
		c.cursor--
	}
}

// Forward navigates to the next HTML frame. Does nothing if already at the
// newest frame.
func (c *CanvasPanel) Forward() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cursor < len(c.history)-1 {
		c.cursor++
	}
}

// Reload returns the HTML of the currently displayed frame for re-injection
// into the WKWebView. Returns "" if the panel is empty.
func (c *CanvasPanel) Reload() string {
	return c.HTML()
}

// CanGoBack reports whether Back() would change the displayed frame.
func (c *CanvasPanel) CanGoBack() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cursor > 0
}

// CanGoForward reports whether Forward() would change the displayed frame.
func (c *CanvasPanel) CanGoForward() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cursor >= 0 && c.cursor < len(c.history)-1
}

// Len returns the number of HTML frames held in history.
func (c *CanvasPanel) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.history)
}

// Clear removes all HTML frames and hides the panel.
func (c *CanvasPanel) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.history = nil
	c.cursor = -1
	c.visible = false
}

// currentHTML returns the active HTML. Must be called with at least a read
// lock held.
func (c *CanvasPanel) currentHTML() string {
	if c.cursor < 0 || c.cursor >= len(c.history) {
		return ""
	}
	return c.history[c.cursor]
}
