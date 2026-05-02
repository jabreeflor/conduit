package sandbox

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mustCreate is a test helper that creates a workspace or fatals.
func mustCreate(t *testing.T, m *Manager, name string) *Workspace {
	t.Helper()
	w, err := m.Create(name, CreateOptions{})
	if err != nil {
		t.Fatalf("Create(%q): %v", name, err)
	}
	return w
}

// readTestFile reads a file for assertions in snapshot tests.
func readTestFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// ─── tests ───────────────────────────────────────────────────────────────────

func TestSnapshot_BasicCreateAndList(t *testing.T) {
	m := newTestManager(t)
	w := mustCreate(t, m, "test-ws")

	wsFile := filepath.Join(w.Path(SubdirWorkspace), "hello.txt")
	writeFile(t, wsFile, []byte("hello world"))

	info, err := w.Snapshot("initial", 0)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if info.ID == "" {
		t.Error("expected non-empty ID")
	}
	if info.FileCount != 1 {
		t.Errorf("FileCount = %d, want 1", info.FileCount)
	}
	if info.Description != "initial" {
		t.Errorf("Description = %q, want %q", info.Description, "initial")
	}

	snaps, err := w.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("len(snaps) = %d, want 1", len(snaps))
	}
	if snaps[0].ID != info.ID {
		t.Errorf("listed ID %q != created ID %q", snaps[0].ID, info.ID)
	}
}

func TestSnapshot_Rollback(t *testing.T) {
	m := newTestManager(t)
	w := mustCreate(t, m, "rollback-ws")

	wsFile := filepath.Join(w.Path(SubdirWorkspace), "data.txt")
	writeFile(t, wsFile, []byte("original"))

	snap, err := w.Snapshot("before-change", 0)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	writeFile(t, wsFile, []byte("modified"))
	if got := readTestFile(t, wsFile); got != "modified" {
		t.Fatalf("pre-rollback: want %q, got %q", "modified", got)
	}

	if err := w.Rollback(snap.ID); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if got := readTestFile(t, wsFile); got != "original" {
		t.Errorf("post-rollback: want %q, got %q", "original", got)
	}
}

func TestSnapshot_RollbackNotFound(t *testing.T) {
	m := newTestManager(t)
	w := mustCreate(t, m, "notfound-ws")

	if err := w.Rollback("nonexistent-id"); err == nil {
		t.Fatal("expected error for missing snapshot ID, got nil")
	}
}

func TestSnapshot_Delete(t *testing.T) {
	m := newTestManager(t)
	w := mustCreate(t, m, "delete-ws")

	writeFile(t, filepath.Join(w.Path(SubdirWorkspace), "f.txt"), []byte("x"))
	snap, err := w.Snapshot("del-me", 0)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	if err := w.DeleteSnapshot(snap.ID); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}
	snaps, _ := w.ListSnapshots()
	if len(snaps) != 0 {
		t.Errorf("expected 0 snapshots after delete, got %d", len(snaps))
	}
}

func TestSnapshot_DeleteNotFound(t *testing.T) {
	m := newTestManager(t)
	w := mustCreate(t, m, "del-nf-ws")

	if err := w.DeleteSnapshot("ghost"); err == nil {
		t.Fatal("expected error for missing snapshot, got nil")
	}
}

func TestSnapshot_AutoPruneOnCreate(t *testing.T) {
	m := newTestManager(t)
	w := mustCreate(t, m, "prune-ws")

	writeFile(t, filepath.Join(w.Path(SubdirWorkspace), "f.txt"), []byte("v"))

	const cap = 3
	for i := 0; i < cap+2; i++ {
		if _, err := w.Snapshot("snap", cap); err != nil {
			t.Fatalf("Snapshot %d: %v", i, err)
		}
	}

	snaps, err := w.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) > cap {
		t.Errorf("expected ≤ %d snapshots after auto-prune, got %d", cap, len(snaps))
	}
}

func TestSnapshot_PruneByCount(t *testing.T) {
	m := newTestManager(t)
	w := mustCreate(t, m, "countprune-ws")

	writeFile(t, filepath.Join(w.Path(SubdirWorkspace), "g.txt"), []byte("data"))

	for i := 0; i < 5; i++ {
		if _, err := w.Snapshot("s", DefaultMaxSnapshots); err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
	}

	deleted, err := w.PruneSnapshots(2, 0)
	if err != nil {
		t.Fatalf("PruneSnapshots: %v", err)
	}
	if deleted != 3 {
		t.Errorf("deleted = %d, want 3", deleted)
	}
	snaps, _ := w.ListSnapshots()
	if len(snaps) != 2 {
		t.Errorf("remaining = %d, want 2", len(snaps))
	}
}

