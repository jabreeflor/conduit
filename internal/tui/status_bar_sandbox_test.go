package tui

import (
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// TestStatusBarShowsActiveSandbox locks in PRD §15.7: the status bar must
// surface the active sandbox name when one is set.
func TestStatusBarShowsActiveSandbox(t *testing.T) {
	m := newModel("claude-haiku-4-5", contracts.FirstRunSetupSnapshot{}, nil).
		WithActiveSandbox("plugin-dev")
	m.width = 120
	out := m.renderStatusBar()
	if !strings.Contains(out, "plugin-dev") {
		t.Errorf("status bar missing active sandbox: %q", out)
	}
}

// TestStatusBarHidesSandboxSegmentWhenUnset ensures the segment is silently
// omitted when no sandbox is active so users without sandboxing don't see a
// dangling marker.
func TestStatusBarHidesSandboxSegmentWhenUnset(t *testing.T) {
	m := newModel("claude-haiku-4-5", contracts.FirstRunSetupSnapshot{}, nil)
	m.width = 120
	out := m.renderStatusBar()
	if strings.Contains(out, "⛶") {
		t.Errorf("status bar shouldn't show sandbox glyph when unset: %q", out)
	}
}
