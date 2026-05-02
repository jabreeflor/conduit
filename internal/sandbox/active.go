// active.go — PRD §15.7  Active sandbox pointer
//
// The active sandbox is a process-spanning selection: future `conduit` runs
// open it by default, and the TUI/GUI status bar reflects it. We persist the
// selection in a single line of text at <root>/sandboxes/.active so it
// survives across CLI invocations without any external state store.
//
// The active pointer is decoupled from any open Workspace handle: switching
// is just a pointer write — no I/O on the workspace itself — and the on-disk
// representation can be inspected with `cat`.

package sandbox

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// activeFilePath returns the absolute path to the active-pointer file.
func (m *Manager) activeFilePath() string {
	return filepath.Join(m.sandboxesPath(), "."+activeFileName)
}

// Active returns the name of the currently-active sandbox. ErrNoActive is
// returned when no pointer file exists yet, which lets callers errors.Is-test
// the "first run / unset" case without parsing strings.
func (m *Manager) Active() (string, error) {
	data, err := os.ReadFile(m.activeFilePath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", ErrNoActive
		}
		return "", fmt.Errorf("read active pointer: %w", err)
	}
	name := strings.TrimSpace(string(data))
	if name == "" {
		return "", ErrNoActive
	}
	if err := validateName(name); err != nil {
		// A stale pointer file with a bad name is treated as unset rather
		// than poisoning every CLI call until someone hand-deletes the file.
		return "", ErrNoActive
	}
	return name, nil
}

// SetActive marks name as the active sandbox. The sandbox must already exist;
// ErrNotFound is returned otherwise so we never have a pointer to a missing
// directory.
func (m *Manager) SetActive(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	path := m.pathFor(name)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %s is not a directory", ErrNotFound, name)
	}

	// Ensure the sandboxes/ dir exists before writing the pointer file —
	// callers may invoke SetActive before any Create on a fresh root.
	if err := os.MkdirAll(m.sandboxesPath(), dirPerm); err != nil {
		return fmt.Errorf("create sandboxes dir: %w", err)
	}

	// Write atomically: write a tmp file in the same dir, then rename. This
	// avoids a window where the file exists but is empty (which Active()
	// already tolerates as "unset", but we can do better).
	tmp := m.activeFilePath() + ".tmp"
	if err := os.WriteFile(tmp, []byte(name+"\n"), configFilePerm); err != nil {
		return fmt.Errorf("write active pointer: %w", err)
	}
	if err := os.Rename(tmp, m.activeFilePath()); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename active pointer: %w", err)
	}
	return nil
}

// ClearActive removes the active-pointer file. Calling ClearActive when no
// pointer exists is a no-op (mirrors Destroy's idempotent friendlier paths).
func (m *Manager) ClearActive() error {
	if err := os.Remove(m.activeFilePath()); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("remove active pointer: %w", err)
	}
	return nil
}

// Switch validates the named sandbox exists, opens it, and records it as the
// active selection in one call. Returns the resolved Workspace handle so the
// caller can immediately drive a session against it.
func (m *Manager) Switch(name string) (*Workspace, error) {
	ws, err := m.Open(name)
	if err != nil {
		return nil, err
	}
	if err := m.SetActive(name); err != nil {
		return nil, err
	}
	return ws, nil
}
