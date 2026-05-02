// snapshot.go — PRD §15.6  Snapshot & Rollback
//
// Each snapshot is stored under snapshots/<id>/ inside the workspace root and
// captures the full filesystem tree of workspace/ and home/ using hardlinks.
//
// Hardlinks are O(file-count), not O(data-size): a 5 GB workspace snapshot
// costs only a few hundred kilobytes of new directory metadata because the
// data blocks are shared with the live tree until either side writes (copy-on-
// write at the filesystem layer on APFS/btrfs; shared until removed on ext4).
//
// Rollback copies snapshot files back with fresh inodes, breaking the shared-
// block relationship so future writes don't affect the snapshot.
//
// Layout:
//
//	snapshots/
//	  <id>/
//	    meta.yaml        – ID, description, created_at, file_count, size_bytes
//	    workspace/       – hardlinked clone of workspace/ subdir
//	    home/            – hardlinked clone of home/ subdir
package sandbox

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

// Defaults for snapshot lifecycle limits (PRD §15.6).
const (
	DefaultMaxSnapshots = 20
	DefaultMaxAge       = 7 * 24 * time.Hour
)

const snapshotMetaFile = "meta.yaml"

// Sentinel errors for snapshot operations.
var (
	// ErrSnapshotNotFound is returned when a snapshot ID is not present.
	ErrSnapshotNotFound = errors.New("snapshot not found")
	// ErrSnapshotLimitReached is returned when Snapshot is called but the
	// caller has already reached the configured cap and pruning is disabled.
	ErrSnapshotLimitReached = errors.New("snapshot limit reached")
)

// SnapshotInfo describes one snapshot bundle. All fields are read-only copies
// from meta.yaml and are safe to cache across calls.
type SnapshotInfo struct {
	// ID uniquely identifies the snapshot (timestamp + 4-byte random hex).
	ID string `json:"id"`
	// Description is the optional human label set at creation time.
	Description string `json:"description,omitempty"`
	// CreatedAt is the UTC instant the snapshot was committed.
	CreatedAt time.Time `json:"created_at"`
	// SizeBytes is the on-disk footprint of the snapshot bundle.
	// Because files are hardlinked the real additional cost is much smaller.
	SizeBytes int64 `json:"size_bytes"`
	// FileCount is the number of regular files captured.
	FileCount int `json:"file_count"`
}

// snapshotMeta is the on-disk YAML schema for each snapshot's meta.yaml.
type snapshotMeta struct {
	ID          string    `yaml:"id"`
	Description string    `yaml:"description,omitempty"`
	CreatedAt   time.Time `yaml:"created_at"`
	SizeBytes   int64     `yaml:"size_bytes"`
	FileCount   int       `yaml:"file_count"`
}

// Snapshot captures the current state of workspace/ and home/ as a new
// snapshot bundle. description may be empty. The snapshot ID is returned
// inside SnapshotInfo.
//
// Auto-prune: if the number of snapshots would exceed maxSnapshots after
// creation, the oldest snapshots are deleted first. Pass maxSnapshots ≤ 0
// to use DefaultMaxSnapshots.
func (w *Workspace) Snapshot(description string, maxSnapshots int) (SnapshotInfo, error) {
	if maxSnapshots <= 0 {
		maxSnapshots = DefaultMaxSnapshots
	}

	id := newSnapshotID()
	bundleDir := filepath.Join(w.Path(SubdirSnapshots), id)

	if err := os.MkdirAll(bundleDir, dirPerm); err != nil {
		return SnapshotInfo{}, fmt.Errorf("create snapshot dir: %w", err)
	}

	var totalBytes int64
	var totalFiles int

	// Capture workspace/ and home/ via hardlinks.
	for _, sub := range []Subdir{SubdirWorkspace, SubdirHome} {
		src := w.Path(sub)
		dst := filepath.Join(bundleDir, string(sub))
		b, n, err := copyTreeCounted(src, dst)
		if err != nil {
			// Roll back the partially-created bundle.
			_ = os.RemoveAll(bundleDir)
			return SnapshotInfo{}, fmt.Errorf("snapshot %s: %w", sub, err)
		}
		totalBytes += b
		totalFiles += n
	}

	now := time.Now().UTC()
	meta := snapshotMeta{
		ID:          id,
		Description: description,
		CreatedAt:   now,
		SizeBytes:   totalBytes,
		FileCount:   totalFiles,
	}
	if err := writeSnapshotMeta(bundleDir, meta); err != nil {
		_ = os.RemoveAll(bundleDir)
		return SnapshotInfo{}, err
	}

	info := SnapshotInfo{
		ID:          id,
		Description: description,
		CreatedAt:   now,
		SizeBytes:   totalBytes,
		FileCount:   totalFiles,
	}

	// Auto-prune if we've exceeded the cap.
	if err := w.pruneToLimit(maxSnapshots); err != nil {
		// Non-fatal: the snapshot was created successfully.
		_ = err
	}

	return info, nil
}

