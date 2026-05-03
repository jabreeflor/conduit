package coding

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestNewSessionCreatesDir(t *testing.T) {
	home := t.TempDir()
	s, err := NewSession(home)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	dir := filepath.Join(home, ".conduit", sessionsSubdir)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat session dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("session path is not a directory")
	}
	if perm := info.Mode().Perm(); perm != sessionDirPerm {
		t.Errorf("dir perms: got %o want %o", perm, sessionDirPerm)
	}
	if s.ID == "" || s.Path == "" {
		t.Fatalf("missing session metadata: %+v", s)
	}
}

func TestNewSessionInDirRecordsGitContext(t *testing.T) {
	repo := initGitRepo(t)
	repo = realPath(t, repo)
	s, err := NewSessionInDir(t.TempDir(), repo)
	if err != nil {
		t.Fatalf("NewSessionInDir: %v", err)
	}
	if s.RepositoryRoot != repo {
		t.Fatalf("RepositoryRoot = %q, want %q", s.RepositoryRoot, repo)
	}
	if s.Branch != "main" {
		t.Fatalf("Branch = %q, want main", s.Branch)
	}
}

func TestWorktreeEnterExitPersistsAcrossLoad(t *testing.T) {
	home := t.TempDir()
	repo := realPath(t, initGitRepo(t))
	orig, _ := os.Getwd()
	defer os.Chdir(orig)

	s, err := NewSessionInDir(home, repo)
	if err != nil {
		t.Fatalf("NewSessionInDir: %v", err)
	}
	worktreePath := realParentPath(t, filepath.Join(home, "wt"))
	state, err := s.WorktreeEnter(context.Background(), WorktreeEnterOptions{
		Path:   worktreePath,
		Branch: "feature/worktree",
	})
	if err != nil {
		t.Fatalf("WorktreeEnter: %v", err)
	}
	if !state.WorktreeActive || state.CWD != worktreePath {
		t.Fatalf("enter state = %+v", state)
	}
	if cwd, _ := os.Getwd(); cwd != worktreePath {
		t.Fatalf("process cwd = %q, want %q", cwd, worktreePath)
	}

	loaded, err := LoadSession(home, s.ID)
	if err != nil {
		t.Fatalf("LoadSession after enter: %v", err)
	}
	if !loaded.WorktreeActive || loaded.WorktreePath != worktreePath || len(loaded.WorktreeHistory) != 1 {
		t.Fatalf("loaded enter state = %+v", loaded.WorktreeState())
	}

	state, err = s.WorktreeExit(context.Background(), WorktreeExitOptions{})
	if err != nil {
		t.Fatalf("WorktreeExit: %v", err)
	}
	if state.WorktreeActive || state.CWD != repo {
		t.Fatalf("exit state = %+v", state)
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree path still exists or stat failed: %v", err)
	}

	loaded, err = LoadSession(home, s.ID)
	if err != nil {
		t.Fatalf("LoadSession after exit: %v", err)
	}
	if loaded.WorktreeActive || loaded.WorktreePath != "" || len(loaded.WorktreeHistory) != 2 {
		t.Fatalf("loaded exit state = %+v", loaded.WorktreeState())
	}
}

func TestNewLocalSessionCreatesWorktree(t *testing.T) {
	home := t.TempDir()
	repo := realPath(t, initGitRepo(t))
	orig, _ := os.Getwd()
	defer os.Chdir(orig)

	s, err := NewLocalSession(context.Background(), home, repo, "local/session")
	if err != nil {
		t.Fatalf("NewLocalSession: %v", err)
	}
	if !s.WorktreeActive || s.WorktreeBranch != "local/session" {
		t.Fatalf("state = %+v", s.WorktreeState())
	}
	if _, err := os.Stat(s.WorktreePath); err != nil {
		t.Fatalf("stat worktree: %v", err)
	}
	_, _ = s.WorktreeExit(context.Background(), WorktreeExitOptions{})
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run(t, dir, "git", "init", "-b", "main")
	run(t, dir, "git", "config", "user.email", "test@example.com")
	run(t, dir, "git", "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", "README.md")
	run(t, dir, "git", "commit", "-m", "init")
	return dir
}

func realPath(t *testing.T, path string) string {
	t.Helper()
	out, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func realParentPath(t *testing.T, path string) string {
	t.Helper()
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(parent, filepath.Base(path))
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}

func TestSessionAppendWritesJSONL(t *testing.T) {
	home := t.TempDir()
	s, err := NewSession(home)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	id, err := s.Append(contracts.CodingTurn{Role: "user", Content: "hi"})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if id == "" {
		t.Fatal("Append returned empty snapshot id")
	}
	data, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatalf("journal not JSONL-terminated: %q", string(data))
	}
}

func TestLoadSessionRoundTrip(t *testing.T) {
	home := t.TempDir()
	s, err := NewSession(home)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	turns := []contracts.CodingTurn{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
		{Role: "tool", Content: "{}"},
	}
	for _, tr := range turns {
		if _, err := s.Append(tr); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	loaded, err := LoadSession(home, s.ID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if len(loaded.Turns) != len(turns) {
		t.Fatalf("turn count: got %d want %d", len(loaded.Turns), len(turns))
	}
	for i, tr := range turns {
		got := loaded.Turns[i]
		if got.Role != tr.Role || got.Content != tr.Content {
			t.Errorf("turn %d: got (%q,%q) want (%q,%q)", i, got.Role, got.Content, tr.Role, tr.Content)
		}
		if got.SnapshotID == "" {
			t.Errorf("turn %d missing snapshot id", i)
		}
	}
}
