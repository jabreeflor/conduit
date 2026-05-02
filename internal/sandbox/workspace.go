// Package sandbox manages the on-disk persistent workspace layout for named
// sandboxes (PRD §15.5).
//
// Each sandbox lives at <root>/sandboxes/<name>/ and contains:
//
//	workspace/    persistent user files
//	home/         persistent fake $HOME
//	cache/        persistent caches
//	snapshots/    snapshot bundles (snapshot creation is out of scope here)
//	logs/         persistent logs
//	tmp/          ephemeral; cleared on EndSession
//	config.yaml   workspace config (name, created_at, quota_bytes, last_used_at)
//
// The default disk quota is 10 GiB. Cleanup tooling clears tmp, optionally
// caches, and optionally aged log files; it never touches workspace/ or home/.
//
// This package is the on-disk substrate only — it is intentionally separate
// from internal/core.SandboxManager, which validates sandbox architecture and
// mount policy. The two concerns can be wired together by higher-level code
// in a follow-up.
package sandbox

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultQuotaBytes is the default disk quota applied to a new workspace
// (PRD §15.5: 10 GiB).
const DefaultQuotaBytes int64 = 10 * 1024 * 1024 * 1024

// Permissions used across the workspace tree.
const (
	dirPerm        fs.FileMode = 0o755
	configFilePerm fs.FileMode = 0o644
	lockFilePerm   fs.FileMode = 0o644
)

const (
	configFileName = "config.yaml"
	lockFileName   = ".lock"
	sandboxesDir   = "sandboxes"
)

var nameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// Sentinel errors callers can wrap-test against with errors.Is.
var (
	// ErrInvalidName is returned when a workspace name fails validation.
	ErrInvalidName = errors.New("invalid sandbox workspace name")
	// ErrAlreadyExists is returned by Create when the workspace already exists.
	ErrAlreadyExists = errors.New("sandbox workspace already exists")
	// ErrNotFound is returned by Open / Destroy when the workspace is missing.
	ErrNotFound = errors.New("sandbox workspace not found")
	// ErrLocked is returned when a session-scoped operation cannot acquire the
	// per-workspace file lock.
	ErrLocked = errors.New("sandbox workspace is locked by another session")
)

// Subdir identifies one of the canonical sub-paths inside a workspace.
type Subdir string

const (
	SubdirWorkspace Subdir = "workspace"
	SubdirHome      Subdir = "home"
	SubdirCache     Subdir = "cache"
	SubdirSnapshots Subdir = "snapshots"
	SubdirLogs      Subdir = "logs"
	SubdirTmp       Subdir = "tmp"
)

// allSubdirs is the canonical layout, in stable order, used by Create.
var allSubdirs = []Subdir{
	SubdirWorkspace,
	SubdirHome,
	SubdirCache,
	SubdirSnapshots,
	SubdirLogs,
	SubdirTmp,
}

// CreateOptions controls Create. A zero CreateOptions yields a 10 GiB quota.
type CreateOptions struct {
	// QuotaBytes is the disk quota applied to the workspace. Values <= 0
	// fall back to DefaultQuotaBytes.
	QuotaBytes int64
}

// WorkspaceInfo is a lightweight directory-listing record returned by List.
type WorkspaceInfo struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
	QuotaBytes int64     `json:"quota_bytes"`
}

// UsageReport summarises bytes-on-disk for one workspace, broken down by
// subdir.
type UsageReport struct {
	TotalBytes int64            `json:"total_bytes"`
	BySubdir   map[Subdir]int64 `json:"by_subdir"`
}

// QuotaResult is the answer EnforceQuota returns: usage versus the configured
// quota.
type QuotaResult struct {
	UsageBytes int64 `json:"usage_bytes"`
	QuotaBytes int64 `json:"quota_bytes"`
	Exceeded   bool  `json:"exceeded"`
	OverBy     int64 `json:"over_by"`
}

// CleanupOptions controls Cleanup. tmp/ is always cleared; the rest are
// opt-in.
type CleanupOptions struct {
	// Caches, when true, clears the entire cache/ subdir.
	Caches bool
	// Logs, when true alongside OlderThan > 0, removes log files older than
	// OlderThan ago by mtime. Logs=true with OlderThan=0 is a no-op (we
	// require an explicit threshold so users can't accidentally wipe history).
	Logs bool
	// OlderThan is the mtime threshold for log file removal. Files older
	// than time.Now().Add(-OlderThan) are removed.
	OlderThan time.Duration
}

// CleanupReport reports bytes freed per category.
type CleanupReport struct {
	TmpBytesFreed   int64 `json:"tmp_bytes_freed"`
	CacheBytesFreed int64 `json:"cache_bytes_freed"`
	LogBytesFreed   int64 `json:"log_bytes_freed"`
}

