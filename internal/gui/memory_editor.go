package gui

import (
	"errors"
	"sync"
)

// MemoryEditor is the view-model for the SOUL.md / USER.md editor surface
// in the GUI Memory tab (PRD §11.2 + §6.13). It buffers an in-progress
// edit so the user can revise without writing back to disk on every keystroke,
// tracks the dirty flag, and exposes Save/Revert hooks the agent panel
// chrome can wire to keyboard shortcuts.
//
// The editor knows nothing about the on-disk representation — persistence
// is owned by the memory package. Callers register a Persister that knows
// how to load and write the specific document (SOUL.md or USER.md).
//
// Safe for concurrent reads; writes serialise on a single mutex.
type MemoryEditor struct {
	mu sync.RWMutex

	doc       MemoryDoc
	persister Persister

	loaded   string // text last read from disk; baseline for the dirty check
	buffered string // user's in-progress edit
	cursor   int    // byte offset within buffered
	loadErr  error
}

// MemoryDoc names which canonical document the editor is bound to. The
// GUI mounts one MemoryEditor per document; the value is informational
// and surfaces in headings/breadcrumbs.
type MemoryDoc int

const (
	DocSoul MemoryDoc = iota // ~/.conduit/SOUL.md
	DocUser                  // ~/.conduit/USER.md
)

// Persister abstracts the read/write path for a memory document. The
// editor calls Load when it is mounted and Save when the user commits.
//
// Implementations live in the memory package — the editor never imports
// it, keeping the GUI package free of disk-layout knowledge.
type Persister interface {
	Load() (string, error)
	Save(text string) error
}

// NewMemoryEditor returns an editor bound to the given document and
// persister. The buffer is empty until Reload is called; the GUI typically
// reloads on mount and on every external file-change notification.
func NewMemoryEditor(doc MemoryDoc, p Persister) *MemoryEditor {
	return &MemoryEditor{doc: doc, persister: p}
}

// Doc reports which document this editor is bound to.
func (e *MemoryEditor) Doc() MemoryDoc {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.doc
}

// Reload re-reads the document via the persister and resets the buffer.
// Any unsaved edit is discarded — callers must check Dirty() and warn the
// user before invoking Reload over an in-progress edit.
func (e *MemoryEditor) Reload() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.persister == nil {
		e.loadErr = errors.New("gui: memory editor has no persister")
		return e.loadErr
	}
	text, err := e.persister.Load()
	if err != nil {
		e.loadErr = err
		return err
	}
	e.loaded = text
	e.buffered = text
	if e.cursor > len(text) {
		e.cursor = len(text)
	}
	e.loadErr = nil
	return nil
}

// LoadError returns the error from the most recent Reload, or nil.
func (e *MemoryEditor) LoadError() error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.loadErr
}

// Buffer returns the current in-progress edit.
func (e *MemoryEditor) Buffer() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.buffered
}

// SetBuffer replaces the in-progress edit. The cursor is clamped to the
// end of the new text if it would otherwise be out of range.
func (e *MemoryEditor) SetBuffer(text string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.buffered = text
	if e.cursor > len(text) {
		e.cursor = len(text)
	}
}

// Cursor returns the current byte-offset cursor position.
func (e *MemoryEditor) Cursor() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cursor
}

// SetCursor moves the cursor; out-of-range values are clamped.
func (e *MemoryEditor) SetCursor(offset int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if offset < 0 {
		offset = 0
	}
	if offset > len(e.buffered) {
		offset = len(e.buffered)
	}
	e.cursor = offset
}

// Dirty reports whether the buffer has diverged from the loaded baseline.
func (e *MemoryEditor) Dirty() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.buffered != e.loaded
}

// Save persists the buffer via the persister and adopts it as the new
// baseline. A no-op when the buffer is clean — callers can safely bind
// Save to a save-on-blur event without thrashing the disk.
func (e *MemoryEditor) Save() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.persister == nil {
		return errors.New("gui: memory editor has no persister")
	}
	if e.buffered == e.loaded {
		return nil
	}
	if err := e.persister.Save(e.buffered); err != nil {
		return err
	}
	e.loaded = e.buffered
	return nil
}

// Revert drops the in-progress edit and restores the loaded baseline.
func (e *MemoryEditor) Revert() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.buffered = e.loaded
	if e.cursor > len(e.buffered) {
		e.cursor = len(e.buffered)
	}
}
