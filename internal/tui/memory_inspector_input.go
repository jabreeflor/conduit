package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jabreeflor/conduit/internal/memory"
)

// MemoryController is the small slice of engine functionality the memory
// inspector needs. The interface keeps the TUI free of a hard dependency on
// the core engine — tests inject a fake implementation.
type MemoryController interface {
	SearchMemory(ctx context.Context, query string) ([]memory.Entry, error)
	DeleteMemory(ctx context.Context, id string) error
	PruneMemory(ctx context.Context, query string) ([]string, error)
	SetMemoryPinned(ctx context.Context, id string, pinned bool) error
}

// openMemoryInspector loads the latest entry list and switches the model into
// inspector mode. Failures surface in the inspector footer; we never crash
// the TUI just because the memory layer hiccuped.
func (m Model) openMemoryInspector() Model {
	if m.inspector == nil {
		m.inspector = NewMemoryInspector()
	}
	m.inspectorOpen = true
	m = m.refreshInspectorEntries("")
	return m
}

// closeMemoryInspector hides the inspector and returns to the normal panels.
func (m Model) closeMemoryInspector() Model {
	m.inspectorOpen = false
	if m.inspector != nil {
		m.inspector.StopFilterEdit()
		m.inspector.CancelPrompt()
	}
	return m
}

// refreshInspectorEntries reloads entries from the controller using query
// (empty = all). Updates the inspector's entry list in place.
func (m Model) refreshInspectorEntries(query string) Model {
	if m.memoryController == nil || m.inspector == nil {
		return m
	}
	entries, err := m.memoryController.SearchMemory(context.Background(), query)
	if err != nil {
		m.inspector.SetMessage("error: " + err.Error())
		return m
	}
	m.inspector.SetEntries(entries)
	return m
}

// handleInspectorKey routes key events while the inspector is open. Returns
// the (possibly mutated) model and a flag indicating whether the key was
// consumed. Anything not consumed falls through to the normal Update path.
func (m Model) handleInspectorKey(msg tea.KeyMsg) (Model, bool) {
	if !m.inspectorOpen || m.inspector == nil {
		return m, false
	}

	// While the filter input has focus, most keys feed the filter directly.
	if m.inspector.Editing() {
		switch msg.Type {
		case tea.KeyEsc, tea.KeyEnter:
			m.inspector.StopFilterEdit()
		case tea.KeyBackspace:
			m.inspector.BackspaceFilter()
		case tea.KeyRunes, tea.KeySpace:
			for _, r := range msg.Runes {
				m.inspector.AppendFilter(r)
			}
		}
		return m, true
	}

	// Confirmation modals: y / n / esc.
	switch m.inspector.Mode() {
	case inspectorConfirm:
		switch msg.String() {
		case "y", "Y":
			m = m.confirmDelete()
		case "n", "N", "esc":
			m.inspector.CancelPrompt()
		}
		return m, true
	case inspectorPrune:
		switch msg.String() {
		case "y", "Y":
			m = m.confirmPrune()
		case "n", "N", "esc":
			m.inspector.CancelPrompt()
		}
		return m, true
	case inspectorDetail:
		switch msg.String() {
		case "esc":
			m.inspector.CloseDetail()
		case "p":
			m = m.togglePinSelected()
		case "d":
			m.inspector.PromptDelete()
		}
		return m, true
	}

	// List mode keystrokes.
	switch msg.String() {
	case "esc":
		m = m.closeMemoryInspector()
	case "up", "k":
		m.inspector.CursorUp()
	case "down", "j":
		m.inspector.CursorDown()
	case "home", "g":
		m.inspector.CursorHome()
	case "end", "G":
		m.inspector.CursorEnd()
	case "/":
		m.inspector.StartFilterEdit()
	case "enter":
		m.inspector.OpenDetail()
	case "p":
		m = m.togglePinSelected()
	case "d":
		m.inspector.PromptDelete()
	case "P":
		m.inspector.PromptPrune()
	case "r":
		m = m.refreshInspectorEntries("")
		m.inspector.SetMessage("reloaded")
	}
	return m, true
}

func (m Model) confirmDelete() Model {
	entry, ok := m.inspector.Selected()
	if !ok || m.memoryController == nil {
		m.inspector.CancelPrompt()
		return m
	}
	if err := m.memoryController.DeleteMemory(context.Background(), entry.ID); err != nil {
		m.inspector.SetMessage("delete failed: " + err.Error())
	} else {
		m.inspector.SetMessage(fmt.Sprintf("deleted %s", truncate(entry.Title, 40)))
	}
	m.inspector.CancelPrompt()
	return m.refreshInspectorEntries("")
}

func (m Model) confirmPrune() Model {
	if m.memoryController == nil {
		m.inspector.CancelPrompt()
		return m
	}
	removed, err := m.memoryController.PruneMemory(context.Background(), m.inspector.Filter())
	if err != nil {
		m.inspector.SetMessage("prune failed: " + err.Error())
	} else {
		m.inspector.SetMessage(fmt.Sprintf("pruned %d entries", len(removed)))
	}
	m.inspector.CancelPrompt()
	return m.refreshInspectorEntries("")
}

func (m Model) togglePinSelected() Model {
	entry, ok := m.inspector.Selected()
	if !ok || m.memoryController == nil {
		return m
	}
	want := !entry.Pinned
	if err := m.memoryController.SetMemoryPinned(context.Background(), entry.ID, want); err != nil {
		m.inspector.SetMessage("pin failed: " + err.Error())
		return m
	}
	if want {
		m.inspector.SetMessage("pinned: " + truncate(entry.Title, 40))
	} else {
		m.inspector.SetMessage("unpinned: " + truncate(entry.Title, 40))
	}
	return m.refreshInspectorEntries("")
}
