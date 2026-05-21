package coding

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

const (
	// sessionsSubdir keeps coding-agent journals separate from usage logs and
	// other ~/.conduit artefacts so a single session can be inspected without
	// cross-cutting noise.
	sessionsSubdir  = "coding-sessions"
	sessionDirPerm  = 0o700
	sessionFilePerm = 0o600
)

// Session is the append-only journal for one `conduit code` REPL run.
// Turns are kept in memory for in-process inspection (status surface,
// auto-continuation, in-flight compaction) and mirrored to a JSONL file
// for resumability across restarts.
type Session struct {
	ID              string
	StartedAt       time.Time
	Turns           []contracts.CodingTurn
	Path            string
	RepositoryRoot  string
	CWD             string
	Branch          string
	WorktreePath    string
	WorktreeBranch  string
	WorktreeActive  bool
	WorktreeHistory []contracts.CodingWorktreeEvent

	mu sync.Mutex
	// homeDir is retained so future helpers (LoadSession variants, log
	// rotation) can re-derive paths without the caller threading it.
	homeDir string
}

// NewSession creates a session, ensures the journal directory exists, and
// reserves the JSONL path. The file itself is created lazily on first
// Append so an aborted REPL never leaves a zero-byte journal behind.
func NewSession(home string) (*Session, error) {
	return NewSessionInDir(home, "")
}

// NewSessionInDir creates a session and records the current git cwd/branch
// when cwd is inside a git repository.
func NewSessionInDir(home, cwd string) (*Session, error) {
	if home == "" {
		return nil, fmt.Errorf("coding: home dir required")
	}
	id, err := newSessionID()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".conduit", sessionsSubdir)
	if err := os.MkdirAll(dir, sessionDirPerm); err != nil {
		return nil, fmt.Errorf("coding: create session dir: %w", err)
	}
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}
	repo, branch := gitContext(cwd)
	return &Session{
		ID:             id,
		StartedAt:      time.Now().UTC(),
		Path:           filepath.Join(dir, id+".jsonl"),
		homeDir:        home,
		RepositoryRoot: repo,
		CWD:            cwd,
		Branch:         branch,
	}, nil
}

// NewLocalSession creates a coding session in a managed git worktree, matching
// the chat.newLocal command semantics from the GUI/keybinding layer.
func NewLocalSession(ctx context.Context, home, repoPath, branch string) (*Session, error) {
	s, err := NewSessionInDir(home, repoPath)
	if err != nil {
		return nil, err
	}
	if _, err := s.WorktreeEnter(ctx, WorktreeEnterOptions{Branch: branch}); err != nil {
		return nil, err
	}
	return s, nil
}

