package gui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// WorktreeTab is the GUI view-model for the coding Worktree tab. It stores the
// latest session snapshot and renders a compact status/history view that native
// frontends can mirror without knowing git internals.
type WorktreeTab struct {
	mu    sync.RWMutex
	state contracts.CodingWorktreeState
}

// NewWorktreeTab returns an empty Worktree tab.
func NewWorktreeTab() *WorktreeTab { return &WorktreeTab{} }

// Update replaces the tab state with the latest coding-session snapshot.
func (t *WorktreeTab) Update(state contracts.CodingWorktreeState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	state.History = append([]contracts.CodingWorktreeEvent(nil), state.History...)
	t.state = state
}

// State returns a defensive copy for native frontends.
func (t *WorktreeTab) State() contracts.CodingWorktreeState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := t.state
	out.History = append([]contracts.CodingWorktreeEvent(nil), t.state.History...)
	return out
}

// View returns the text body used by lightweight GUI tests and fallback panes.
func (t *WorktreeTab) View() string {
	state := t.State()
	if state.SessionID == "" {
		return "Worktree\nNo coding session attached."
	}

	var sb strings.Builder
	sb.WriteString("Worktree\n")
	fmt.Fprintf(&sb, "Session: %s\n", state.SessionID)
	if state.WorktreeActive {
		fmt.Fprintf(&sb, "Status: active on %s\n", fallback(state.WorktreeBranch, "-"))
		fmt.Fprintf(&sb, "Path: %s\n", fallback(state.WorktreePath, "-"))
	} else {
		fmt.Fprintf(&sb, "Status: repository on %s\n", fallback(state.Branch, "-"))
	}
	fmt.Fprintf(&sb, "CWD: %s\n", fallback(state.CWD, "-"))
	if len(state.History) == 0 {
		sb.WriteString("History: none")
		return sb.String()
	}
	sb.WriteString("History:\n")
	for _, ev := range state.History {
		fmt.Fprintf(&sb, "- %s %s %s\n", ev.At.Format("15:04:05"), ev.Action, fallback(ev.Branch, ev.Path))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func fallback(v, alt string) string {
	if v == "" {
		return alt
	}
	return v
}
