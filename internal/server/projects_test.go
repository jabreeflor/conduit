package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeJournal joins each turn (encoded as JSON) with newlines and
// drops the result at sessionsDir/{id}.jsonl. Returns the resulting
// path so callers can chmod or otherwise massage it.
func writeJournal(t *testing.T, sessionsDir, id string, turns []map[string]any) string {
	t.Helper()
	var b strings.Builder
	for _, turn := range turns {
		raw, err := json.Marshal(turn)
		if err != nil {
			t.Fatalf("marshal turn: %v", err)
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	path := filepath.Join(sessionsDir, id+".jsonl")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestHandleProjects(t *testing.T) {
	dir := t.TempDir()

	// Two sessions sharing /repo/conduit (one older, one newer) — must group.
	writeJournal(t, dir, "code-1-aaa", []map[string]any{
		{
			"Index": 0, "At": "2026-05-06T10:00:00Z",
			"Role": "user", "Content": "first conduit chat — older session",
			"RepositoryRoot": "/repo/conduit", "GitBranch": "main",
		},
		{
			"Index": 1, "At": "2026-05-06T10:01:00Z",
			"Role": "assistant", "Content": "ok",
			"RepositoryRoot": "/repo/conduit", "GitBranch": "main",
		},
	})
	writeJournal(t, dir, "code-2-bbb", []map[string]any{
		{
			"Index": 0, "At": "2026-05-06T12:00:00Z",
			"Role":           "user",
			"Content":        strings.Repeat("x", 200), // long → must truncate
			"RepositoryRoot": "/repo/conduit", "GitBranch": "feat/projects-shell",
		},
		{
			"Index": 1, "At": "2026-05-06T12:00:30Z",
			"Role": "assistant", "Content": "yes",
			"RepositoryRoot": "/repo/conduit", "GitBranch": "feat/projects-shell",
		},
		{
			"Index": 2, "At": "2026-05-06T12:01:00Z",
			"Role": "user", "Content": "follow-up",
			"RepositoryRoot": "/repo/conduit", "GitBranch": "feat/projects-shell",
		},
	})

	// One session in a different repo — must be its own project,
	// and because its lastActivity (2026-05-06T13:00) > conduit's
	// (12:01), it must sort first.
	writeJournal(t, dir, "code-3-ccc", []map[string]any{
		{
			"Index": 0, "At": "2026-05-06T13:00:00Z",
			"Role": "user", "Content": "other repo prompt",
			"RepositoryRoot": "/repo/other", "GitBranch": "main",
		},
	})

	// One session with no RepositoryRoot — must land in orphans.
	writeJournal(t, dir, "code-4-ddd", []map[string]any{
		{
			"Index": 0, "At": "2026-05-06T11:00:00Z",
			"Role": "user", "Content": "stray prompt",
			// no RepositoryRoot
		},
	})

	s := New(Config{SessionsDir: dir, HomeDir: "/home/test"})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/projects", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}

	var got projectsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\nbody: %s", err, rr.Body.String())
	}

	// Two projects, in lastActivity-desc order: /repo/other (13:00),
	// /repo/conduit (12:01).
	if len(got.Projects) != 2 {
		t.Fatalf("projects: got %d want 2 (%+v)", len(got.Projects), got.Projects)
	}
	if got.Projects[0].AbsolutePath != "/repo/other" {
		t.Errorf("project[0] sort: got %q want /repo/other", got.Projects[0].AbsolutePath)
	}
	if got.Projects[1].AbsolutePath != "/repo/conduit" {
		t.Errorf("project[1] sort: got %q want /repo/conduit", got.Projects[1].AbsolutePath)
	}

	// Conduit project: 2 chats, branch from most-recent session,
	// chats sorted createdAt desc.
	conduit := got.Projects[1]
	if conduit.Name != "conduit" {
		t.Errorf("name: got %q want conduit", conduit.Name)
	}
	if conduit.SessionCount != 2 {
		t.Errorf("sessionCount: got %d want 2", conduit.SessionCount)
	}
	if conduit.Branch != "feat/projects-shell" {
		t.Errorf("branch: got %q want feat/projects-shell", conduit.Branch)
	}
	if conduit.LastActivity != "2026-05-06T12:01:00Z" {
		t.Errorf("lastActivity: got %q want 2026-05-06T12:01:00Z", conduit.LastActivity)
	}
	if len(conduit.Chats) != 2 {
		t.Fatalf("chats: got %d want 2", len(conduit.Chats))
	}
	// Newest chat first.
	if conduit.Chats[0].ID != "code-2-bbb" {
		t.Errorf("chat[0]: got %q want code-2-bbb", conduit.Chats[0].ID)
	}
	if conduit.Chats[1].ID != "code-1-aaa" {
		t.Errorf("chat[1]: got %q want code-1-aaa", conduit.Chats[1].ID)
	}
	// Title truncation: 200 x's clamps to 77 + "..." = 80 runes.
	wantPrefix := strings.Repeat("x", 77) + "..."
	if conduit.Chats[0].Title != wantPrefix {
		t.Errorf("title truncation: got %q (len=%d) want %q", conduit.Chats[0].Title, len([]rune(conduit.Chats[0].Title)), wantPrefix)
	}
	// Older chat title pulled from first user content verbatim.
	if conduit.Chats[1].Title != "first conduit chat — older session" {
		t.Errorf("title: got %q want %q", conduit.Chats[1].Title, "first conduit chat — older session")
	}
	if conduit.Chats[1].TurnCount != 2 {
		t.Errorf("turnCount: got %d want 2", conduit.Chats[1].TurnCount)
	}
	if conduit.Chats[1].CreatedAt != "2026-05-06T10:00:00Z" {
		t.Errorf("createdAt: got %q want 2026-05-06T10:00:00Z", conduit.Chats[1].CreatedAt)
	}

	// Stable ID matches the helper.
	if got, want := conduit.ID, projectIDFromPath("/repo/conduit"); got != want {
		t.Errorf("project id: got %q want %q", got, want)
	}
	if len(conduit.ID) != 16 {
		t.Errorf("project id length: got %d want 16", len(conduit.ID))
	}

	// Other project: 1 chat, lastActivity 13:00.
	other := got.Projects[0]
	if other.SessionCount != 1 || len(other.Chats) != 1 {
		t.Errorf("other project chats: %+v", other)
	}
	if other.LastActivity != "2026-05-06T13:00:00Z" {
		t.Errorf("other lastActivity: got %q", other.LastActivity)
	}

	// Orphans: 1 entry, the stray prompt.
	if len(got.Orphans) != 1 {
		t.Fatalf("orphans: got %d want 1 (%+v)", len(got.Orphans), got.Orphans)
	}
	if got.Orphans[0].ID != "code-4-ddd" {
		t.Errorf("orphan id: got %q want code-4-ddd", got.Orphans[0].ID)
	}
	if got.Orphans[0].Title != "stray prompt" {
		t.Errorf("orphan title: got %q want %q", got.Orphans[0].Title, "stray prompt")
	}
}

func TestHandleProjectsMissingDir(t *testing.T) {
	s := New(Config{SessionsDir: filepath.Join(t.TempDir(), "does-not-exist")})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/projects", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	var got projectsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Projects) != 0 || len(got.Orphans) != 0 {
		t.Errorf("missing-dir response: got %+v want empty projects+orphans", got)
	}
}

func TestHomeRelative(t *testing.T) {
	cases := []struct {
		abs, home, want string
	}{
		{"/Users/me/code/conduit", "/Users/me", "~/code/conduit"},
		{"/Users/me", "/Users/me", "~"},
		{"/var/tmp/x", "/Users/me", "/var/tmp/x"},
		{"/Users/me/code", "", "/Users/me/code"},
	}
	for _, c := range cases {
		if got := homeRelative(c.abs, c.home); got != c.want {
			t.Errorf("homeRelative(%q,%q): got %q want %q", c.abs, c.home, got, c.want)
		}
	}
}

// Sanity: ensure project ID is deterministic so the GUI can use it as a key.
func TestProjectIDStable(t *testing.T) {
	a := projectIDFromPath("/repo/conduit")
	b := projectIDFromPath("/repo/conduit")
	if a != b {
		t.Errorf("non-deterministic id: %s vs %s", a, b)
	}
	if a == projectIDFromPath("/repo/other") {
		t.Errorf("id collision: %s", a)
	}
	// Hash prefix surfaces as 16 hex chars.
	if len(a) != 16 {
		t.Errorf("id length: got %d want 16 (id=%q)", len(a), a)
	}
	_ = fmt.Sprintf // keep fmt import if future asserts need it
}
