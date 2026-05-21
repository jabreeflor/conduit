package gui

import (
	"errors"
	"testing"
)

// fakePersister is an in-memory Persister for tests.
type fakePersister struct {
	data    string
	loadErr error
	saveErr error
	saves   int
}

func (p *fakePersister) Load() (string, error) {
	if p.loadErr != nil {
		return "", p.loadErr
	}
	return p.data, nil
}

func (p *fakePersister) Save(text string) error {
	if p.saveErr != nil {
		return p.saveErr
	}
	p.data = text
	p.saves++
	return nil
}

func TestMemoryEditor_reloadHydratesBuffer(t *testing.T) {
	p := &fakePersister{data: "# SOUL\n\ncuriosity > certainty\n"}
	e := NewMemoryEditor(DocSoul, p)
	if err := e.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := e.Buffer(); got != p.data {
		t.Fatalf("Buffer mismatch: %q vs %q", got, p.data)
	}
	if e.Dirty() {
		t.Fatal("buffer should be clean immediately after Reload")
	}
}

func TestMemoryEditor_dirtyAfterEdit(t *testing.T) {
	p := &fakePersister{data: "alpha"}
	e := NewMemoryEditor(DocUser, p)
	if err := e.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	e.SetBuffer("beta")
	if !e.Dirty() {
		t.Fatal("expected Dirty after SetBuffer")
	}
}

func TestMemoryEditor_saveAdoptsBaseline(t *testing.T) {
	p := &fakePersister{data: "alpha"}
	e := NewMemoryEditor(DocUser, p)
	_ = e.Reload()
	e.SetBuffer("beta")

	if err := e.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if p.saves != 1 {
		t.Fatalf("expected 1 save call, got %d", p.saves)
	}
	if p.data != "beta" {
		t.Fatalf("persisted data mismatch: %q", p.data)
	}
	if e.Dirty() {
		t.Fatal("Dirty should clear after successful Save")
	}
}

func TestMemoryEditor_saveNoopWhenClean(t *testing.T) {
	p := &fakePersister{data: "alpha"}
	e := NewMemoryEditor(DocUser, p)
	_ = e.Reload()

	if err := e.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if p.saves != 0 {
		t.Fatalf("Save should not write when clean, got %d writes", p.saves)
	}
}

func TestMemoryEditor_revertRestoresBaseline(t *testing.T) {
	p := &fakePersister{data: "alpha"}
	e := NewMemoryEditor(DocUser, p)
	_ = e.Reload()
	e.SetBuffer("beta")
	e.Revert()
	if e.Buffer() != "alpha" {
		t.Fatalf("Revert did not restore baseline: %q", e.Buffer())
	}
	if e.Dirty() {
		t.Fatal("Dirty after Revert")
	}
}

func TestMemoryEditor_loadError(t *testing.T) {
	want := errors.New("disk on fire")
	p := &fakePersister{loadErr: want}
	e := NewMemoryEditor(DocSoul, p)
	if err := e.Reload(); !errors.Is(err, want) {
		t.Fatalf("Reload error: want %v, got %v", want, err)
	}
	if e.LoadError() == nil {
		t.Fatal("LoadError should remember the failure")
	}
}

func TestMemoryEditor_saveErrorKeepsBufferDirty(t *testing.T) {
	want := errors.New("read-only fs")
	p := &fakePersister{data: "alpha", saveErr: want}
	e := NewMemoryEditor(DocUser, p)
	_ = e.Reload()
	e.SetBuffer("beta")
	if err := e.Save(); !errors.Is(err, want) {
		t.Fatalf("Save error: want %v, got %v", want, err)
	}
	if !e.Dirty() {
		t.Fatal("buffer should remain Dirty when Save fails")
	}
}

func TestMemoryEditor_cursorClamping(t *testing.T) {
	p := &fakePersister{data: "abcde"}
	e := NewMemoryEditor(DocSoul, p)
	_ = e.Reload()

	e.SetCursor(-5)
	if got := e.Cursor(); got != 0 {
		t.Fatalf("negative cursor not clamped, got %d", got)
	}
	e.SetCursor(99)
	if got := e.Cursor(); got != len("abcde") {
		t.Fatalf("over-range cursor not clamped to %d, got %d", len("abcde"), got)
	}

	// SetBuffer must reclamp the cursor when it would now be out of range.
	e.SetCursor(len("abcde"))
	e.SetBuffer("ab")
	if got := e.Cursor(); got != len("ab") {
		t.Fatalf("cursor not reclamped after shrinking buffer, got %d", got)
	}
}

func TestMemoryEditor_noPersister(t *testing.T) {
	e := NewMemoryEditor(DocSoul, nil)
	if err := e.Reload(); err == nil {
		t.Fatal("Reload should error without a persister")
	}
	if err := e.Save(); err == nil {
		t.Fatal("Save should error without a persister")
	}
}
