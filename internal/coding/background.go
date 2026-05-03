package coding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// BackgroundState classifies a daemon session's lifecycle.
type BackgroundState string

const (
	BackgroundRunning   BackgroundState = "running"
	BackgroundCompleted BackgroundState = "completed"
	BackgroundFailed    BackgroundState = "failed"
	BackgroundKilled    BackgroundState = "killed"
)

const (
	backgroundSubdir = "background-sessions"
)

// BackgroundJob is the persisted record for one daemon coding session
// (`conduit code-bg`). One job has exactly one underlying coding Session
// — the JSONL journal stays the source of truth for transcript content,
// while the BackgroundJob struct captures lifecycle/control metadata.
type BackgroundJob struct {
	ID         string          `json:"id"`
	SessionID  string          `json:"session_id"`
	Prompt     string          `json:"prompt"`
	State      BackgroundState `json:"state"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt time.Time       `json:"finished_at,omitempty"`
	Error      string          `json:"error,omitempty"`
	LogPath    string          `json:"log_path"`
	PID        int             `json:"pid,omitempty"`
}

// BackgroundManager owns the in-process registry of running daemon
// sessions plus the on-disk index used by `code-ps` / `code-logs` after
// a process restart. The manager is safe for concurrent use; each job
// gets its own goroutine and append-only log file.
type BackgroundManager struct {
	homeDir string
	mu      sync.Mutex
	jobs    map[string]*backgroundEntry
}

type backgroundEntry struct {
	job    BackgroundJob
	cancel context.CancelFunc
	done   chan struct{}
}

// NewBackgroundManager wires a manager rooted at home/.conduit. The
// directory is created lazily on the first Start.
func NewBackgroundManager(homeDir string) (*BackgroundManager, error) {
	if homeDir == "" {
		return nil, errors.New("background: home dir required")
	}
	return &BackgroundManager{homeDir: homeDir, jobs: map[string]*backgroundEntry{}}, nil
}

// BackgroundStartOptions controls one launch.
type BackgroundStartOptions struct {
	Prompt   string
	Streamer Streamer
	// MaxInputTokens caps the per-job context budget; 0 picks the default.
	MaxInputTokens int
}

// Start launches a daemon coding session and returns immediately. The
// returned BackgroundJob has its initial state populated; later calls
// to Get/List see live updates as the goroutine progresses.
func (m *BackgroundManager) Start(ctx context.Context, opts BackgroundStartOptions) (BackgroundJob, error) {
	if opts.Streamer == nil {
		return BackgroundJob{}, errors.New("background: streamer required")
	}
	if strings.TrimSpace(opts.Prompt) == "" {
		return BackgroundJob{}, errors.New("background: prompt required")
	}
	dir := filepath.Join(m.homeDir, ".conduit", backgroundSubdir)
	if err := os.MkdirAll(dir, sessionDirPerm); err != nil {
		return BackgroundJob{}, fmt.Errorf("background: create dir: %w", err)
	}

	sess, err := NewSession(m.homeDir)
	if err != nil {
		return BackgroundJob{}, err
	}
	if _, err := sess.Append(contracts.CodingTurn{Role: "user", Content: opts.Prompt}); err != nil {
		return BackgroundJob{}, err
	}

	jobID := "bg-" + sess.ID
	logPath := filepath.Join(dir, jobID+".log")
	job := BackgroundJob{
		ID:        jobID,
		SessionID: sess.ID,
		Prompt:    opts.Prompt,
		State:     BackgroundRunning,
		StartedAt: time.Now().UTC(),
		LogPath:   logPath,
		PID:       os.Getpid(),
	}

	jobCtx, cancel := context.WithCancel(ctx)
	entry := &backgroundEntry{job: job, cancel: cancel, done: make(chan struct{})}

	m.mu.Lock()
	m.jobs[jobID] = entry
	m.mu.Unlock()

	if err := m.persist(job); err != nil {
		cancel()
		return BackgroundJob{}, err
	}

	go m.run(jobCtx, entry, sess, opts)
	return job, nil
}

func (m *BackgroundManager) run(ctx context.Context, entry *backgroundEntry, sess *Session, opts BackgroundStartOptions) {
	defer close(entry.done)

	logF, err := os.OpenFile(entry.job.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, sessionFilePerm)
	if err != nil {
		m.finish(entry, BackgroundFailed, err)
		return
	}
	defer logF.Close()

	// Stream into the log file so `code-logs` can tail it live without
	// touching the JSONL journal mid-write.
	full, _, streamErr := opts.Streamer.Stream(ctx, opts.Prompt, func(delta string) {
		_, _ = logF.WriteString(delta)
	})
	if streamErr != nil {
		state := BackgroundFailed
		if errors.Is(streamErr, context.Canceled) {
			state = BackgroundKilled
		}
		m.finish(entry, state, streamErr)
		return
	}
	if _, err := sess.Append(contracts.CodingTurn{Role: "assistant", Content: full}); err != nil {
		m.finish(entry, BackgroundFailed, err)
		return
	}
	m.finish(entry, BackgroundCompleted, nil)
}

func (m *BackgroundManager) finish(entry *backgroundEntry, state BackgroundState, err error) {
	m.mu.Lock()
	entry.job.State = state
	entry.job.FinishedAt = time.Now().UTC()
	if err != nil {
		entry.job.Error = err.Error()
	}
	snapshot := entry.job
	m.mu.Unlock()
	_ = m.persist(snapshot)
}

// Kill cancels a running job. Already-finished jobs are no-ops.
func (m *BackgroundManager) Kill(id string) error {
	m.mu.Lock()
	entry, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("background: unknown job %q", id)
	}
	if entry.job.State != BackgroundRunning {
		return nil
	}
	entry.cancel()
	return nil
}

// Get returns a snapshot of one job; the returned struct is safe to
// inspect and mutate independently of the manager.
func (m *BackgroundManager) Get(id string) (BackgroundJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.jobs[id]
	if !ok {
		return BackgroundJob{}, fmt.Errorf("background: unknown job %q", id)
	}
	return entry.job, nil
}

// List returns all in-process jobs sorted by start time descending. For
// jobs that finished in a previous process invocation, callers should
// fall back to ListPersisted.
func (m *BackgroundManager) List() []BackgroundJob {
	m.mu.Lock()
	out := make([]BackgroundJob, 0, len(m.jobs))
	for _, e := range m.jobs {
		out = append(out, e.job)
	}
	m.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out
}

// Wait blocks until the named job's goroutine finishes.
func (m *BackgroundManager) Wait(id string) error {
	m.mu.Lock()
	entry, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("background: unknown job %q", id)
	}
	<-entry.done
	return nil
}

// ReadLog returns the entire log file contents for the named job. Callers
// that want to tail incrementally can open job.LogPath directly.
func (m *BackgroundManager) ReadLog(id string) ([]byte, error) {
	job, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(job.LogPath)
}

// persist writes the job snapshot to its index file. The on-disk index
// lets `code-ps` after a restart still see what ran in this session,
// even if the goroutines are gone.
func (m *BackgroundManager) persist(job BackgroundJob) error {
	dir := filepath.Join(m.homeDir, ".conduit", backgroundSubdir)
	if err := os.MkdirAll(dir, sessionDirPerm); err != nil {
		return err
	}
	idxPath := filepath.Join(dir, job.ID+".json")
	tmp := idxPath + ".tmp"
	b, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, sessionFilePerm); err != nil {
		return err
	}
	return os.Rename(tmp, idxPath)
}

// ListPersisted returns every job whose index file exists on disk,
// including those started by previous processes. The list is sorted by
// start time descending.
func (m *BackgroundManager) ListPersisted() ([]BackgroundJob, error) {
	dir := filepath.Join(m.homeDir, ".conduit", backgroundSubdir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []BackgroundJob
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var job BackgroundJob
		if err := json.Unmarshal(b, &job); err != nil {
			continue
		}
		out = append(out, job)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out, nil
}
