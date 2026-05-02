package scheduler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DefaultStorePath is the JSON file Conduit uses to persist schedules across
// restarts. Callers can override via NewStore for tests.
const DefaultStorePath = "~/.conduit/schedules.json"

// Store reads and writes the persisted schedule list. It is safe for use by
// a single Scheduler; concurrent access from multiple processes is not
// supported.
type Store struct {
	path string
}

// NewStore constructs a store rooted at path. A leading "~" is expanded to
// the user's home directory. The parent directory is created lazily on Save.
func NewStore(path string) (*Store, error) {
	if path == "" {
		path = DefaultStorePath
	}
	expanded, err := expandPath(path)
	if err != nil {
		return nil, err
	}
	return &Store{path: expanded}, nil
}

// Path returns the resolved on-disk location.
func (s *Store) Path() string { return s.path }

// Load reads the schedule file. A missing file yields an empty slice.
func (s *Store) Load() ([]Schedule, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("scheduler/store: read %s: %w", s.path, err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	var out []Schedule
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("scheduler/store: parse %s: %w", s.path, err)
	}
	return out, nil
}

// Save writes the schedules atomically: a sibling temp file is created in the
// target directory, fsynced, then renamed over the destination.
func (s *Store) Save(entries []Schedule) error {
	if entries == nil {
		entries = []Schedule{}
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("scheduler/store: mkdir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("scheduler/store: marshal: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".schedules-*.json")
	if err != nil {
		return fmt.Errorf("scheduler/store: temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("scheduler/store: write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("scheduler/store: sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("scheduler/store: close temp: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("scheduler/store: chmod temp: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("scheduler/store: rename: %w", err)
	}
	cleanup = false
	return nil
}

// expandPath resolves a leading "~" against the current user's home directory.
func expandPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("scheduler/store: empty path")
	}
	if path[0] != '~' {
		return filepath.Clean(path), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("scheduler/store: resolve home: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	if path[1] != '/' && path[1] != filepath.Separator {
		return "", fmt.Errorf("scheduler/store: cannot expand %q", path)
	}
	return filepath.Join(home, path[2:]), nil
}