// workspaceConfig is the on-disk YAML schema for config.yaml. Field tags use
// snake_case so the file is friendly to manual edits.
type workspaceConfig struct {
	Name       string    `yaml:"name"`
	CreatedAt  time.Time `yaml:"created_at"`
	LastUsedAt time.Time `yaml:"last_used_at"`
	QuotaBytes int64     `yaml:"quota_bytes"`
}

// Manager is the entry point: Create / Open / List / Destroy named workspaces.
type Manager struct {
	root string // absolute, points at <root>/.. (the directory that contains "sandboxes/")
}

// NewManager returns a Manager rooted at root. If root is empty the user's
// $HOME/.conduit is used, falling back to the OS temp dir when even that is
// unavailable so the construction is always non-nil.
func NewManager(root string) *Manager {
	if root == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			root = filepath.Join(home, ".conduit")
		} else {
			root = filepath.Join(os.TempDir(), "conduit")
		}
	}
	return &Manager{root: filepath.Clean(root)}
}

// Root returns the absolute root path the manager operates under. Useful for
// tests and diagnostics.
func (m *Manager) Root() string { return m.root }

// sandboxesPath returns <root>/sandboxes.
func (m *Manager) sandboxesPath() string {
	return filepath.Join(m.root, sandboxesDir)
}

// pathFor returns <root>/sandboxes/<name>.
func (m *Manager) pathFor(name string) string {
	return filepath.Join(m.sandboxesPath(), name)
}

// validateName enforces the layout invariant for workspace names so we never
// stash arbitrary path segments under <root>/sandboxes.
func validateName(name string) error {
	if !nameRegex.MatchString(name) {
		return fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	return nil
}

// Create materialises a new workspace under the manager's root. It returns
// ErrAlreadyExists if a workspace with the same name is already present, and
// ErrInvalidName for names that do not match the regex.
func (m *Manager) Create(name string, opts CreateOptions) (*Workspace, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}

	path := m.pathFor(name)
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrAlreadyExists, name)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	if err := os.MkdirAll(path, dirPerm); err != nil {
		return nil, fmt.Errorf("create workspace dir: %w", err)
	}

	for _, sub := range allSubdirs {
		if err := os.MkdirAll(filepath.Join(path, string(sub)), dirPerm); err != nil {
			return nil, fmt.Errorf("create %s: %w", sub, err)
		}
	}

	quota := opts.QuotaBytes
	if quota <= 0 {
		quota = DefaultQuotaBytes
	}

	now := time.Now().UTC()
	cfg := workspaceConfig{
		Name:       name,
		CreatedAt:  now,
		LastUsedAt: now,
		QuotaBytes: quota,
	}
	if err := writeConfig(path, cfg); err != nil {
		return nil, err
	}

	return &Workspace{path: path, name: name, cfg: cfg}, nil
}

// Open returns a handle to an existing workspace. ErrNotFound is returned when
// the sandbox directory does not exist.
func (m *Manager) Open(name string) (*Workspace, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}

	path := m.pathFor(name)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s is not a directory", ErrNotFound, name)
	}

	cfg, err := readConfig(path)
	if err != nil {
		return nil, err
	}
	return &Workspace{path: path, name: name, cfg: cfg}, nil
}