// ListSnapshots returns all snapshots, newest first.
func (w *Workspace) ListSnapshots() ([]SnapshotInfo, error) {
	entries, err := os.ReadDir(w.Path(SubdirSnapshots))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read snapshots dir: %w", err)
	}

	out := make([]SnapshotInfo, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		bundleDir := filepath.Join(w.Path(SubdirSnapshots), entry.Name())
		meta, err := readSnapshotMeta(bundleDir)
		if err != nil {
			// Skip bundles with unreadable metadata.
			continue
		}
		out = append(out, SnapshotInfo{
			ID:          meta.ID,
			Description: meta.Description,
			CreatedAt:   meta.CreatedAt,
			SizeBytes:   meta.SizeBytes,
			FileCount:   meta.FileCount,
		})
	}

	// Newest first.
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// Rollback restores workspace/ and home/ from the snapshot identified by id.
// The restoration is done via a fresh file copy (not hardlinks) so the live
// tree and the snapshot remain independent after rollback.
func (w *Workspace) Rollback(id string) error {
	bundleDir := filepath.Join(w.Path(SubdirSnapshots), id)
	if _, err := os.Stat(bundleDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrSnapshotNotFound, id)
		}
		return fmt.Errorf("stat snapshot: %w", err)
	}

	// Verify meta exists (guards against partial bundles).
	if _, err := readSnapshotMeta(bundleDir); err != nil {
		return fmt.Errorf("snapshot %s is incomplete or corrupt: %w", id, err)
	}

	for _, sub := range []Subdir{SubdirWorkspace, SubdirHome} {
		src := filepath.Join(bundleDir, string(sub))
		dst := w.Path(sub)

		// Atomically replace dst: move current to a staging path, restore
		// from snapshot, then remove staging. If restore fails we reinstate.
		staging := dst + ".rollback-staging"
		if err := os.Rename(dst, staging); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("stage %s for rollback: %w", sub, err)
		}

		if err := copyTree(src, dst); err != nil {
			// Attempt to reinstate the original.
			_ = os.RemoveAll(dst)
			if renErr := os.Rename(staging, dst); renErr != nil {
				return fmt.Errorf("rollback %s failed and reinstate failed: original=%v copy=%v", sub, renErr, err)
			}
			return fmt.Errorf("restore %s: %w", sub, err)
		}

		// Remove the staging copy now that restore succeeded.
		_ = os.RemoveAll(staging)
	}
	return nil
}

// DeleteSnapshot removes the snapshot bundle identified by id.
func (w *Workspace) DeleteSnapshot(id string) error {
	bundleDir := filepath.Join(w.Path(SubdirSnapshots), id)
	if _, err := os.Stat(bundleDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrSnapshotNotFound, id)
		}
		return fmt.Errorf("stat snapshot: %w", err)
	}
	if err := os.RemoveAll(bundleDir); err != nil {
		return fmt.Errorf("delete snapshot %s: %w", id, err)
	}
	return nil
}

// PruneSnapshots removes snapshots older than maxAge and keeps only the most
// recent maxCount snapshots. Pass maxCount ≤ 0 to use DefaultMaxSnapshots;
// pass maxAge ≤ 0 to skip age-based pruning. Returns the number of deleted
// snapshots.
func (w *Workspace) PruneSnapshots(maxCount int, maxAge time.Duration) (int, error) {
	if maxCount <= 0 {
		maxCount = DefaultMaxSnapshots
	}
	snaps, err := w.ListSnapshots()
	if err != nil {
		return 0, err
	}

	var deleted int
	threshold := time.Now().UTC().Add(-maxAge)

	// Age-based pruning first.
	if maxAge > 0 {
		for _, s := range snaps {
			if s.CreatedAt.Before(threshold) {
				if delErr := w.DeleteSnapshot(s.ID); delErr == nil {
					deleted++
				}
			}
		}
		// Refresh list after age pruning.
		snaps, err = w.ListSnapshots()
		if err != nil {
			return deleted, err
		}
	}

	// Count-based pruning: keep the newest maxCount, remove the rest.
	if len(snaps) > maxCount {
		// snaps is already newest-first; remove from the tail.
		for _, s := range snaps[maxCount:] {
			if delErr := w.DeleteSnapshot(s.ID); delErr == nil {
				deleted++
			}
		}
	}
	return deleted, nil
}

// pruneToLimit is the internal helper called after Snapshot to enforce the cap.
func (w *Workspace) pruneToLimit(max int) error {
	snaps, err := w.ListSnapshots()
	if err != nil {
		return err
	}
	if len(snaps) <= max {
		return nil
	}
	// snaps is newest-first; prune from the end.
	for _, s := range snaps[max:] {
		_ = w.DeleteSnapshot(s.ID)
	}
	return nil
}

