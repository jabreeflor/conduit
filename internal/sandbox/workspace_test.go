package sandbox

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestManager returns a Manager rooted at a freshly-allocated tempdir so
// every test gets its own isolated <root>/sandboxes tree.
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	return NewManager(t.TempDir())
}

// writeFile writes b at path, creating parent dirs as needed.
func writeFile(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestNewManagerDefaultsToHome(t *testing.T) {
	m := NewManager("")
	if m.Root() == "" {
		t.Fatal("default Root() is empty")
	}
	if !filepath.IsAbs(m.Root()) {
		t.Fatalf("default Root() not absolute: %s", m.Root())
	}
}

func TestCreateOpenListDestroyRoundTrip(t *testing.T) {
	m := newTestManager(t)

	ws, err := m.Create("alpha", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ws.Name() != "alpha" {
		t.Fatalf("Name = %q, want alpha", ws.Name())
	}
	if ws.Quota() != DefaultQuotaBytes {
		t.Fatalf("Quota = %d, want default %d", ws.Quota(), DefaultQuotaBytes)
	}

	// Layout dirs exist with 0o755.
	for _, sub := range []Subdir{SubdirWorkspace, SubdirHome, SubdirCache, SubdirSnapshots, SubdirLogs, SubdirTmp} {
		info, err := os.Stat(ws.Path(sub))
		if err != nil {
			t.Fatalf("stat %s: %v", sub, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", sub)
		}
		if runtime.GOOS != "windows" {
			if perm := info.Mode().Perm(); perm != 0o755 {
				t.Fatalf("perm of %s = %o, want 0755", sub, perm)
			}
		}
	}

	// config.yaml present with 0o644.
	cfgPath := filepath.Join(ws.Root(), "config.yaml")
	cfgInfo, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("stat config.yaml: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := cfgInfo.Mode().Perm(); perm != 0o644 {
			t.Fatalf("config.yaml perm = %o, want 0644", perm)
		}
	}

	// Open round-trips name + quota.
	got, err := m.Open("alpha")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got.Name() != "alpha" || got.Quota() != DefaultQuotaBytes {
		t.Fatalf("Open returned name=%q quota=%d", got.Name(), got.Quota())
	}

	// Create a second workspace so List has something to sort.
	if _, err := m.Create("beta", CreateOptions{QuotaBytes: 1024}); err != nil {
		t.Fatalf("Create beta: %v", err)
	}

	infos, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("List returned %d items, want 2", len(infos))
	}
	if infos[0].Name != "alpha" || infos[1].Name != "beta" {
		t.Fatalf("List order = [%s,%s], want [alpha,beta]", infos[0].Name, infos[1].Name)
	}
	if infos[1].QuotaBytes != 1024 {
		t.Fatalf("beta quota = %d, want 1024", infos[1].QuotaBytes)
	}
	if infos[0].CreatedAt.IsZero() {
		t.Fatal("alpha CreatedAt is zero")
	}

	// Destroy alpha; beta survives.
	if err := m.Destroy("alpha"); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if _, err := os.Stat(ws.Root()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("alpha root still present: stat err=%v", err)
	}
	if _, err := m.Open("alpha"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open alpha after destroy err=%v, want ErrNotFound", err)
	}
	if _, err := m.Open("beta"); err != nil {
		t.Fatalf("Open beta survived destroy err=%v", err)
	}
}

func TestCreateRejectsInvalidNames(t *testing.T) {
	m := newTestManager(t)
	cases := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"empty", "", ErrInvalidName},
		{"uppercase", "Alpha", ErrInvalidName},
		{"leading_dot", ".alpha", ErrInvalidName},
		{"slash", "alpha/beta", ErrInvalidName},
		{"space", "alpha beta", ErrInvalidName},
		{"too_long", strings.Repeat("a", 65), ErrInvalidName},
		{"leading_dash", "-alpha", ErrInvalidName},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := m.Create(tc.input, CreateOptions{})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Create(%q) err=%v, want %v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func TestCreateRejectsDuplicate(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Create("dup", CreateOptions{}); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := m.Create("dup", CreateOptions{})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("second Create err=%v, want ErrAlreadyExists", err)
	}
}

