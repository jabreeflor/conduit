package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrCheckpointNotFound is returned by Checkpointer.Load when no checkpoint
// exists for the given run ID.
var ErrCheckpointNotFound = errors.New("workflow: checkpoint not found")

// Checkpointer persists and reloads Run state.
//
// Implementations must make Save atomic with respect to concurrent readers:
// after Save returns, a subsequent Load on any goroutine must observe either
// the previous Run or the freshly written Run, never a partial write.
type Checkpointer interface {
	// Save writes run to durable storage keyed by run.ID.
	Save(run *Run) error
	// Load reads the Run previously written under runID. If no such Run
	// exists, Load returns ErrCheckpointNotFound.
	Load(runID string) (*Run, error)
}

// FileCheckpointer is a Checkpointer that stores Runs as JSON files in a
// directory. Each Run is a single file named "<run-id>.json".
//
// Save uses the standard tmpfile+rename pattern so a crash during write
// cannot leave a torn file in place.
type FileCheckpointer struct {
	dir string
}

// NewFileCheckpointer returns a FileCheckpointer rooted at dir. The directory
// is created if it does not exist.
func NewFileCheckpointer(dir string) (*FileCheckpointer, error) {
	if dir == "" {
		return nil, errors.New("workflow: checkpoint directory must not be empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("workflow: create checkpoint dir: %w", err)
	}
	return &FileCheckpointer{dir: dir}, nil
}

// DefaultCheckpointDir returns the standard checkpoint directory under the
// user's home: ~/.conduit/runs.
func DefaultCheckpointDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("workflow: resolve home: %w", err)
	}
	return filepath.Join(home, ".conduit", "runs"), nil
}

// Dir returns the directory backing this FileCheckpointer.
func (c *FileCheckpointer) Dir() string { return c.dir }

// Save writes run to <dir>/<run.ID>.json atomically.
func (c *FileCheckpointer) Save(run *Run) error {
	if run == nil {
		return errors.New("workflow: cannot save nil Run")
	}
	if run.ID == "" {
		return errors.New("workflow: Run.ID must be set before Save")
	}

	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return fmt.Errorf("workflow: marshal run: %w", err)
	}

	finalPath := c.pathFor(run.ID)
	tmp, err := os.CreateTemp(c.dir, run.ID+".*.tmp")
	if err != nil {
		return fmt.Errorf("workflow: create temp checkpoint: %w", err)
	}
	tmpPath := tmp.Name()

	// Best-effort cleanup if anything below fails before rename.
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("workflow: write temp checkpoint: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("workflow: sync temp checkpoint: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("workflow: close temp checkpoint: %w", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		cleanup()
		return fmt.Errorf("workflow: rename checkpoint into place: %w", err)
	}
	return nil
}

// Load reads the Run previously written under runID.
func (c *FileCheckpointer) Load(runID string) (*Run, error) {
	if runID == "" {
		return nil, errors.New("workflow: runID must not be empty")
	}
	data, err := os.ReadFile(c.pathFor(runID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrCheckpointNotFound
		}
		return nil, fmt.Errorf("workflow: read checkpoint: %w", err)
	}
	var run Run
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, fmt.Errorf("workflow: decode checkpoint: %w", err)
	}
	return &run, nil
}

func (c *FileCheckpointer) pathFor(runID string) string {
	return filepath.Join(c.dir, runID+".json")
}