// ─── helpers ────────────────────────────────────────────────────────────────

// newSnapshotID returns a unique, lexicographically time-sortable ID:
// "20060102T150405Z-<8 hex chars>".
func newSnapshotID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fallback to timestamp-only when crypto/rand is unavailable.
		return time.Now().UTC().Format("20060102T150405Z") + "-00000000"
	}
	return time.Now().UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(b[:])
}

// hardlinkTree walks src and recreates the directory tree at dst using
// hard-links for regular files. Symlinks are reproduced as symlinks.
// Returns (bytesLinked, fileCount, error).
//
// When os.Link fails (cross-device, unsupported FS) the file is copied
// instead so the snapshot always succeeds.
func hardlinkTree(src, dst string) (int64, int, error) {
	// If src doesn't exist, create an empty dst and return.
	if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) {
		return 0, 0, os.MkdirAll(dst, dirPerm)
	}

	var totalBytes int64
	var fileCount int

	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		switch {
		case d.IsDir():
			return os.MkdirAll(target, dirPerm)

		case d.Type()&fs.ModeSymlink != 0:
			linkDst, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(linkDst, target)

		default:
			// Regular file: try hardlink first, fall back to copy.
			if err := os.Link(path, target); err != nil {
				// Cross-device or unsupported: full copy.
				n, copyErr := copyFile(path, target)
				if copyErr != nil {
					return fmt.Errorf("copy %s: %w", rel, copyErr)
				}
				totalBytes += n
			} else {
				info, _ := d.Info()
				if info != nil {
					totalBytes += info.Size()
				}
			}
			fileCount++
			return nil
		}
	})
	return totalBytes, fileCount, err
}

// copyTree walks src and recreates dst with independent file copies (no
// hardlinks). Used by Rollback so the restored files are fully independent
// from the snapshot.
func copyTree(src, dst string) error {
	if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) {
		return os.MkdirAll(dst, dirPerm)
	}

	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		switch {
		case d.IsDir():
			return os.MkdirAll(target, dirPerm)

		case d.Type()&fs.ModeSymlink != 0:
			linkDst, err := os.Readlink(path)
			if err != nil {
				return err
			}
			// Remove any existing symlink at target before recreation.
			_ = os.Remove(target)
			return os.Symlink(linkDst, target)

		default:
			if _, err := copyFile(path, target); err != nil {
				return fmt.Errorf("copy %s: %w", rel, err)
			}
			return nil
		}
	})
}

// copyFile copies src to dst, creating or truncating dst. Returns bytes written.
func copyFile(src, dst string) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()

	// Preserve source permissions.
	info, err := in.Stat()
	if err != nil {
		return 0, err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return 0, err
	}
	defer func() {
		if closeErr := out.Close(); err == nil {
			err = closeErr
		}
	}()

	n, err := io.Copy(out, in)
	return n, err
}

// writeSnapshotMeta persists snapshotMeta to <bundleDir>/meta.yaml.
func writeSnapshotMeta(bundleDir string, meta snapshotMeta) error {
	data, err := yaml.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal snapshot meta: %w", err)
	}
	target := filepath.Join(bundleDir, snapshotMetaFile)
	if err := os.WriteFile(target, data, configFilePerm); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	return nil
}

// readSnapshotMeta parses <bundleDir>/meta.yaml.
func readSnapshotMeta(bundleDir string) (snapshotMeta, error) {
	target := filepath.Join(bundleDir, snapshotMetaFile)
	data, err := os.ReadFile(target)
	if err != nil {
		return snapshotMeta{}, fmt.Errorf("read %s: %w", target, err)
	}
	var meta snapshotMeta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return snapshotMeta{}, fmt.Errorf("parse %s: %w", target, err)
	}
	return meta, nil
}

// copyTreeCounted walks src and copies all files to dst, returning
// (bytesWritten, fileCount, error). Unlike hardlinkTree, the resulting files
// are independent inodes so in-place writes to the live tree do not affect
// the snapshot.
func copyTreeCounted(src, dst string) (int64, int, error) {
	if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) {
		return 0, 0, os.MkdirAll(dst, dirPerm)
	}

	var totalBytes int64
	var fileCount int

	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dst, rel)

		switch {
		case d.IsDir():
			return os.MkdirAll(target, dirPerm)
		case d.Type()&fs.ModeSymlink != 0:
			linkDst, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(linkDst, target)
		default:
			n, err := copyFile(path, target)
			if err != nil {
				return fmt.Errorf("copy %s: %w", rel, err)
			}
			totalBytes += n
			fileCount++
			return nil
		}
	})
	return totalBytes, fileCount, err
}
