package eval

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeResponder struct {
	output string
}

func (f fakeResponder) Replay(_ context.Context, req ReplayRequest) (ReplayResponse, error) {
	return ReplayResponse{
		Output:       f.output + req.Prompt,
		Provider:     "fake",
		InputTokens:  3,
		OutputTokens: 5,
	}, nil
}

func TestReplaySessionWritesResultsAndDiff(t *testing.T) {
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "sessions")
	resultsDir := filepath.Join(dir, "results")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(sessionDir, "abc123.jsonl")
	writeSession(t, sessionPath, []string{
		`{"role":"user","content":"hello"}`,
		`{"role":"assistant","content":"hi","turn_id":"t1","model":"claude-sonnet-4-6"}`,
	})

	summary, err := ReplaySession(context.Background(), ReplayOptions{
		SessionID:   "abc123",
		Model:       "gpt-4o",
		Diff:        true,
		SessionsDir: sessionDir,
		ResultsDir:  resultsDir,
		Responder:   fakeResponder{output: "replayed: "},
		Now:         func() time.Time { return time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Turns != 1 || summary.Matches != 0 {
		t.Fatalf("summary = %+v, want one non-matching turn", summary)
	}
	if len(summary.Diffs) != 1 || !strings.Contains(summary.Diffs[0], "-hi") || !strings.Contains(summary.Diffs[0], "+replayed: hello") {
		t.Fatalf("diff = %#v", summary.Diffs)
	}

	data, err := os.ReadFile(summary.ResultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result ReplayResult
	if err := json.Unmarshal(bytesTrimNewline(data), &result); err != nil {
		t.Fatal(err)
	}
	if result.Provider != "fake" || result.Prompt != "hello" || result.ActualOutput != "replayed: hello" {
		t.Fatalf("result = %+v", result)
	}
}

func TestReadSessionAcceptsCommonEventShapes(t *testing.T) {
	msgs, err := readSessionJSONL(strings.NewReader(strings.Join([]string{
		`{"type":"user_message","message":"question"}`,
		`{"type":"assistant_message","output":"answer"}`,
		`{"type":"tool_call","content":"ignored"}`,
	}, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want 2", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Fatalf("msgs = %+v", msgs)
	}
}

func TestResolveSessionPathAcceptsDirectPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	writeSession(t, path, []string{`{"role":"user","content":"hello"}`})

	got, err := ResolveSessionPath(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("path = %q, want %q", got, path)
	}
}

func TestSnapshotResponderMarksMatch(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "abc.jsonl")
	writeSession(t, sessionPath, []string{
		`{"role":"user","content":"hello"}`,
		`{"role":"assistant","content":"hi"}`,
	})

	summary, err := ReplaySession(context.Background(), ReplayOptions{
		SessionID:  sessionPath,
		Model:      "gpt-4o",
		ResultsDir: filepath.Join(dir, "results"),
		Now:        func() time.Time { return time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !summary.SnapshotMode || summary.Turns != 1 || summary.Matches != 1 {
		t.Fatalf("summary = %+v, want snapshot matching one turn", summary)
	}
}

func writeSession(t *testing.T, path string, lines []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func bytesTrimNewline(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