// Append assigns a snapshot ID, persists the turn to the JSONL journal,
// and records it in the in-memory slice. Persistence happens before the
// in-memory append so a crash cannot leave the runtime believing in a
// turn that never made it to disk.
func (s *Session) Append(turn contracts.CodingTurn) (contracts.CodingSnapshotID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if turn.At.IsZero() {
		turn.At = time.Now().UTC()
	}
	if turn.Index == 0 {
		turn.Index = len(s.Turns)
	}
	if turn.CWD == "" {
		turn.CWD = s.CWD
	}
	if turn.GitBranch == "" {
		turn.GitBranch = s.Branch
	}
	if turn.RepositoryRoot == "" {
		turn.RepositoryRoot = s.RepositoryRoot
	}
	if turn.WorktreePath == "" {
		turn.WorktreePath = s.WorktreePath
	}
	if turn.WorktreeBranch == "" {
		turn.WorktreeBranch = s.WorktreeBranch
	}
	turn.WorktreeActive = s.WorktreeActive
	id, err := newSnapshotID(turn.At)
	if err != nil {
		return "", err
	}
	turn.SnapshotID = id

	line, err := json.Marshal(turn)
	if err != nil {
		return "", fmt.Errorf("coding: marshal turn: %w", err)
	}
	line = append(line, '\n')

	f, err := os.OpenFile(s.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, sessionFilePerm)
	if err != nil {
		return "", fmt.Errorf("coding: open session journal: %w", err)
	}
	if _, err := f.Write(line); err != nil {
		f.Close()
		return "", fmt.Errorf("coding: write turn: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("coding: close session journal: %w", err)
	}

	s.Turns = append(s.Turns, turn)
	return id, nil
}

// LoadSession reads back a previously written journal so the REPL can
// resume mid-conversation. The journal is the source of truth — anything
// not on disk is gone.
func LoadSession(home, id string) (*Session, error) {
	if home == "" {
		return nil, fmt.Errorf("coding: home dir required")
	}
	if id == "" {
		return nil, fmt.Errorf("coding: session id required")
	}
	path := filepath.Join(home, ".conduit", sessionsSubdir, id+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("coding: open session journal: %w", err)
	}
	defer f.Close()

	session := &Session{
		ID:      id,
		Path:    path,
		homeDir: home,
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var turn contracts.CodingTurn
		if err := json.Unmarshal(scanner.Bytes(), &turn); err != nil {
			return nil, fmt.Errorf("coding: decode turn: %w", err)
		}
		if session.StartedAt.IsZero() || turn.At.Before(session.StartedAt) {
			session.StartedAt = turn.At
		}
		session.applyTurnState(turn)
		session.Turns = append(session.Turns, turn)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("coding: scan session journal: %w", err)
	}
	return session, nil
}

// WorktreeEnterOptions controls a worktree_enter transition. Empty fields pick
// deterministic managed defaults under ~/.conduit/worktrees.
type WorktreeEnterOptions struct {
	RepositoryRoot string
	Path           string
	Branch         string
	BaseBranch     string
}

// WorktreeExitOptions controls a worktree_exit transition.
type WorktreeExitOptions struct {
	Keep bool
}

// WorktreeEnter creates a managed git worktree and switches the process cwd to
// it so subsequent relative tools operate inside the isolated branch.
func (s *Session) WorktreeEnter(ctx context.Context, opts WorktreeEnterOptions) (contracts.CodingWorktreeState, error) {
	if s == nil {
		return contracts.CodingWorktreeState{}, fmt.Errorf("coding: session required")
	}
	repo := opts.RepositoryRoot
	if repo == "" {
		repo = s.RepositoryRoot
	}
	if repo == "" {
		repo, _ = gitRepoRoot(s.CWD)
	}
	if repo == "" {
		return contracts.CodingWorktreeState{}, fmt.Errorf("coding: cwd is not inside a git repository")
	}
	base := opts.BaseBranch
	if base == "" {
		base = s.Branch
	}
	if base == "" {
		base, _ = gitBranch(repo)
	}
	branch := opts.Branch
	if branch == "" {
		branch = "conduit/" + sanitizeBranchPart(s.ID)
	}
	path := opts.Path
	if path == "" {
		path = filepath.Join(s.homeDir, ".conduit", "worktrees", sanitizePathPart(s.ID))
	}
	if err := os.MkdirAll(filepath.Dir(path), sessionDirPerm); err != nil {
		return contracts.CodingWorktreeState{}, fmt.Errorf("coding: create worktree parent: %w", err)
	}
	if parent, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		path = filepath.Join(parent, filepath.Base(path))
	}

	args := []string{"-C", repo, "worktree", "add", "-b", branch, path}
	if base != "" {
		args = append(args, base)
	}
	if _, err := runGit(ctx, args...); err != nil {
		return contracts.CodingWorktreeState{}, err
	}

	prev := s.CWD
	if err := os.Chdir(path); err != nil {
		return contracts.CodingWorktreeState{}, fmt.Errorf("coding: chdir worktree: %w", err)
	}
	ev := contracts.CodingWorktreeEvent{
		At:             time.Now().UTC(),
		Action:         "enter",
		RepositoryRoot: repo,
		Path:           path,
		Branch:         branch,
		BaseBranch:     base,
		PreviousCWD:    prev,
		Message:        "entered managed git worktree",
	}
	s.recordWorktreeEvent(ev, repo, path, branch, true)
	return s.WorktreeState(), nil
}

// WorktreeExit leaves the active worktree, optionally removing it from git.
func (s *Session) WorktreeExit(ctx context.Context, opts WorktreeExitOptions) (contracts.CodingWorktreeState, error) {
	if s == nil {
		return contracts.CodingWorktreeState{}, fmt.Errorf("coding: session required")
	}
	if !s.WorktreeActive || s.WorktreePath == "" {
		return contracts.CodingWorktreeState{}, fmt.Errorf("coding: no active worktree")
	}
	target := s.RepositoryRoot
	if target == "" {
		target = s.CWD
	}
	if err := os.Chdir(target); err != nil {
		return contracts.CodingWorktreeState{}, fmt.Errorf("coding: chdir repository: %w", err)
	}
	if !opts.Keep {
		if _, err := runGit(ctx, "-C", target, "worktree", "remove", "--force", s.WorktreePath); err != nil {
			return contracts.CodingWorktreeState{}, err
		}
	}
	ev := contracts.CodingWorktreeEvent{
		At:             time.Now().UTC(),
		Action:         "exit",
		RepositoryRoot: target,
		Path:           s.WorktreePath,
		Branch:         s.WorktreeBranch,
		Kept:           opts.Keep,
		Message:        "exited managed git worktree",
	}
	s.recordWorktreeEvent(ev, target, "", "", false)
	return s.WorktreeState(), nil
}

