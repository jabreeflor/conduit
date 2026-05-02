package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCloneCopiesWorkspaceHomeCacheAndLogs(t *testing.T) {
	m := newTestManager(t)
	src, err := m.Create("src", CreateOptions{
		QuotaBytes:  3 << 30,
		MemoryBytes: 2 << 30,
		CPULimit:    1.0,
	})
	if err != nil {
		t.Fatalf("Create src: %v", err)
	}

	// Seed source files in workspace/, home/, cache/, logs/.
	writeFile(t, filepath.Join(src.Path(SubdirWorkspace), "code.go"), []byte("package main\n"))
	writeFile(t, filepath.Join(src.Path(SubdirWorkspace), "nested", "x"), []byte("nested"))
	writeFile(t, filepath.Join(src.Path(SubdirHome), ".bashrc"), []byte("export X=1\n"))
	writeFile(t, filepath.Join(src.Path(SubdirCache), "blob"), []byte("cached"))
	writeFile(t, filepath.Join(src.Path(SubdirLogs), "session.log"), []byte("log line\n"))
	// snapshots/ and tmp/ should be skipped during clone.
	writeFile(t, filepath.Join(src.Path(SubdirSnapshots), "leaked.txt"), []byte("snap"))
	writeFile(t, filepath.Join(src.Path(SubdirTmp), "scratch"), []byte("tmp"))

	dst, err := m.Clone("src", "dst", CloneOptions{})
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if dst.Name() != "dst" {
		t.Fatalf("dst.Name = %q", dst.Name())
	}

	for _, rel := range []struct{ sub, name string }{
		{string(SubdirWorkspace), "code.go"},
		{string(SubdirWorkspace), filepath.Join("nested", "x")},
		{string(SubdirHome), ".bashrc"},
		{string(SubdirCache), "blob"},
		{string(SubdirLogs), "session.log"},
	} {
		path := filepath.Join(dst.Root(), rel.sub, rel.name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %s to exist in clone: %v", path, err)
		}
	}

	for _, sub := range []Subdir{SubdirSnapshots, SubdirTmp} {
		entries, err := os.ReadDir(dst.Path(sub))
		if err != nil {
			t.Fatalf("read %s: %v", sub, err)
		}
		if len(entries) != 0 {
			t.Errorf("%s should be empty in clone, got %d entries", sub, len(entries))
		}
	}

	// Limits inherited from source.
	if dst.Quota() != 3<<30 {
		t.Errorf("clone Quota = %d, want %d", dst.Quota(), 3<<30)
	}
	if dst.MemoryLimit() != 2<<30 {
		t.Errorf("clone MemoryLimit = %d, want %d", dst.MemoryLimit(), 2<<30)
	}
	if dst.CPULimit() != 1.0 {
		t.Errorf("clone CPULimit = %f, want 1.0", dst.CPULimit())
	}
}