func TestOpenMissingReturnsNotFound(t *testing.T) {
	m := newTestManager(t)
	_, err := m.Open("nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open missing err=%v, want ErrNotFound", err)
	}
}

func TestDestroyMissingReturnsNotFound(t *testing.T) {
	m := newTestManager(t)
	err := m.Destroy("nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Destroy missing err=%v, want ErrNotFound", err)
	}
}

func TestListEmptyAndIgnoresJunk(t *testing.T) {
	m := newTestManager(t)

	// List on empty root returns nil, nil.
	infos, err := m.List()
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if len(infos) != 0 {
		t.Fatalf("List empty returned %d", len(infos))
	}

	// Junk dir with invalid name is ignored.
	if err := os.MkdirAll(filepath.Join(m.Root(), "sandboxes", "BadName"), 0o755); err != nil {
		t.Fatalf("mkdir junk: %v", err)
	}
	if _, err := m.Create("good", CreateOptions{}); err != nil {
		t.Fatalf("Create good: %v", err)
	}
	infos, err = m.List()
	if err != nil {
		t.Fatalf("List with junk: %v", err)
	}
	if len(infos) != 1 || infos[0].Name != "good" {
		t.Fatalf("List ignored junk wrong: %+v", infos)
	}
}

func TestSessionLifecycleClearsTmpButPreservesWorkspaceAndHome(t *testing.T) {
	m := newTestManager(t)
	ws, err := m.Create("life", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Pre-populate workspace/, home/, and tmp/.
	writeFile(t, filepath.Join(ws.Path(SubdirWorkspace), "code.go"), []byte("package main"))
	writeFile(t, filepath.Join(ws.Path(SubdirHome), ".profile"), []byte("export X=1"))
	// Pre-existing tmp content from a prior session that EndSession should clear.
	writeFile(t, filepath.Join(ws.Path(SubdirTmp), "leftover.txt"), []byte("should-die"))
	writeFile(t, filepath.Join(ws.Path(SubdirCache), "warm.bin"), []byte("cached"))
	writeFile(t, filepath.Join(ws.Path(SubdirLogs), "session.log"), []byte("hello"))

	preLastUsed := ws.cfg.LastUsedAt
	time.Sleep(2 * time.Millisecond)

	sess, err := ws.StartSession()
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if sess.StartedAt().IsZero() {
		t.Fatal("StartedAt is zero")
	}

	// last_used_at should bump on disk.
	reopened, err := m.Open("life")
	if err != nil {
		t.Fatalf("Open after StartSession: %v", err)
	}
	if !reopened.cfg.LastUsedAt.After(preLastUsed) {
		t.Fatalf("LastUsedAt not bumped: pre=%v post=%v", preLastUsed, reopened.cfg.LastUsedAt)
	}

	// Add some session-scoped tmp content.
	writeFile(t, filepath.Join(ws.Path(SubdirTmp), "scratch.txt"), []byte("session-scoped"))

	if err := ws.EndSession(sess); err != nil {
		t.Fatalf("EndSession: %v", err)
	}

	// tmp/ exists but is empty.
	entries, err := os.ReadDir(ws.Path(SubdirTmp))
	if err != nil {
		t.Fatalf("read tmp: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("tmp not cleared, has %d entries", len(entries))
	}

	// workspace/ + home/ + cache/ + logs/ are preserved.
	for _, p := range []string{
		filepath.Join(ws.Path(SubdirWorkspace), "code.go"),
		filepath.Join(ws.Path(SubdirHome), ".profile"),
		filepath.Join(ws.Path(SubdirCache), "warm.bin"),
		filepath.Join(ws.Path(SubdirLogs), "session.log"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("preserved file missing: %s: %v", p, err)
		}
	}
}

func TestUsageSumsBytesAcrossNestedFiles(t *testing.T) {
	m := newTestManager(t)
	ws, err := m.Create("usage", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	writeFile(t, filepath.Join(ws.Path(SubdirWorkspace), "a.txt"), []byte("hello"))             // 5
	writeFile(t, filepath.Join(ws.Path(SubdirWorkspace), "nested", "b.txt"), []byte("worldly")) // 7
	writeFile(t, filepath.Join(ws.Path(SubdirCache), "c.bin"), []byte("xx"))                    // 2

	usage, err := ws.Usage()
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if got := usage.BySubdir[SubdirWorkspace]; got != 12 {
		t.Fatalf("workspace usage = %d, want 12", got)
	}
	if got := usage.BySubdir[SubdirCache]; got != 2 {
		t.Fatalf("cache usage = %d, want 2", got)
	}
	// total should be at least 14 (plus config.yaml at workspace root).
	if usage.TotalBytes < 14 {
		t.Fatalf("total usage = %d, want >= 14", usage.TotalBytes)
	}
}

func TestQuotaUnderAndOver(t *testing.T) {
	m := newTestManager(t)

	t.Run("under", func(t *testing.T) {
		ws, err := m.Create("under", CreateOptions{QuotaBytes: 1 << 20})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		writeFile(t, filepath.Join(ws.Path(SubdirWorkspace), "tiny"), []byte("xx"))
		res, err := ws.EnforceQuota()
		if err != nil {
			t.Fatalf("EnforceQuota: %v", err)
		}
		if res.Exceeded {
			t.Fatalf("under-quota reported Exceeded: %+v", res)
		}
		if res.OverBy != 0 {
			t.Fatalf("under-quota OverBy = %d, want 0", res.OverBy)
		}
	})

	t.Run("over", func(t *testing.T) {
		ws, err := m.Create("over", CreateOptions{QuotaBytes: 4})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		writeFile(t, filepath.Join(ws.Path(SubdirWorkspace), "big.bin"), []byte("0123456789"))
		res, err := ws.EnforceQuota()
		if err != nil {
			t.Fatalf("EnforceQuota: %v", err)
		}
		if !res.Exceeded {
			t.Fatalf("over-quota not Exceeded: %+v", res)
		}
		if res.OverBy <= 0 {
			t.Fatalf("over-quota OverBy = %d, want > 0", res.OverBy)
		}
	})
}

func TestSetQuotaPersists(t *testing.T) {
	m := newTestManager(t)
	ws, err := m.Create("setq", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := ws.SetQuota(2048); err != nil {
		t.Fatalf("SetQuota: %v", err)
	}

	got, err := m.Open("setq")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got.Quota() != 2048 {
		t.Fatalf("Quota = %d, want 2048", got.Quota())
	}

	if err := ws.SetQuota(0); err == nil {
		t.Fatal("SetQuota(0) returned nil err, want positive-required error")
	}
}

func TestCleanupBehaviour(t *testing.T) {
	m := newTestManager(t)
	ws, err := m.Create("clean", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Seed every subdir with files. workspace/ + home/ should be untouched.
	writeFile(t, filepath.Join(ws.Path(SubdirWorkspace), "keep.go"), []byte("package main"))
	writeFile(t, filepath.Join(ws.Path(SubdirHome), ".profile"), []byte("home"))
	writeFile(t, filepath.Join(ws.Path(SubdirCache), "warm.bin"), []byte("0123456789"))
	writeFile(t, filepath.Join(ws.Path(SubdirTmp), "scratch"), []byte("ephemeral"))
	oldLog := filepath.Join(ws.Path(SubdirLogs), "old.log")
	newLog := filepath.Join(ws.Path(SubdirLogs), "new.log")
	writeFile(t, oldLog, []byte("old"))
	writeFile(t, newLog, []byte("fresh"))

	// Backdate the old log by 48h.
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldLog, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	report, err := ws.Cleanup(CleanupOptions{
		Caches:    true,
		Logs:      true,
		OlderThan: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if report.TmpBytesFreed == 0 {
		t.Fatal("TmpBytesFreed = 0, want > 0")
	}
	if report.CacheBytesFreed == 0 {
		t.Fatal("CacheBytesFreed = 0, want > 0")
	}
	if report.LogBytesFreed == 0 {
		t.Fatal("LogBytesFreed = 0, want > 0")
	}

	// workspace/ and home/ untouched.
	if _, err := os.Stat(filepath.Join(ws.Path(SubdirWorkspace), "keep.go")); err != nil {
		t.Fatalf("workspace file missing after cleanup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ws.Path(SubdirHome), ".profile")); err != nil {
		t.Fatalf("home file missing after cleanup: %v", err)
	}

	// tmp/ + cache/ are empty.
	if entries, _ := os.ReadDir(ws.Path(SubdirTmp)); len(entries) != 0 {
		t.Fatalf("tmp not empty: %d entries", len(entries))
	}
	if entries, _ := os.ReadDir(ws.Path(SubdirCache)); len(entries) != 0 {
		t.Fatalf("cache not empty: %d entries", len(entries))
	}

	// Old log gone, new log preserved.
	if _, err := os.Stat(oldLog); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("old log still present: stat err=%v", err)
	}
	if _, err := os.Stat(newLog); err != nil {
		t.Fatalf("new log missing: %v", err)
	}
}

func TestCleanupTmpAlwaysClearedEvenWithoutOpts(t *testing.T) {
	m := newTestManager(t)
	ws, err := m.Create("alwaysclean", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeFile(t, filepath.Join(ws.Path(SubdirTmp), "x"), []byte("xx"))
	writeFile(t, filepath.Join(ws.Path(SubdirCache), "keep"), []byte("keep"))
	writeFile(t, filepath.Join(ws.Path(SubdirLogs), "keep.log"), []byte("keep"))

	report, err := ws.Cleanup(CleanupOptions{})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if report.TmpBytesFreed == 0 {
		t.Fatal("TmpBytesFreed = 0, want > 0")
	}
	if report.CacheBytesFreed != 0 {
		t.Fatalf("CacheBytesFreed = %d, want 0 (Caches=false)", report.CacheBytesFreed)
	}
	if report.LogBytesFreed != 0 {
		t.Fatalf("LogBytesFreed = %d, want 0 (no threshold)", report.LogBytesFreed)
	}
	if _, err := os.Stat(filepath.Join(ws.Path(SubdirCache), "keep")); err != nil {
		t.Fatalf("cache file gone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ws.Path(SubdirLogs), "keep.log")); err != nil {
		t.Fatalf("log file gone: %v", err)
	}
}

func TestCleanupLogsRequiresThreshold(t *testing.T) {
	m := newTestManager(t)
	ws, err := m.Create("logs", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	logPath := filepath.Join(ws.Path(SubdirLogs), "ancient.log")
	writeFile(t, logPath, []byte("history"))
	old := time.Now().Add(-365 * 24 * time.Hour)
	if err := os.Chtimes(logPath, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	// Logs=true but OlderThan=0 should be a no-op (per spec).
	report, err := ws.Cleanup(CleanupOptions{Logs: true, OlderThan: 0})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if report.LogBytesFreed != 0 {
		t.Fatalf("LogBytesFreed = %d, want 0", report.LogBytesFreed)
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("log was removed despite OlderThan=0: %v", err)
	}
}

func TestPathUnknownSubdirFallsBackToRoot(t *testing.T) {
	m := newTestManager(t)
	ws, err := m.Create("paths", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got := ws.Path(Subdir("bogus"))
	if got != ws.Root() {
		t.Fatalf("Path(bogus) = %s, want %s", got, ws.Root())
	}
}

func TestEndSessionRejectsConcurrentLockHolder(t *testing.T) {
	m := newTestManager(t)
	ws, err := m.Create("lockcontend", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sess, err := ws.StartSession()
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	// Manually create the lock file to simulate a concurrent holder. Because
	// the lock TTL is 2s, this call should fail with ErrLocked rather than
	// hang the test.
	lockPath := filepath.Join(ws.Root(), ".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("seed lock: %v", err)
	}
	_ = f.Close()
	t.Cleanup(func() { _ = os.Remove(lockPath) })

	err = ws.EndSession(sess)
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("EndSession err=%v, want ErrLocked", err)
	}
}

func TestConcurrentEndSessionsSerialiseSafely(t *testing.T) {
	// This test uses two real goroutines hitting EndSession on the same
	// workspace. Both should succeed eventually because the lock is released
	// at end-of-call; the test verifies the lock retry path doesn't deadlock
	// or leak the lock file.
	m := newTestManager(t)
	ws, err := m.Create("paralleldone", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sess, err := ws.StartSession()
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- ws.EndSession(sess)
		}()
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		if e != nil {
			t.Fatalf("concurrent EndSession err=%v", e)
		}
	}
	// Lock file should be released.
	if _, err := os.Stat(filepath.Join(ws.Root(), ".lock")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("lock file leaked: stat err=%v", err)
	}
}

func TestStartSessionRecreatesMissingTmp(t *testing.T) {
	m := newTestManager(t)
	ws, err := m.Create("recreate", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Remove tmp/ out-of-band; StartSession should recreate it.
	if err := os.RemoveAll(ws.Path(SubdirTmp)); err != nil {
		t.Fatalf("rm tmp: %v", err)
	}
	if _, err := ws.StartSession(); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	info, err := os.Stat(ws.Path(SubdirTmp))
	if err != nil {
		t.Fatalf("tmp not recreated: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("tmp not a dir")
	}
}

func TestEndSessionRecreatesMissingTmp(t *testing.T) {
	m := newTestManager(t)
	ws, err := m.Create("endmissing", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sess, err := ws.StartSession()
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	// Remove tmp/ out-of-band — EndSession should still succeed and leave a
	// fresh empty tmp/ behind.
	if err := os.RemoveAll(ws.Path(SubdirTmp)); err != nil {
		t.Fatalf("rm tmp: %v", err)
	}
	if err := ws.EndSession(sess); err != nil {
		t.Fatalf("EndSession: %v", err)
	}
	if _, err := os.Stat(ws.Path(SubdirTmp)); err != nil {
		t.Fatalf("tmp not recreated by EndSession: %v", err)
	}
}

func TestOpenInvalidNameReturnsErrInvalidName(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Open("Bad/Name"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("Open invalid err=%v, want ErrInvalidName", err)
	}
	if err := m.Destroy("Bad/Name"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("Destroy invalid err=%v, want ErrInvalidName", err)
	}
}

func TestOpenWithCorruptConfigReturnsParseError(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Create("corrupt", CreateOptions{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	cfgPath := filepath.Join(m.pathFor("corrupt"), "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(":\n  not-yaml: ["), 0o644); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}
	if _, err := m.Open("corrupt"); err == nil {
		t.Fatal("Open corrupt returned nil err")
	}
}

func TestOpenRejectsFilePosingAsWorkspace(t *testing.T) {
	m := newTestManager(t)
	// Create a regular file at <root>/sandboxes/file
	target := m.pathFor("file")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("not-a-dir"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := m.Open("file"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open file-as-workspace err=%v, want ErrNotFound", err)
	}
}

func TestCleanupOnEmptyTmpReportsZeroBytes(t *testing.T) {
	m := newTestManager(t)
	ws, err := m.Create("emptytmp", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	report, err := ws.Cleanup(CleanupOptions{})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if report.TmpBytesFreed != 0 || report.CacheBytesFreed != 0 || report.LogBytesFreed != 0 {
		t.Fatalf("expected zero report, got %+v", report)
	}
}

func TestUsageOnFreshWorkspaceCountsOnlyConfig(t *testing.T) {
	m := newTestManager(t)
	ws, err := m.Create("fresh", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	usage, err := ws.Usage()
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	for _, sub := range []Subdir{SubdirWorkspace, SubdirHome, SubdirCache, SubdirSnapshots, SubdirLogs, SubdirTmp} {
		if usage.BySubdir[sub] != 0 {
			t.Fatalf("%s usage = %d, want 0", sub, usage.BySubdir[sub])
		}
	}
	// total accounts for config.yaml.
	if usage.TotalBytes <= 0 {
		t.Fatalf("total = %d, want > 0 (config.yaml)", usage.TotalBytes)
	}
}

func TestEnforceQuotaUnderQuotaShape(t *testing.T) {
	m := newTestManager(t)
	ws, err := m.Create("shape", CreateOptions{QuotaBytes: 1 << 30})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	res, err := ws.EnforceQuota()
	if err != nil {
		t.Fatalf("EnforceQuota: %v", err)
	}
	if res.QuotaBytes != 1<<30 {
		t.Fatalf("QuotaBytes = %d, want 1GiB", res.QuotaBytes)
	}
	if res.UsageBytes <= 0 {
		t.Fatalf("UsageBytes = %d, want > 0", res.UsageBytes)
	}
	if res.Exceeded {
		t.Fatal("under quota reported Exceeded")
	}
}

func TestNewManagerFallsBackWhenHomeUnavailable(t *testing.T) {
	// On macOS UserHomeDir reads $HOME first; clearing it forces the fallback
	// path. We restore the env afterward.
	t.Setenv("HOME", "")
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", "")
	}
	m := NewManager("")
	if m.Root() == "" {
		t.Fatal("Root empty even after fallback")
	}
}

func TestListMissingRootIsEmpty(t *testing.T) {
	// Pass a deeply-nested non-existent path; List should return (nil, nil).
	m := NewManager(filepath.Join(t.TempDir(), "does", "not", "exist"))
	infos, err := m.List()
	if err != nil {
		t.Fatalf("List on missing root err=%v", err)
	}
	if infos != nil {
		t.Fatalf("List on missing root = %+v, want nil", infos)
	}
}

func TestPruneOldFilesPreservesNewer(t *testing.T) {
	m := newTestManager(t)
	ws, err := m.Create("prune", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	a := filepath.Join(ws.Path(SubdirLogs), "a.log")
	b := filepath.Join(ws.Path(SubdirLogs), "nested", "b.log")
	writeFile(t, a, []byte("aaaaa"))
	writeFile(t, b, []byte("bbbbb"))
	old := time.Now().Add(-100 * time.Hour)
	if err := os.Chtimes(a, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	report, err := ws.Cleanup(CleanupOptions{Logs: true, OlderThan: 24 * time.Hour})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if report.LogBytesFreed != 5 {
		t.Fatalf("LogBytesFreed = %d, want 5", report.LogBytesFreed)
	}
	if _, err := os.Stat(a); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("old log survived: %v", err)
	}
	if _, err := os.Stat(b); err != nil {
		t.Fatalf("new log gone: %v", err)
	}
}

func TestClearDirAtomicMissingDirRecreatesIt(t *testing.T) {
	// Direct unit test of the helper for the missing-dir branch. We reach it
	// via Cleanup on a workspace whose tmp/ has been removed.
	m := newTestManager(t)
	ws, err := m.Create("missingtmp", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := os.RemoveAll(ws.Path(SubdirTmp)); err != nil {
		t.Fatalf("rm tmp: %v", err)
	}
	report, err := ws.Cleanup(CleanupOptions{})
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if report.TmpBytesFreed != 0 {
		t.Fatalf("TmpBytesFreed = %d, want 0", report.TmpBytesFreed)
	}
	if _, err := os.Stat(ws.Path(SubdirTmp)); err != nil {
		t.Fatalf("tmp not recreated: %v", err)
	}
}

func TestClearDirAtomicRejectsNonDir(t *testing.T) {
	// If a caller (not the public API) passes a regular file, clearDirAtomic
	// must report an error. We exercise this through the helper directly.
	tmp := t.TempDir()
	target := filepath.Join(tmp, "file")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, _, err := clearDirAtomic(target); err == nil {
		t.Fatal("clearDirAtomic on file returned nil err")
	}
}

func TestDirSizeMissingReturnsZero(t *testing.T) {
	tmp := t.TempDir()
	got, err := dirSize(filepath.Join(tmp, "absent"))
	if err != nil {
		t.Fatalf("dirSize missing: %v", err)
	}
	if got != 0 {
		t.Fatalf("dirSize missing = %d, want 0", got)
	}
}

func TestCreateFailsWhenFileBlocksWorkspacePath(t *testing.T) {
	m := newTestManager(t)
	if err := os.MkdirAll(m.sandboxesPath(), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Place a regular file where the workspace dir wants to live.
	if err := os.WriteFile(m.pathFor("blocked"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := m.Create("blocked", CreateOptions{}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("Create over file err=%v, want ErrAlreadyExists", err)
	}
}

func TestCreateInUnwritableRootFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses mode bits")
	}
	root := t.TempDir()
	// Pre-create sandboxes/ then make it 0o555 (no-write) so MkdirAll fails.
	sb := filepath.Join(root, "sandboxes")
	if err := os.MkdirAll(sb, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(sb, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(sb, 0o755) })

	m := NewManager(root)
	if _, err := m.Create("denied", CreateOptions{}); err == nil {
		t.Fatal("Create succeeded against read-only sandboxes/")
	}
}

func TestStartSessionFailsWhenConfigUnwritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses mode bits")
	}
	m := newTestManager(t)
	ws, err := m.Create("ro", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Replace config.yaml with a directory so writeConfig (which does
	// WriteFile) fails. We restore it on cleanup so the workspace is
	// destroyable under t.TempDir().
	cfgPath := filepath.Join(ws.Root(), "config.yaml")
	if err := os.Remove(cfgPath); err != nil {
		t.Fatalf("rm cfg: %v", err)
	}
	if err := os.Mkdir(cfgPath, 0o755); err != nil {
		t.Fatalf("mkdir cfg: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(cfgPath) })

	if _, err := ws.StartSession(); err == nil {
		t.Fatal("StartSession with config.yaml-as-dir returned nil err")
	}
}

func TestUsageReportsErrorWhenSubdirUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses mode bits")
	}
	m := newTestManager(t)
	ws, err := m.Create("noread", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Put a file inside cache/, then chmod cache/ to 0o000 so WalkDir cannot
	// read its contents. Restore on cleanup so t.TempDir teardown works.
	writeFile(t, filepath.Join(ws.Path(SubdirCache), "x.bin"), []byte("xx"))
	if err := os.Chmod(ws.Path(SubdirCache), 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(ws.Path(SubdirCache), 0o755) })

	if _, err := ws.Usage(); err == nil {
		t.Fatal("Usage on unreadable subdir returned nil err")
	}
}

func TestCleanupReportsErrorWhenLogsUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses mode bits")
	}
	m := newTestManager(t)
	ws, err := m.Create("logsro", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Seed a log so WalkDir must descend, then make logs/ unreadable.
	writeFile(t, filepath.Join(ws.Path(SubdirLogs), "old.log"), []byte("x"))
	old := time.Now().Add(-365 * 24 * time.Hour)
	_ = os.Chtimes(filepath.Join(ws.Path(SubdirLogs), "old.log"), old, old)
	if err := os.Chmod(ws.Path(SubdirLogs), 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(ws.Path(SubdirLogs), 0o755) })

	if _, err := ws.Cleanup(CleanupOptions{Logs: true, OlderThan: 1 * time.Hour}); err == nil {
		t.Fatal("Cleanup on unreadable logs/ returned nil err")
	}
}

func TestCleanupCacheReportsErrorWhenUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses mode bits")
	}
	m := newTestManager(t)
	ws, err := m.Create("cachero", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeFile(t, filepath.Join(ws.Path(SubdirCache), "x"), []byte("xx"))
	if err := os.Chmod(ws.Path(SubdirCache), 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(ws.Path(SubdirCache), 0o755) })

	if _, err := ws.Cleanup(CleanupOptions{Caches: true}); err == nil {
		t.Fatal("Cleanup on unreadable cache returned nil err")
	}
}

func TestDestroyOnFileReturnsErrorThroughRemove(t *testing.T) {
	// Force the Destroy path to take rm-based cleanup. We use a regular file
	// posing as a workspace dir; validateName allows the name, Stat succeeds,
	// RemoveAll succeeds — the file is removed.
	m := newTestManager(t)
	if err := os.MkdirAll(m.sandboxesPath(), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	target := m.pathFor("file")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := m.Destroy("file"); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("file still present: %v", err)
	}
}

func TestCleanupAcquiresLock(t *testing.T) {
	m := newTestManager(t)
	ws, err := m.Create("cleanlock", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	lockPath := filepath.Join(ws.Root(), ".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("seed lock: %v", err)
	}
	_ = f.Close()
	t.Cleanup(func() { _ = os.Remove(lockPath) })

	_, err = ws.Cleanup(CleanupOptions{})
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("Cleanup err=%v, want ErrLocked", err)
	}
}
