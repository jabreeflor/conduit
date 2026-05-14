// Package e2e holds Conduit's end-to-end test suite: full user journeys
// that exercise multiple subsystems through their public entry points
// (CLI, HTTP/WS, store APIs) with all side-effecting dependencies
// (filesystem, LLM provider) faked or sandboxed under t.TempDir().
//
// Each journey lives in its own file:
//
//	coding_journey_test.go   - conduit code REPL + sessions + tools + hooks
//	server_journey_test.go   - conduit serve + HTTP + WebSocket agent
//	sandbox_journey_test.go  - sandbox create/snapshot/rollback/clone/destroy
//	sessions_journey_test.go - sessions create/fork/replay/find lifecycle
//
// The shared fakes live here: ScriptedStreamer (a programmable
// coding.Streamer that emits a fixed sequence of deltas), conduitHome
// (a $HOME-shaped tempdir scaffold), and a few JSONL/fixture helpers.
package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/coding"
)

// ScriptedStreamer is a fully deterministic coding.Streamer for tests.
//
// Each Stream call pops the next Reply from Replies. Reply.Deltas are
// emitted one by one via onDelta; Reply.Full is returned as the full
// string (empty means concatenation of deltas). Reply.FinishReason is
// the finish reason ("stop", "length", "max_tokens"). Reply.Err, if
// non-nil, is returned instead of streaming — useful for testing error
// paths.
//
// The streamer is goroutine-safe: tests can hand the same streamer to
// multiple REPLs or WebSocket handlers and still get deterministic
// per-call output.
type ScriptedStreamer struct {
	mu      sync.Mutex
	Replies []Reply
	calls   []ScriptedCall
}

// Reply is one scripted Stream call.
type Reply struct {
	Deltas       []string
	Full         string
	FinishReason string
	Err          error
}

// ScriptedCall captures the inputs the REPL passed for one Stream call,
// so tests can assert on prompts in order.
type ScriptedCall struct {
	Prompt string
	At     time.Time
}

// NewScripted builds a streamer that emits the given replies in order.
func NewScripted(replies ...Reply) *ScriptedStreamer {
	return &ScriptedStreamer{Replies: append([]Reply(nil), replies...)}
}

// Calls returns a snapshot of every Stream invocation so far.
func (s *ScriptedStreamer) Calls() []ScriptedCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ScriptedCall, len(s.calls))
	copy(out, s.calls)
	return out
}

// Stream implements coding.Streamer.
func (s *ScriptedStreamer) Stream(ctx context.Context, prompt string, onDelta func(string)) (string, string, error) {
	s.mu.Lock()
	s.calls = append(s.calls, ScriptedCall{Prompt: prompt, At: time.Now()})
	if len(s.Replies) == 0 {
		s.mu.Unlock()
		// Default trailing reply so misconfigured tests still terminate
		// quickly with a recognisable diagnostic.
		return "scripted: no more replies", "stop", nil
	}
	r := s.Replies[0]
	s.Replies = s.Replies[1:]
	s.mu.Unlock()

	if r.Err != nil {
		return "", "", r.Err
	}
	for _, d := range r.Deltas {
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		if onDelta != nil {
			onDelta(d)
		}
	}
	full := r.Full
	if full == "" {
		full = strings.Join(r.Deltas, "")
	}
	finish := r.FinishReason
	if finish == "" {
		finish = "stop"
	}
	return full, finish, nil
}

// _ enforces interface conformance at compile time.
var _ coding.Streamer = (*ScriptedStreamer)(nil)

// conduitHome is a tempdir scaffold mimicking the layout the rest of
// Conduit expects: $HOME/.conduit/{coding-sessions,sessions} plus
// SOUL.md / USER.md memory files. Tests use this to keep journeys
// isolated from the host's real ~/.conduit.
type conduitHome struct {
	Root            string
	ConduitDir      string
	CodingSessions  string
	Sessions        string
	SkillsWorkspace string
	SkillsPersonal  string
	WorkspaceDir    string
}

// newHome materialises a fresh conduit-shaped home rooted at t.TempDir().
// All directories exist; SOUL.md and USER.md are pre-seeded with stable
// content so /api/memory has something to return.
func newHome(t *testing.T) *conduitHome {
	t.Helper()
	root := t.TempDir()
	h := &conduitHome{
		Root:            root,
		ConduitDir:      filepath.Join(root, ".conduit"),
		CodingSessions:  filepath.Join(root, ".conduit", "coding-sessions"),
		Sessions:        filepath.Join(root, ".conduit", "sessions"),
		SkillsWorkspace: filepath.Join(root, "workspace", ".conduit", "skills"),
		SkillsPersonal:  filepath.Join(root, ".conduit", "skills"),
		WorkspaceDir:    filepath.Join(root, "workspace"),
	}
	for _, d := range []string{h.ConduitDir, h.CodingSessions, h.Sessions, h.SkillsWorkspace, h.SkillsPersonal, h.WorkspaceDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := os.WriteFile(filepath.Join(h.ConduitDir, "SOUL.md"), []byte("# SOUL\n\nYou are Conduit.\n"), 0o600); err != nil {
		t.Fatalf("seed SOUL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(h.ConduitDir, "USER.md"), []byte("# USER\n\nThe user prefers concise replies.\n"), 0o600); err != nil {
		t.Fatalf("seed USER.md: %v", err)
	}
	return h
}

// readJSONL reads a JSONL file and decodes each line into a fresh map.
// It returns the decoded entries in file order. Empty lines are skipped.
// Failures are reported via t.Fatalf so callers can write straight-line
// assertions.
func readJSONL(t *testing.T, path string) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var out []map[string]any
	for i, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("decode %s line %d: %v\n%s", path, i+1, err, line)
		}
		out = append(out, m)
	}
	return out
}

// waitFor polls fn every 5 ms until it returns true or the timeout
// expires. Returns true on success; tests should t.Fatal on false.
// Use this instead of time.Sleep when waiting for an async event from
// the streamer or WebSocket emitter.
func waitFor(timeout time.Duration, fn func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fn()
}

// errFakeProvider is the sentinel error ScriptedStreamer uses when a
// test wants to assert that the REPL surfaces provider failures cleanly.
var errFakeProvider = errors.New("fake-provider: synthetic failure")

// firstUserContent returns the first user-role content from a JSONL
// turn dump produced by readJSONL. Empty string when none is present.
func firstUserContent(entries []map[string]any) string {
	for _, e := range entries {
		if role, _ := e["role"].(string); role == "user" {
			if c, ok := e["content"].(string); ok {
				return c
			}
		}
	}
	return ""
}

// mustWrite writes data to path, creating parents as needed, t.Fatal on
// failure. Returns the absolute path for chaining.
func mustWrite(t *testing.T, path string, data string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// debugDump prints a labelled, indented JSON snapshot of v. Use in tests
// only when a failure is otherwise opaque; never leave behind in passing
// runs.
func debugDump(t *testing.T, label string, v any) {
	t.Helper()
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Printf("--- %s ---\n%s\n", label, b)
}

// noopT is a stub used by helpers that may run outside of tests. Kept
// unexported so it never leaks.
type noopT struct{}

func (noopT) Helper()                           {}
func (noopT) Fatalf(format string, args ...any) {}