// List returns a stable, name-sorted view of every workspace under the root.
// Missing or unreadable config.yaml files are skipped (with their Name field
// preserved) rather than aborting the whole listing.
func (m *Manager) List() ([]WorkspaceInfo, error) {
	entries, err := os.ReadDir(m.sandboxesPath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sandboxes dir: %w", err)
	}

	out := make([]WorkspaceInfo, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if err := validateName(name); err != nil {
			// Skip directories that don't match the schema rather than
			// failing the whole listing.
			continue
		}
		path := m.pathFor(name)
		info := WorkspaceInfo{Name: name, Path: path}
		if cfg, err := readConfig(path); err == nil {
			info.CreatedAt = cfg.CreatedAt
			info.LastUsedAt = cfg.LastUsedAt
			info.QuotaBytes = cfg.QuotaBytes
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Destroy removes the workspace tree. Returns ErrNotFound if the workspace
// does not exist; the caller can errors.Is-test for idempotent teardown.
func (m *Manager) Destroy(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	path := m.pathFor(name)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove workspace: %w", err)
	}
	return nil
}

// Workspace is a handle to one persistent sandbox directory.
type Workspace struct {
	path string
	name string
	cfg  workspaceConfig
}

// Name returns the workspace's name.
func (w *Workspace) Name() string { return w.name }

// Path returns the absolute path of the requested subdir. Unknown values
// return the workspace root.
func (w *Workspace) Path(sub Subdir) string {
	switch sub {
	case SubdirWorkspace, SubdirHome, SubdirCache, SubdirSnapshots, SubdirLogs, SubdirTmp:
		return filepath.Join(w.path, string(sub))
	default:
		return w.path
	}
}

// Root returns the absolute workspace directory itself (containing
// config.yaml and the subdirs).
func (w *Workspace) Root() string { return w.path }

// Quota returns the configured disk quota in bytes.
func (w *Workspace) Quota() int64 { return w.cfg.QuotaBytes }

// SetQuota updates the workspace quota and persists the change to
// config.yaml. quota must be > 0.
func (w *Workspace) SetQuota(bytes int64) error {
	if bytes <= 0 {
		return fmt.Errorf("quota must be positive, got %d", bytes)
	}
	w.cfg.QuotaBytes = bytes
	return writeConfig(w.path, w.cfg)
}

// Session is a lightweight token returned by StartSession; pass it to
// EndSession to clear tmp/.
type Session struct {
	startedAt time.Time
}

// StartedAt is the wall-clock time the session started.
func (s *Session) StartedAt() time.Time { return s.startedAt }

// StartSession marks the workspace as in use by touching last_used_at and
// returns a session token. tmp/ is created (idempotently) so callers can
// rely on its existence even if it was deleted out-of-band.
func (w *Workspace) StartSession() (*Session, error) {
	now := time.Now().UTC()
	w.cfg.LastUsedAt = now
	if err := writeConfig(w.path, w.cfg); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(w.Path(SubdirTmp), dirPerm); err != nil {
		return nil, fmt.Errorf("ensure tmp dir: %w", err)
	}
	return &Session{startedAt: now}, nil
}

// EndSession clears tmp/ atomically (rename-then-rm) so a partial failure
// does not strand half-deleted state. workspace/, home/, cache/, snapshots/,
// and logs/ are preserved.
//
// EndSession is guarded by a per-workspace file lock; concurrent sessions on
// the same workspace are rejected with an ErrLocked-wrapped error.
func (w *Workspace) EndSession(_ *Session) error {
	release, err := w.acquireLock()
	if err != nil {
		return err
	}
	defer release()

	if _, _, err := clearDirAtomic(w.Path(SubdirTmp)); err != nil {
		return fmt.Errorf("clear tmp: %w", err)
	}
	return nil
}

// Usage walks the entire workspace tree and sums file sizes by subdir. The
// total includes config.yaml and any other files at the workspace root.
func (w *Workspace) Usage() (UsageReport, error) {
	report := UsageReport{BySubdir: make(map[Subdir]int64, len(allSubdirs))}
	for _, sub := range allSubdirs {
		size, err := dirSize(w.Path(sub))
		if err != nil {
			return UsageReport{}, fmt.Errorf("size %s: %w", sub, err)
		}
		report.BySubdir[sub] = size
		report.TotalBytes += size
	}

	// Include workspace-root files (e.g. config.yaml) so the total reflects
	// real disk footprint.
	entries, err := os.ReadDir(w.path)
	if err != nil {
		return UsageReport{}, fmt.Errorf("read workspace root: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return UsageReport{}, fmt.Errorf("stat %s: %w", entry.Name(), err)
		}
		report.TotalBytes += info.Size()
	}
	return report, nil
}

// EnforceQuota compares current usage against the configured quota.
func (w *Workspace) EnforceQuota() (QuotaResult, error) {
	usage, err := w.Usage()
	if err != nil {
		return QuotaResult{}, err
	}
	res := QuotaResult{
		UsageBytes: usage.TotalBytes,
		QuotaBytes: w.cfg.QuotaBytes,
	}
	if usage.TotalBytes > w.cfg.QuotaBytes {
		res.Exceeded = true
		res.OverBy = usage.TotalBytes - w.cfg.QuotaBytes
	}
	return res, nil
}

// Cleanup always clears tmp/, optionally clears cache/, and optionally
// removes log files older than opts.OlderThan. workspace/ and home/ are never
// touched.
//
// Cleanup is guarded by the same per-workspace lock as EndSession.
func (w *Workspace) Cleanup(opts CleanupOptions) (CleanupReport, error) {
	release, err := w.acquireLock()
	if err != nil {
		return CleanupReport{}, err
	}
	defer release()

	report := CleanupReport{}

	tmpFreed, _, err := clearDirAtomic(w.Path(SubdirTmp))
	if err != nil {
		return CleanupReport{}, fmt.Errorf("clear tmp: %w", err)
	}
	report.TmpBytesFreed = tmpFreed

	if opts.Caches {
		cacheFreed, _, err := clearDirAtomic(w.Path(SubdirCache))
		if err != nil {
			return CleanupReport{}, fmt.Errorf("clear cache: %w", err)
		}
		report.CacheBytesFreed = cacheFreed
	}

	if opts.Logs && opts.OlderThan > 0 {
		freed, err := pruneOldFiles(w.Path(SubdirLogs), time.Now().Add(-opts.OlderThan))
		if err != nil {
			return CleanupReport{}, fmt.Errorf("prune logs: %w", err)
		}
		report.LogBytesFreed = freed
	}

	return report, nil
}

// writeConfig persists workspaceConfig to <path>/config.yaml with 0o644.
func writeConfig(path string, cfg workspaceConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	target := filepath.Join(path, configFileName)
	if err := os.WriteFile(target, data, configFilePerm); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	return nil
}

// readConfig parses <path>/config.yaml.
func readConfig(path string) (workspaceConfig, error) {
	target := filepath.Join(path, configFileName)
	data, err := os.ReadFile(target)
	if err != nil {
		return workspaceConfig{}, fmt.Errorf("read %s: %w", target, err)
	}
	var cfg workspaceConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return workspaceConfig{}, fmt.Errorf("parse %s: %w", target, err)
	}
	return cfg, nil
}

// dirSize sums the regular-file byte counts under root, walking subdirs.
// A missing root returns 0 with no error so newly-created workspaces with
// hand-deleted subdirs don't poison Usage.
func dirSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	return total, nil
}