// WorktreeState returns a copy of the current worktree snapshot.
func (s *Session) WorktreeState() contracts.CodingWorktreeState {
	history := append([]contracts.CodingWorktreeEvent(nil), s.WorktreeHistory...)
	return contracts.CodingWorktreeState{
		SessionID:      s.ID,
		RepositoryRoot: s.RepositoryRoot,
		CWD:            s.CWD,
		Branch:         s.Branch,
		WorktreePath:   s.WorktreePath,
		WorktreeBranch: s.WorktreeBranch,
		WorktreeActive: s.WorktreeActive,
		History:        history,
	}
}

func (s *Session) applyTurnState(turn contracts.CodingTurn) {
	if turn.CWD != "" {
		s.CWD = turn.CWD
	}
	if turn.GitBranch != "" {
		s.Branch = turn.GitBranch
	}
	if turn.RepositoryRoot != "" {
		s.RepositoryRoot = turn.RepositoryRoot
	}
	if turn.WorktreePath != "" {
		s.WorktreePath = turn.WorktreePath
	}
	if turn.WorktreeBranch != "" {
		s.WorktreeBranch = turn.WorktreeBranch
	}
	s.WorktreeActive = turn.WorktreeActive
	if turn.WorktreeEvent != nil {
		s.WorktreeHistory = append(s.WorktreeHistory, *turn.WorktreeEvent)
		if turn.WorktreeEvent.Action == "exit" {
			s.WorktreePath = ""
			s.WorktreeBranch = ""
			s.WorktreeActive = false
		}
	}
}

func (s *Session) recordWorktreeEvent(ev contracts.CodingWorktreeEvent, repo, path, branch string, active bool) {
	s.RepositoryRoot = repo
	s.CWD = repo
	s.WorktreePath = path
	s.WorktreeBranch = branch
	s.WorktreeActive = active
	if active {
		s.CWD = path
		s.Branch = branch
	} else if repo != "" {
		if b, err := gitBranch(repo); err == nil {
			s.Branch = b
		}
	}
	s.WorktreeHistory = append(s.WorktreeHistory, ev)
	_, _ = s.Append(contracts.CodingTurn{Role: "system", Content: ev.Message, WorktreeEvent: &ev})
}

func gitContext(cwd string) (string, string) {
	repo, _ := gitRepoRoot(cwd)
	branch, _ := gitBranch(cwd)
	return repo, branch
}

func gitRepoRoot(cwd string) (string, error) {
	if cwd == "" {
		return "", fmt.Errorf("empty cwd")
	}
	out, err := runGit(context.Background(), "-C", cwd, "rev-parse", "--show-toplevel")
	return strings.TrimSpace(out), err
}

func gitBranch(cwd string) (string, error) {
	if cwd == "" {
		return "", fmt.Errorf("empty cwd")
	}
	out, err := runGit(context.Background(), "-C", cwd, "branch", "--show-current")
	return strings.TrimSpace(out), err
}

func runGit(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(buf.String()))
	}
	return buf.String(), nil
}

func sanitizeBranchPart(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '/' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-/")
}

func sanitizePathPart(s string) string {
	return strings.ReplaceAll(sanitizeBranchPart(s), "/", "-")
}

func newSessionID() (string, error) {
	buf := make([]byte, 3)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("coding: random session id: %w", err)
	}
	return fmt.Sprintf("code-%d-%s", time.Now().UTC().Unix(), hex.EncodeToString(buf)), nil
}

func newSnapshotID(t time.Time) (contracts.CodingSnapshotID, error) {
	buf := make([]byte, 3)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("coding: random snapshot id: %w", err)
	}
	return contracts.CodingSnapshotID(fmt.Sprintf("snap-%s-%s", t.UTC().Format(time.RFC3339), hex.EncodeToString(buf))), nil
}
