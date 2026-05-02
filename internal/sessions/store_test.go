package sessions

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "sessions")
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store
}

func TestStoreCreateAndAppendRoundTrip(t *testing.T) {
	store := newTestStore(t)
	sess, err := store.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	turns := []Turn{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi", Model: "claude-sonnet-4-6"},
	}
	var lastID string
	for i := range turns {
		turns[i].ParentID = lastID
		written, err := store.Append(sess, turns[i])
		if err != nil {
			t.Fatalf("Append: %v", err)
		}
		if written.ID == "" {
			t.Fatalf("Append returned empty turn id")
		}
		if written.SessionID != sess.ID {
			t.Errorf("session id stamp: got %q want %q", written.SessionID, sess.ID)
		}
		lastID = written.ID
	}

	loaded, err := store.Load(sess.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Turns) != len(turns) {
		t.Fatalf("turn count: got %d want %d", len(loaded.Turns), len(turns))
	}
	if loaded.Turns[1].ParentID != loaded.Turns[0].ID {
		t.Errorf("parent link broken: got %q want %q", loaded.Turns[1].ParentID, loaded.Turns[0].ID)
	}
	data, err := os.ReadFile(sess.Path)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatalf("journal not JSONL-terminated: %q", string(data))
	}
}

func TestStoreListSortsByUpdatedAtDesc(t *testing.T) {
	store := newTestStore(t)
	older, _ := store.Create()
	if _, err := store.Append(older, Turn{Role: "user", Content: "older"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	newer, _ := store.Create()
	if _, err := store.Append(newer, Turn{Role: "user", Content: "newer"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	infos, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("List len: got %d want 2", len(infos))
	}
	if infos[0].UpdatedAt.Before(infos[1].UpdatedAt) {
		t.Errorf("expected newest first; got %v then %v", infos[0].UpdatedAt, infos[1].UpdatedAt)
	}
	if infos[0].Title == "" {
		t.Errorf("expected title from first user turn, got empty")
	}
}

func TestStoreFindTurn(t *testing.T) {
	store := newTestStore(t)
	sess, _ := store.Create()
	target, err := store.Append(sess, Turn{Role: "user", Content: "find me"})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, gotTurn, err := store.FindTurn(target.ID)
	if err != nil {
		t.Fatalf("FindTurn: %v", err)
	}
	if got.ID != sess.ID {
		t.Errorf("session id: got %q want %q", got.ID, sess.ID)
	}
	if gotTurn.ID != target.ID {
		t.Errorf("turn id: got %q want %q", gotTurn.ID, target.ID)
	}
	if _, _, err := store.FindTurn("no-such-turn"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing turn should wrap ErrNotExist, got %v", err)
	}
}

func TestStoreForkPreservesPrefix(t *testing.T) {
	store := newTestStore(t)
	sess, _ := store.Create()
	a, _ := store.Append(sess, Turn{Role: "user", Content: "one"})
	b, _ := store.Append(sess, Turn{Role: "assistant", Content: "two", ParentID: a.ID})
	c, _ := store.Append(sess, Turn{Role: "user", Content: "three", ParentID: b.ID})
	_ = c // not in fork prefix

	forked, err := store.Fork(sess.ID, b.ID)
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if forked.ID == sess.ID {
		t.Fatalf("fork must produce a new session id")
	}
	if forked.ForkParentID != b.ID {
		t.Errorf("ForkParentID: got %q want %q", forked.ForkParentID, b.ID)
	}
	if len(forked.Turns) != 2 {
		t.Fatalf("forked turn count: got %d want 2", len(forked.Turns))
	}
	if forked.Turns[0].ParentID != b.ID {
		t.Errorf("forked root ParentID should reference source turn id; got %q", forked.Turns[0].ParentID)
	}
	if forked.Turns[0].Role != "user" || forked.Turns[1].Role != "assistant" {
		t.Errorf("forked roles: got %q,%q", forked.Turns[0].Role, forked.Turns[1].Role)
	}

	// Reload from disk to make sure ForkParentID survives a round trip.
	reloaded, err := store.Load(forked.ID)
	if err != nil {
		t.Fatalf("Load forked: %v", err)
	}
	if reloaded.ForkParentID != b.ID {
		t.Errorf("reloaded ForkParentID: got %q want %q", reloaded.ForkParentID, b.ID)
	}
}

type stubResponder struct {
	last []Turn
	out  string
}

func (s *stubResponder) Respond(history []Turn, model string, params map[string]string) (string, error) {
	s.last = history
	if s.out == "" {
		return "stub response from " + model, nil
	}
	return s.out, nil
}

func TestStoreReplayCreatesNewBranch(t *testing.T) {
	store := newTestStore(t)
	sess, _ := store.Create()
	a, _ := store.Append(sess, Turn{Role: "user", Content: "ask"})
	b, _ := store.Append(sess, Turn{Role: "assistant", Content: "first answer", ParentID: a.ID})

	resp := &stubResponder{out: "replayed answer"}
	forked, newTurn, err := store.Replay(sess.ID, b.ID, ReplayOptions{
		Model:     "claude-opus-4-6",
		Responder: resp,
	})
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if forked.ID == sess.ID {
		t.Fatal("replay must create a new session")
	}
	if newTurn.Role != "assistant" || newTurn.Content != "replayed answer" {
		t.Errorf("new turn role/content: got %q/%q", newTurn.Role, newTurn.Content)
	}
	if newTurn.Model != "claude-opus-4-6" {
		t.Errorf("new turn model not propagated, got %q", newTurn.Model)
	}
	if len(resp.last) == 0 {
		t.Fatal("responder received empty history")
	}
}

func TestStoreReplayRequiresResponder(t *testing.T) {
	store := newTestStore(t)
	sess, _ := store.Create()
	a, _ := store.Append(sess, Turn{Role: "user", Content: "ask"})
	if _, _, err := store.Replay(sess.ID, a.ID, ReplayOptions{Model: "x"}); err == nil {
		t.Fatal("expected error when Responder is nil")
	}
}

func TestNewStoreDefaultDirCreated(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	store, err := NewStore("")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if !strings.HasPrefix(store.Dir(), root) {
		t.Errorf("default dir should sit under HOME (%s); got %s", root, store.Dir())
	}
	if _, err := os.Stat(store.Dir()); err != nil {
		t.Fatalf("default dir not created: %v", err)
	}
}