// clearDirAtomic empties dir of its contents atomically: it renames dir to a
// sibling staging path, recreates an empty dir, and then removes the staging
// tree. If the dir is missing it is recreated and 0 bytes reported.
//
// Returns (bytesFreed, fileCount, error).
func clearDirAtomic(dir string) (int64, int, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if mkErr := os.MkdirAll(dir, dirPerm); mkErr != nil {
				return 0, 0, fmt.Errorf("recreate %s: %w", dir, mkErr)
			}
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return 0, 0, fmt.Errorf("%s is not a directory", dir)
	}

	bytes, err := dirSize(dir)
	if err != nil {
		return 0, 0, err
	}
	count, err := countFiles(dir)
	if err != nil {
		return 0, 0, err
	}

	staging := dir + ".purge-" + fmt.Sprintf("%d", time.Now().UnixNano())
	if err := os.Rename(dir, staging); err != nil {
		return 0, 0, fmt.Errorf("rename %s: %w", dir, err)
	}
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		// best-effort: leave staging in place so a follow-up call can
		// recover, but surface the error.
		return 0, 0, fmt.Errorf("recreate %s: %w", dir, err)
	}
	if err := os.RemoveAll(staging); err != nil {
		return bytes, count, fmt.Errorf("remove staging %s: %w", staging, err)
	}
	return bytes, count, nil
}

// countFiles counts regular files under root.
func countFiles(root string) (int, error) {
	var n int
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			n++
		}
		return nil
	})
	return n, err
}

// pruneOldFiles removes regular files under root whose mtime is strictly
// before threshold. Empty directories left behind are kept; we only delete
// files. Returns total bytes freed.
func pruneOldFiles(root string, threshold time.Time) (int64, error) {
	var freed int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.ModTime().Before(threshold) {
			size := info.Size()
			if rmErr := os.Remove(path); rmErr != nil {
				return fmt.Errorf("remove %s: %w", path, rmErr)
			}
			freed += size
		}
		return nil
	})
	if err != nil {
		return freed, err
	}
	return freed, nil
}

// acquireLock takes the <workspace>/.lock file using O_CREATE|O_EXCL with a
// short retry/timeout window so concurrent sessions on the same workspace
// fail loudly. Returns a release function that removes the lock file.
//
// We deliberately avoid syscall flock here so the dependency surface stays
// at zero and the behaviour is identical on darwin/linux.
func (w *Workspace) acquireLock() (func(), error) {
	lockPath := filepath.Join(w.path, lockFileName)
	deadline := time.Now().Add(2 * time.Second)
	backoff := 5 * time.Millisecond
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, lockFilePerm)
		if err == nil {
			// Best-effort write of the holder PID for diagnostics.
			fmt.Fprintf(f, "pid=%d\nstart=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))
			_ = f.Close()
			return func() { _ = os.Remove(lockPath) }, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("acquire lock %s: %w", lockPath, err)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: %s", ErrLocked, lockPath)
		}
		time.Sleep(backoff)
		if backoff < 80*time.Millisecond {
			backoff *= 2
		}
	}
}
