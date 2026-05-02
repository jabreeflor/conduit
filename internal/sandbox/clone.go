// clone.go — PRD §15.7  Sandbox cloning
//
// Clone copies workspace/, home/, cache/, and logs/ from a source sandbox to
// a fresh destination sandbox. snapshots/ and tmp/ are intentionally not
// cloned: snapshots are point-in-time records of the source, and tmp/ is by
// definition session-scoped scratch space.
//
// Clone reuses hardlinkTree (snapshot.go) so the on-disk cost is O(file count)
// rather than O(bytes). The two trees stay independent for *future* writes
// because file-system COW (APFS / btrfs) breaks the link on first write, and
// because Conduit always writes via O_TRUNC + create-then-rename rather than
// in-place — which preserves source bytes even on filesystems without COW.

package sandbox

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// CloneOptions controls Clone. A zero CloneOptions inherits the source's
// disk quota and resource limits.
type CloneOptions struct {
	// QuotaBytes overrides the source's disk quota when > 0.
	QuotaBytes int64
	// MemoryBytes overrides the source's memory ceiling when > 0.
	MemoryBytes int64
	// CPULimit overrides the source's CPU allowance when > 0.
	CPULimit float64
}

// cloneSubdirs is the set of subdirs Clone copies. snapshots/ and tmp/ are
// intentionally omitted — see file-level docstring.
var cloneSubdirs = []Subdir{
	SubdirWorkspace,
	SubdirHome,
	SubdirCache,
	SubdirLogs,
}

// Clone creates a new sandbox at dst by hardlink-copying the workspace data
// from src. The destination must not already exist; the source must.
//
// On any partial failure the destination is removed in full so the caller
// never sees a half-built sandbox.
func (m *Manager) Clone(src, dst string, opts CloneOptions) (*Workspace, error) {
	if err := validateName(src); err != nil {
		return nil, fmt.Errorf("source: %w", err)
	}
	if err := validateName(dst); err != nil {
		return nil, fmt.Errorf("destination: %w", err)
	}
	if src == dst {
		return nil, fmt.Errorf("source and destination must differ: both %q", src)
	}

	srcPath := m.pathFor(src)
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, src)
		}
		return nil, fmt.Errorf("stat source %s: %w", srcPath, err)
	}
	if !srcInfo.IsDir() {
		return nil, fmt.Errorf("%w: %s is not a directory", ErrNotFound, src)
	}

	dstPath := m.pathFor(dst)
	if _, err := os.Stat(dstPath); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrAlreadyExists, dst)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("stat destination %s: %w", dstPath, err)
	}

	srcCfg, err := readConfig(srcPath)
	if err != nil {
		return nil, fmt.Errorf("read source config: %w", err)
	}

	// Lock the source so a session-scoped operation (EndSession / Cleanup)
	// can't race the clone mid-walk and leave half-deleted tmp inside the
	// hardlinked snapshot. We grab the lock through a temporary handle.
	srcWS := &Workspace{path: srcPath, name: src, cfg: srcCfg}
	release, err := srcWS.acquireLock()
	if err != nil {
		return nil, err
	}
	defer release()

	if err := os.MkdirAll(dstPath, dirPerm); err != nil {
		return nil, fmt.Errorf("create destination dir: %w", err)
	}

	// Pre-create the layout, including the subdirs we don't clone, so the
	// destination passes the same shape invariants as a freshly Create()d
	// workspace.
	for _, sub := range allSubdirs {
		if err := os.MkdirAll(filepath.Join(dstPath, string(sub)), dirPerm); err != nil {
			_ = os.RemoveAll(dstPath)
			return nil, fmt.Errorf("create %s in destination: %w", sub, err)
		}
	}

	for _, sub := range cloneSubdirs {
		if _, _, err := hardlinkTree(srcWS.Path(sub), filepath.Join(dstPath, string(sub))); err != nil {
			_ = os.RemoveAll(dstPath)
			return nil, fmt.Errorf("clone %s: %w", sub, err)
		}
	}

	now := time.Now().UTC()
	dstCfg := workspaceConfig{
		Name:        dst,
		CreatedAt:   now,
		LastUsedAt:  now,
		QuotaBytes:  pickInt64(opts.QuotaBytes, srcCfg.QuotaBytes, DefaultQuotaBytes),
		MemoryBytes: pickInt64(opts.MemoryBytes, srcCfg.MemoryBytes, DefaultMemoryBytes),
		CPULimit:    pickFloat(opts.CPULimit, srcCfg.CPULimit, DefaultCPULimit),
	}
	if err := writeConfig(dstPath, dstCfg); err != nil {
		_ = os.RemoveAll(dstPath)
		return nil, err
	}

	return &Workspace{path: dstPath, name: dst, cfg: dstCfg}, nil
}

// pickInt64 returns the first positive value among override, inherited, fallback.
func pickInt64(override, inherited, fallback int64) int64 {
	if override > 0 {
		return override
	}
	if inherited > 0 {
		return inherited
	}
	return fallback
}

// pickFloat is the float64 analogue of pickInt64 for CPU shares.
func pickFloat(override, inherited, fallback float64) float64 {
	if override > 0 {
		return override
	}
	if inherited > 0 {
		return inherited
	}
	return fallback
}