func TestSnapshot_PruneByAge(t *testing.T) {
	m := newTestManager(t)
	w := mustCreate(t, m, "ageprune-ws")

	writeFile(t, filepath.Join(w.Path(SubdirWorkspace), "h.txt"), []byte("data"))

	snap, err := w.Snapshot("old", DefaultMaxSnapshots)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// Back-date the snapshot meta to simulate an old snapshot.
	bundleDir := filepath.Join(w.Path(SubdirSnapshots), snap.ID)
	meta, err := readSnapshotMeta(bundleDir)
	if err != nil {
		t.Fatalf("readSnapshotMeta: %v", err)
	}
	meta.CreatedAt = time.Now().UTC().Add(-10 * 24 * time.Hour)
	if err := writeSnapshotMeta(bundleDir, meta); err != nil {
		t.Fatalf("writeSnapshotMeta: %v", err)
	}

	if _, err := w.Snapshot("new", DefaultMaxSnapshots); err != nil {
		t.Fatalf("second Snapshot: %v", err)
	}

	deleted, err := w.PruneSnapshots(DefaultMaxSnapshots, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("PruneSnapshots: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}
	snaps, _ := w.ListSnapshots()
	if len(snaps) != 1 {
		t.Errorf("remaining = %d, want 1", len(snaps))
	}
}

func TestSnapshot_MultipleFilesAndDirs(t *testing.T) {
	m := newTestManager(t)
	w := mustCreate(t, m, "tree-ws")

	ws := w.Path(SubdirWorkspace)
	writeFile(t, filepath.Join(ws, "a.txt"), []byte("alpha"))
	writeFile(t, filepath.Join(ws, "sub", "b.txt"), []byte("beta"))
	writeFile(t, filepath.Join(ws, "sub", "deep", "c.txt"), []byte("gamma"))

	snap, err := w.Snapshot("tree", 0)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snap.FileCount != 3 {
		t.Errorf("FileCount = %d, want 3", snap.FileCount)
	}

	os.RemoveAll(filepath.Join(ws, "sub"))
	writeFile(t, filepath.Join(ws, "new.txt"), []byte("new"))

	if err := w.Rollback(snap.ID); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if got := readTestFile(t, filepath.Join(ws, "a.txt")); got != "alpha" {
		t.Errorf("a.txt = %q, want %q", got, "alpha")
	}
	if got := readTestFile(t, filepath.Join(ws, "sub", "b.txt")); got != "beta" {
		t.Errorf("sub/b.txt = %q, want %q", got, "beta")
	}
	if got := readTestFile(t, filepath.Join(ws, "sub", "deep", "c.txt")); got != "gamma" {
		t.Errorf("sub/deep/c.txt = %q, want %q", got, "gamma")
	}
}

func TestSnapshot_IDFormat(t *testing.T) {
	id := newSnapshotID()
	// Expected: "20060102T150405Z-xxxxxxxx"
	if len(id) < 17 {
		t.Errorf("ID too short: %q", id)
	}
	if id[16] != '-' {
		t.Errorf("expected dash at position 16 in ID %q, got %c", id, id[16])
	}
}

func TestSnapshot_ListNewestFirst(t *testing.T) {
	m := newTestManager(t)
	w := mustCreate(t, m, "order-ws")

	writeFile(t, filepath.Join(w.Path(SubdirWorkspace), "x.txt"), []byte("x"))

	for i := 0; i < 3; i++ {
		if _, err := w.Snapshot("s", 0); err != nil {
			t.Fatalf("Snapshot %d: %v", i, err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	snaps, err := w.ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) != 3 {
		t.Fatalf("len = %d, want 3", len(snaps))
	}
	for i := 1; i < len(snaps); i++ {
		if snaps[i-1].CreatedAt.Before(snaps[i].CreatedAt) {
			t.Errorf("snapshots not in newest-first order at index %d", i)
		}
	}
}

func TestSnapshot_EmptyWorkspaceRollback(t *testing.T) {
	m := newTestManager(t)
	w := mustCreate(t, m, "empty-ws")

	snap, err := w.Snapshot("empty", 0)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snap.FileCount != 0 {
		t.Errorf("FileCount = %d, want 0", snap.FileCount)
	}

	writeFile(t, filepath.Join(w.Path(SubdirWorkspace), "tmp.txt"), []byte("temp"))

	if err := w.Rollback(snap.ID); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	entries, _ := os.ReadDir(w.Path(SubdirWorkspace))
	if len(entries) != 0 {
		t.Errorf("workspace should be empty after rollback to empty snapshot, got %d entries", len(entries))
	}
}
