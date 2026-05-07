package gui

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/sessions"
)

func newTreeWithOneSession(t *testing.T) (*sessions.Tree, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := sessions.NewStore(filepath.Join(dir, ".conduit"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	sess, err := store.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	turnA, err := store.Append(sess, sessions.Turn{
		Role: "user", Content: "hello", At: time.Now(),
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := store.Append(sess, sessions.Turn{
		ParentID: turnA.ID,
		Role:     "assistant", Content: "hi", At: time.Now(),
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	tree, err := sessions.BuildTree(store)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	return tree, turnA.ID
}

func TestNewSessionTree_emptyByDefault(t *testing.T) {
	st := NewSessionTree(nil)
	if st.Tree() != nil {
		t.Fatal("expected nil tree on construction")
	}
	if st.Selected() != "" {
		t.Fatal("expected empty selection on construction")
	}
	if st.PendingFork() != nil {
		t.Fatal("expected no pending fork on construction")
	}
}

func TestSessionTree_toggleAndExpand(t *testing.T) {
	st := NewSessionTree(nil)
	if st.IsExpanded("s1") {
		t.Fatal("sessions should default to collapsed")
	}
	st.Toggle("s1")
	if !st.IsExpanded("s1") {
		t.Fatal("expected expanded after first toggle")
	}
	st.Toggle("s1")
	if st.IsExpanded("s1") {
		t.Fatal("expected collapsed after second toggle")
	}
}

func TestSessionTree_selectAndLookup(t *testing.T) {
	tree, turnID := newTreeWithOneSession(t)
	st := NewSessionTree(tree)

	if st.SelectedTurn() != nil {
		t.Fatal("expected no SelectedTurn before Select")
	}
	st.Select(turnID)
	if got := st.Selected(); got != turnID {
		t.Fatalf("Selected: want %q, got %q", turnID, got)
	}
	if turn := st.SelectedTurn(); turn == nil || turn.ID != turnID {
		t.Fatalf("SelectedTurn missing or wrong id: %+v", turn)
	}
	st.Select("")
	if st.SelectedTurn() != nil {
		t.Fatal("expected nil after clearing selection")
	}
}

func TestSessionTree_stageForkWithoutSelection(t *testing.T) {
	st := NewSessionTree(nil)
	if st.StageFork(ForkSnapshot, true, "new-session") {
		t.Fatal("StageFork should refuse when no turn is selected")
	}
	if st.PendingFork() != nil {
		t.Fatal("no plan should be staged when StageFork rejected")
	}
}

func TestSessionTree_stageAndCommitFork(t *testing.T) {
	tree, turnID := newTreeWithOneSession(t)
	st := NewSessionTree(tree)
	st.Select(turnID)

	if !st.StageFork(ForkSnapshot, true, "child-session") {
		t.Fatal("StageFork failed despite a valid selection")
	}
	plan := st.PendingFork()
	if plan == nil {
		t.Fatal("expected pending fork after StageFork")
	}
	if plan.SourceTurnID != turnID {
		t.Fatalf("SourceTurnID: want %q, got %q", turnID, plan.SourceTurnID)
	}
	if plan.Mode != ForkSnapshot {
		t.Fatalf("Mode: want ForkSnapshot, got %v", plan.Mode)
	}
	if !plan.CopyMemory {
		t.Fatal("CopyMemory should round-trip true")
	}
	if plan.NewSessionID != "child-session" {
		t.Fatalf("NewSessionID: got %q", plan.NewSessionID)
	}

	st.CommitFork()
	if st.PendingFork() != nil {
		t.Fatal("CommitFork should clear the staged plan")
	}
}

func TestSessionTree_cancelFork(t *testing.T) {
	tree, turnID := newTreeWithOneSession(t)
	st := NewSessionTree(tree)
	st.Select(turnID)
	st.StageFork(ForkReplay, false, "x")

	st.CancelFork()
	if st.PendingFork() != nil {
		t.Fatal("CancelFork should drop the staged plan")
	}
}

func TestSessionTree_unspecifiedFalsesBackToDefault(t *testing.T) {
	tree, turnID := newTreeWithOneSession(t)
	st := NewSessionTree(tree)
	st.Select(turnID)

	// Passing ForkModeUnspecified forces the view-model to substitute the
	// project default. Today the default is unset (TODO in DefaultForkMode);
	// this test pins the contract so changing the default is a deliberate
	// edit and not silent drift.
	st.StageFork(ForkModeUnspecified, false, "x")
	plan := st.PendingFork()
	if plan == nil {
		t.Fatal("expected staged plan")
	}
	if plan.Mode != DefaultForkMode() {
		t.Fatalf("Mode should fall back to DefaultForkMode, got %v", plan.Mode)
	}
}
