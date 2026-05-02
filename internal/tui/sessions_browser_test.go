package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jabreeflor/conduit/internal/sessions"
)

func newPopulatedStore(t *testing.T) (*sessions.Store, sessions.Turn) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "sessions")
	store, err := sessions.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	sess, err := store.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	a, err := store.Append(sess, sessions.Turn{Role: "user", Content: "hello browser"})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := store.Append(sess, sessions.Turn{Role: "assistant", Content: "hi", ParentID: a.ID}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	return store, a
}

func TestSessionsBrowserViewIncludesSessionRow(t *testing.T) {
	store, _ := newPopulatedStore(t)
	tree, err := sessions.BuildTree(store)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	browser := NewSessionsBrowser(tree)
	out := browser.View()
	if !strings.Contains(out, "Session Tree Browser") {
		t.Errorf("missing header in view: %q", out)
	}
	if !strings.Contains(out, "user") {
		t.Errorf("expected turn role label in view: %q", out)
	}
}

func TestSessionsBrowserNavigationAndForkAction(t *testing.T) {
	store, turnA := newPopulatedStore(t)
	tree, _ := sessions.BuildTree(store)
	browser := NewSessionsBrowser(tree)

	// Walk down until we land on the user turn.
	for i := 0; i < 5 && !rowIsTurn(browser, turnA.ID); i++ {
		next, _ := browser.Update(tea.KeyMsg{Type: tea.KeyDown})
		browser = next
	}
	if !rowIsTurn(browser, turnA.ID) {
		t.Fatalf("could not navigate to user turn; rows=%d cursor=%d", len(browser.rows), browser.cursor)
	}
	// Press 'f' to fork.
	next, _ := browser.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	browser = next
	if !browser.IsClosed() {
		t.Fatal("browser should close after fork action")
	}
	sel := browser.Selected()
	if sel.Action != ActionFork {
		t.Errorf("action: got %v want ActionFork", sel.Action)
	}
	if sel.TurnID != turnA.ID {
		t.Errorf("turn id: got %q want %q", sel.TurnID, turnA.ID)
	}
}

func TestSessionsBrowserQuitDismissesCleanly(t *testing.T) {
	store, _ := newPopulatedStore(t)
	tree, _ := sessions.BuildTree(store)
	browser := NewSessionsBrowser(tree)

	next, _ := browser.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	browser = next
	if !browser.IsClosed() {
		t.Fatal("browser should close on q")
	}
	if browser.Selected().Action != ActionNone {
		t.Errorf("expected ActionNone, got %v", browser.Selected().Action)
	}
}

func TestSessionsBrowserForkOnSessionRowReportsError(t *testing.T) {
	store, _ := newPopulatedStore(t)
	tree, _ := sessions.BuildTree(store)
	browser := NewSessionsBrowser(tree)
	// Cursor starts at row 0 = session header; fork should be rejected.
	next, _ := browser.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	browser = next
	if browser.IsClosed() {
		t.Fatal("browser should not close when fork target is a session header")
	}
	if browser.errText == "" {
		t.Errorf("expected an error message, got empty")
	}
}

func rowIsTurn(b SessionsBrowser, turnID string) bool {
	row, ok := b.currentRow()
	return ok && row.kind == sessions.NodeKindTurn && row.turnID == turnID
}
