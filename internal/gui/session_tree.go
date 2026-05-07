package gui

import (
	"sync"

	"github.com/jabreeflor/conduit/internal/sessions"
)

// SessionTree is the view-model for the GUI session tree browser (PRD §11.2).
//
// It wraps a sessions.Tree with the UI state the browser needs: expand/collapse
// per session, a selected turn, and a pending fork intent. The underlying
// sessions.Tree is authoritative — this struct never mutates it; it only
// records the user's navigation and the action they are about to take.
//
// Safe for concurrent use: the WebSocket push pump that refreshes the tree
// runs on a different goroutine from the input handler that updates selection.
type SessionTree struct {
	mu sync.RWMutex

	tree *sessions.Tree

	expanded map[string]bool // sessionID -> expanded?
	selected string          // turn ID; "" if none
	fork     *ForkPlan
}

// ForkPlan captures the user's intent to fork from a specific turn. The plan
// is staged before commit so the GUI can show a confirmation sheet ("fork
// from turn N — copy memory? replay first?") and the workflow engine can
// execute the chosen semantics.
//
// Mode controls how the new session relates to the parent:
//   - ForkSnapshot: copy parent state up to and including the source turn,
//     new session evolves independently (no shared writes).
//   - ForkReplay:   re-execute turns 1..N to rebuild state deterministically;
//     useful when the parent's tool calls produced side-effects you want to
//     reproduce against a new model.
//   - ForkReference: new session points back at parent turns up to N
//     (copy-on-write); cheapest, but parent edits leak into the fork until
//     a divergent write occurs.
type ForkPlan struct {
	SourceTurnID string
	Mode         ForkMode
	CopyMemory   bool // include the parent's memory snapshot
	NewSessionID string
}

// ForkMode is the semantics chosen for ForkPlan.Mode.
//
//   - ForkSnapshot: copy parent state at the source turn; no later coupling
//   - ForkReplay:   re-execute turns 1..N to rebuild state deterministically
//   - ForkReference: child references parent turns up to N (copy-on-write)
type ForkMode int

const (
	ForkModeUnspecified ForkMode = iota
	ForkSnapshot
	ForkReplay
	ForkReference
)

// DefaultForkMode returns the ForkMode used when the user activates "fork
// from turn" without explicitly picking a mode. Snapshot is the default:
// isolation by default avoids surprising parent-session leakage when the
// user is exploring a divergent path.
func DefaultForkMode() ForkMode {
	return ForkSnapshot
}

// NewSessionTree returns a view-model wrapping tree. The tree may be nil
// (the GUI shows an empty state until the first sessions.BuildTree pump).
func NewSessionTree(tree *sessions.Tree) *SessionTree {
	return &SessionTree{
		tree:     tree,
		expanded: map[string]bool{},
	}
}

// SetTree swaps the underlying forest. Selection and expand state are
// preserved by id, so a refresh that re-builds the tree does not collapse
// the browser.
func (s *SessionTree) SetTree(tree *sessions.Tree) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tree = tree
}

// Tree returns the wrapped forest, or nil before the first SetTree.
func (s *SessionTree) Tree() *sessions.Tree {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tree
}

// Toggle flips the expand state for a session id. Sessions default to
// collapsed; toggling an unknown id expands it.
func (s *SessionTree) Toggle(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expanded[sessionID] = !s.expanded[sessionID]
}

// IsExpanded reports whether a session is currently expanded.
func (s *SessionTree) IsExpanded(sessionID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.expanded[sessionID]
}

// Select sets the highlighted turn id. Pass "" to clear the selection.
func (s *SessionTree) Select(turnID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selected = turnID
}

// Selected returns the currently selected turn id, or "" if none.
func (s *SessionTree) Selected() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.selected
}

// SelectedTurn returns the *sessions.Turn pointed at by Selected, or nil if
// no selection or the tree has not yet been hydrated.
func (s *SessionTree) SelectedTurn() *sessions.Turn {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.selected == "" || s.tree == nil {
		return nil
	}
	n := s.tree.LookupTurn(s.selected)
	if n == nil || n.Kind != sessions.NodeKindTurn {
		return nil
	}
	t := n.Turn
	return &t
}

// StageFork records a pending ForkPlan rooted at the selected turn. Returns
// false if no turn is selected. The plan is held until either CommitFork is
// called (caller has executed it) or CancelFork.
func (s *SessionTree) StageFork(mode ForkMode, copyMemory bool, newSessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.selected == "" {
		return false
	}
	if mode == ForkModeUnspecified {
		mode = DefaultForkMode()
	}
	s.fork = &ForkPlan{
		SourceTurnID: s.selected,
		Mode:         mode,
		CopyMemory:   copyMemory,
		NewSessionID: newSessionID,
	}
	return true
}

// PendingFork returns a snapshot of the staged ForkPlan, or nil if none.
func (s *SessionTree) PendingFork() *ForkPlan {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.fork == nil {
		return nil
	}
	p := *s.fork
	return &p
}

// CommitFork clears the staged plan; the caller is responsible for actually
// applying it via the sessions package.
func (s *SessionTree) CommitFork() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fork = nil
}

// CancelFork drops the staged plan without applying it.
func (s *SessionTree) CancelFork() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fork = nil
}
