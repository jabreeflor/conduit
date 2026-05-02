package coding

import (
	"os"
	"path/filepath"
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
