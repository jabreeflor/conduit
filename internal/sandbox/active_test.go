package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestActiveUnsetReturnsErrNoActive(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Active(); !errors.Is(err, ErrNoActive) {
		t.Fatalf("Active on fresh root err=%v, want ErrNoActive", err)
	}
}

func TestSetActiveRequiresExistingSandbox(t *testing.T) {
	m := newTestManager(t)
	if err := m.SetActive("ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetActive missing err=%v, want ErrNotFound", err)
	}
}

func TestSetActiveRejectsInvalidName(t *testing.T) {
	m := newTestManager(t)
	if err := m.SetActive("Bad Name"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("SetActive bad name err=%v, want ErrInvalidName", err)
	}
}

func TestSetActiveAndActiveRoundTrip(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Create("alpha", CreateOptions{}); err != nil {
		t.Fatalf("Create alpha: %v", err)
	}
	if err := m.SetActive("alpha"); err != nil {
		t.Fatalf("SetActive alpha: %v", err)
	}
	got, err := m.Active()
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if got != "alpha" {
		t.Fatalf("Active = %q, want alpha", got)
	}
}

func TestSetActiveOverwritesPrevious(t *testing.T) {
	m := newTestManager(t)
	for _, n := range []string{"alpha", "beta"} {
		if _, err := m.Create(n, CreateOptions{}); err != nil {
			t.Fatalf("Create %s: %v", n, err)
		}
	}
	if err := m.SetActive("alpha"); err != nil {
		t.Fatalf("SetActive alpha: %v", err)
	}
	if err := m.SetActive("beta"); err != nil {
		t.Fatalf("SetActive beta: %v", err)
	}
	got, err := m.Active()
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if got != "beta" {
		t.Fatalf("Active = %q, want beta after switch", got)
	}
}

func TestClearActive(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Create("alpha", CreateOptions{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := m.SetActive("alpha"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if err := m.ClearActive(); err != nil {
		t.Fatalf("ClearActive: %v", err)
	}
	if _, err := m.Active(); !errors.Is(err, ErrNoActive) {
		t.Fatalf("Active after Clear err=%v, want ErrNoActive", err)
	}
	// Idempotent: clearing again is fine.
	if err := m.ClearActive(); err != nil {
		t.Fatalf("ClearActive idempotent: %v", err)
	}
}

func TestActiveStaleNameTreatedAsUnset(t *testing.T) {
	// A pointer file that contains a name we wouldn't accept (e.g. uppercase
	// or a path traversal attempt) should read as ErrNoActive rather than
	// poisoning every CLI call.
	m := newTestManager(t)
	if err := os.MkdirAll(m.sandboxesPath(), 0o755); err != nil {
		t.Fatalf("mkdir sandboxes: %v", err)
	}
	pointer := filepath.Join(m.sandboxesPath(), ".active")
	if err := os.WriteFile(pointer, []byte("../etc/passwd\n"), 0o644); err != nil {
		t.Fatalf("write stale pointer: %v", err)
	}
	if _, err := m.Active(); !errors.Is(err, ErrNoActive) {
		t.Fatalf("Active on stale pointer err=%v, want ErrNoActive", err)
	}
}

func TestSwitchOpensAndActivates(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Create("gamma", CreateOptions{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	ws, err := m.Switch("gamma")
	if err != nil {
		t.Fatalf("Switch: %v", err)
	}
	if ws.Name() != "gamma" {
		t.Fatalf("Switch returned name=%q, want gamma", ws.Name())
	}
	got, err := m.Active()
	if err != nil {
		t.Fatalf("Active after Switch: %v", err)
	}
	if got != "gamma" {
		t.Fatalf("Active = %q, want gamma", got)
	}
}

func TestSwitchMissingReturnsNotFound(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Switch("ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Switch missing err=%v, want ErrNotFound", err)
	}
}

func TestDestroyClearsActivePointer(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Create("doomed", CreateOptions{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := m.SetActive("doomed"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if err := m.Destroy("doomed"); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if _, err := m.Active(); !errors.Is(err, ErrNoActive) {
		t.Fatalf("Active after Destroy of active err=%v, want ErrNoActive", err)
	}
}

func TestListMarksActiveSandbox(t *testing.T) {
	m := newTestManager(t)
	for _, n := range []string{"alpha", "beta", "gamma"} {
		if _, err := m.Create(n, CreateOptions{}); err != nil {
			t.Fatalf("Create %s: %v", n, err)
		}
	}
	if err := m.SetActive("beta"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	infos, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, info := range infos {
		want := info.Name == "beta"
		if info.Active != want {
			t.Fatalf("info[%s].Active = %v, want %v", info.Name, info.Active, want)
		}
	}
}
