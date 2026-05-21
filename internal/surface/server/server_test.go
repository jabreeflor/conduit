package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHandleInfo(t *testing.T) {
	s := New(Config{Provider: "codex", Model: "gpt-5.5", Version: "test"})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/info", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	var got infoResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Provider != "codex" || got.Model != "gpt-5.5" || got.Version != "test" {
		t.Errorf("unexpected info: %+v", got)
	}
}

func TestHandleMemory(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".conduit"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".conduit", "SOUL.md"), []byte("soul-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := New(Config{HomeDir: home})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/memory", nil))
	var got memoryResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Soul != "soul-bytes" {
		t.Errorf("soul: got %q want %q", got.Soul, "soul-bytes")
	}
	if got.User != "" {
		t.Errorf("user: want empty for missing USER.md, got %q", got.User)
	}
}

func TestHandleMemorySaveRoundTrip(t *testing.T) {
	home := t.TempDir()
	s := New(Config{HomeDir: home})

	// POST writes both files even though .conduit doesn't exist yet.
	bodyIn := `{"soul":"# Soul\nbe precise","user":"# User\nprefers Go"}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/memory",
		strings.NewReader(bodyIn))
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("save status: got %d want 200 (body %s)", rr.Code, rr.Body.String())
	}

	// Files landed on disk with the expected content.
	soul, err := os.ReadFile(filepath.Join(home, ".conduit", "SOUL.md"))
	if err != nil {
		t.Fatalf("read SOUL.md: %v", err)
	}
	if string(soul) != "# Soul\nbe precise" {
		t.Errorf("SOUL.md: got %q", string(soul))
	}

	// GET reads them back identically.
	rr2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/api/memory", nil))
	var got memoryResponse
	if err := json.Unmarshal(rr2.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Soul != "# Soul\nbe precise" || got.User != "# User\nprefers Go" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestHandleMemoryMethodNotAllowed(t *testing.T) {
	s := New(Config{HomeDir: t.TempDir()})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/api/memory", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE status: got %d want 405", rr.Code)
	}
}

func TestHandleSessions(t *testing.T) {
	dir := t.TempDir()
	// Two journal files with predictable mtimes so order is verifiable.
	older := filepath.Join(dir, "code-1-aaa.jsonl")
	newer := filepath.Join(dir, "code-2-bbb.jsonl")
	if err := os.WriteFile(older, []byte(`{"role":"user","content":"first prompt"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte(`{"prompt":"second prompt"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	_ = os.Chtimes(older, now.Add(-2*time.Hour), now.Add(-2*time.Hour))
	_ = os.Chtimes(newer, now, now)

	s := New(Config{SessionsDir: dir})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/sessions", nil))
	var got []sessionInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("count: got %d want 2", len(got))
	}
	if got[0].ID != "code-2-bbb" {
		t.Errorf("order: got first=%q want code-2-bbb", got[0].ID)
	}
	if got[0].Summary != "second prompt" {
		t.Errorf("summary: got %q want %q", got[0].Summary, "second prompt")
	}
	if got[1].Summary != "first prompt" {
		t.Errorf("summary[1]: got %q", got[1].Summary)
	}
}

func TestHandleSessionsMissingDir(t *testing.T) {
	s := New(Config{SessionsDir: filepath.Join(t.TempDir(), "does-not-exist")})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/sessions", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	if got := rr.Body.String(); got != "[]\n" {
		t.Errorf("body: got %q want %q", got, "[]\n")
	}
}