func TestCloneOverridesLimits(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Create("base", CreateOptions{
		QuotaBytes:  3 << 30,
		MemoryBytes: 2 << 30,
		CPULimit:    1.0,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	dst, err := m.Clone("base", "narrow", CloneOptions{
		QuotaBytes:  1 << 30,
		MemoryBytes: 512 << 20,
		CPULimit:    0.25,
	})
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if dst.Quota() != 1<<30 || dst.MemoryLimit() != 512<<20 || dst.CPULimit() != 0.25 {
		t.Errorf("override mismatch: q=%d m=%d c=%f", dst.Quota(), dst.MemoryLimit(), dst.CPULimit())
	}
}

func TestCloneRejectsBadAndConflictingNames(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Create("orig", CreateOptions{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := m.Clone("Bad", "copy", CloneOptions{}); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("clone bad src err=%v, want ErrInvalidName", err)
	}
	if _, err := m.Clone("orig", "Bad Copy", CloneOptions{}); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("clone bad dst err=%v, want ErrInvalidName", err)
	}
	if _, err := m.Clone("orig", "orig", CloneOptions{}); err == nil {
		t.Fatalf("clone src==dst should error")
	}
}

func TestCloneSourceMissing(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Clone("missing", "copy", CloneOptions{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("clone missing src err=%v, want ErrNotFound", err)
	}
}

func TestCloneDestExists(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Create("a", CreateOptions{}); err != nil {
		t.Fatalf("Create a: %v", err)
	}
	if _, err := m.Create("b", CreateOptions{}); err != nil {
		t.Fatalf("Create b: %v", err)
	}
	if _, err := m.Clone("a", "b", CloneOptions{}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("clone existing dst err=%v, want ErrAlreadyExists", err)
	}
}

func TestCloneIndependentOfSourceWrites(t *testing.T) {
	// After cloning, modifications to the source tree must not surface in
	// the destination — even though hardlinks share inodes — because
	// Conduit always writes via create-then-rename, which breaks the link.
	m := newTestManager(t)
	src, err := m.Create("orig", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	target := filepath.Join(src.Path(SubdirWorkspace), "data.txt")
	writeFile(t, target, []byte("v1"))

	dst, err := m.Clone("orig", "copy", CloneOptions{})
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	staging := target + ".new"
	if err := os.WriteFile(staging, []byte("v2"), 0o644); err != nil {
		t.Fatalf("write staging: %v", err)
	}
	if err := os.Rename(staging, target); err != nil {
		t.Fatalf("rename: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dst.Path(SubdirWorkspace), "data.txt"))
	if err != nil {
		t.Fatalf("read clone payload: %v", err)
	}
	if string(got) != "v1" {
		t.Fatalf("clone payload = %q, want v1 (source mutation leaked through hardlink)", got)
	}
}

func TestCloneCreatesAllLayoutSubdirs(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Create("orig", CreateOptions{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	dst, err := m.Clone("orig", "copy", CloneOptions{})
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	for _, sub := range []Subdir{SubdirWorkspace, SubdirHome, SubdirCache, SubdirSnapshots, SubdirLogs, SubdirTmp} {
		info, err := os.Stat(dst.Path(sub))
		if err != nil {
			t.Fatalf("stat %s: %v", sub, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", sub)
		}
	}
	if _, err := os.Stat(filepath.Join(dst.Root(), "config.yaml")); err != nil {
		t.Fatalf("config.yaml missing in clone: %v", err)
	}
}

func TestCloneDoesNotMutateSourceConfig(t *testing.T) {
	m := newTestManager(t)
	src, err := m.Create("orig", CreateOptions{QuotaBytes: 1 << 20})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	beforeQuota := src.Quota()
	beforeCreated := src.cfg.CreatedAt

	if _, err := m.Clone("orig", "copy", CloneOptions{QuotaBytes: 4 << 20}); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	reopened, err := m.Open("orig")
	if err != nil {
		t.Fatalf("Open after Clone: %v", err)
	}
	if reopened.Quota() != beforeQuota {
		t.Fatalf("source quota mutated: got %d, want %d", reopened.Quota(), beforeQuota)
	}
	if !reopened.cfg.CreatedAt.Equal(beforeCreated) {
		t.Fatalf("source created_at mutated")
	}
}

func TestCloneFailureLeavesNoPartialDestination(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Create("orig", CreateOptions{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := os.MkdirAll(m.sandboxesPath(), 0o755); err != nil {
		t.Fatalf("mkdir sandboxes: %v", err)
	}
	conflict := filepath.Join(m.sandboxesPath(), "copy")
	if err := os.WriteFile(conflict, []byte("not-a-dir"), 0o644); err != nil {
		t.Fatalf("plant conflict file: %v", err)
	}
	if _, err := m.Clone("orig", "copy", CloneOptions{}); err == nil {
		t.Fatalf("Clone should have failed when dst exists as a file")
	}
	if info, statErr := os.Stat(conflict); statErr != nil {
		t.Fatalf("conflict file missing after failed clone: %v", statErr)
	} else if info.IsDir() {
		t.Fatalf("conflict file was replaced by a directory")
	}
	// Traversing through a regular file gives ENOTDIR, not ENOENT, so we
	// can't rely on fs.ErrNotExist here. Asserting that the conflict file's
	// content is still untouched is sufficient to prove no clone bytes
	// leaked into the path.
	got, readErr := os.ReadFile(conflict)
	if readErr != nil {
		t.Fatalf("read conflict file after failed clone: %v", readErr)
	}
	if string(got) != "not-a-dir" {
		t.Fatalf("conflict file content mutated: got %q, want %q", got, "not-a-dir")
	}
}
