package coding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type execFakeStreamer struct {
	out    string
	finish string
	err    error
}

func (f execFakeStreamer) Stream(_ context.Context, _ string, _ func(string)) (string, string, error) {
	return f.out, f.finish, f.err
}

func TestRunExecHappyPath(t *testing.T) {
	home := t.TempDir()
	res, err := RunExec(context.Background(), ExecOptions{
		Prompt:         "say hi",
		HomeDir:        home,
		Streamer:       execFakeStreamer{out: "hi back", finish: "stop"},
		MaxInputTokens: 100,
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if res.Output != "hi back" || res.FinishReason != "stop" || res.Turns != 1 {
		t.Errorf("bad result: %+v", res)
	}
	if res.SessionID == "" {
		t.Errorf("expected session id")
	}
	if res.SchemaVersion != "conduit-exec/1" {
		t.Errorf("schema bumped without test update: %s", res.SchemaVersion)
	}
	// Session journal should exist on disk.
	dir := filepath.Join(home, ".conduit", sessionsSubdir)
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("expected one journal file, got %d", len(entries))
	}
}

func TestRunExecNoSessionSkipsJournal(t *testing.T) {
	home := t.TempDir()
	_, err := RunExec(context.Background(), ExecOptions{
		Prompt:    "ephemeral",
		HomeDir:   home,
		NoSession: true,
		Streamer:  execFakeStreamer{out: "ok", finish: "stop"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".conduit", sessionsSubdir)); !os.IsNotExist(err) {
		t.Errorf("expected no sessions dir, got: %v", err)
	}
}

func TestRunExecPromptFile(t *testing.T) {
	home := t.TempDir()
	pf := filepath.Join(t.TempDir(), "p.txt")
	if err := os.WriteFile(pf, []byte("from file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := RunExec(context.Background(), ExecOptions{
		PromptFile: pf,
		HomeDir:    home,
		Streamer:   execFakeStreamer{out: "ack", finish: "stop"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Prompt != "from file" {
		t.Errorf("prompt should be trimmed of trailing newline: %q", res.Prompt)
	}
}

func TestRunExecStdin(t *testing.T) {
	home := t.TempDir()
	res, err := RunExec(context.Background(), ExecOptions{
		HomeDir:  home,
		Stdin:    strings.NewReader("piped prompt\n"),
		Streamer: execFakeStreamer{out: "ok", finish: "stop"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Prompt != "piped prompt" {
		t.Errorf("got %q", res.Prompt)
	}
}

func TestRunExecRequiresPrompt(t *testing.T) {
	home := t.TempDir()
	_, err := RunExec(context.Background(), ExecOptions{
		HomeDir:  home,
		Streamer: execFakeStreamer{out: "ok", finish: "stop"},
	})
	if err == nil {
		t.Errorf("expected error when no prompt source provided")
	}
}

func TestRunExecStreamerError(t *testing.T) {
	home := t.TempDir()
	res, err := RunExec(context.Background(), ExecOptions{
		Prompt:   "x",
		HomeDir:  home,
		Streamer: execFakeStreamer{err: errors.New("provider down")},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if res.ExitCode != 1 {
		t.Errorf("expected exit 1, got %d", res.ExitCode)
	}
}

func TestRenderExecResultJSONAndText(t *testing.T) {
	res := ExecResult{
		SchemaVersion: "conduit-exec/1",
		SessionID:     "code-123",
		Output:        "hello",
		FinishReason:  "stop",
		Turns:         1,
	}
	var buf bytes.Buffer
	if err := RenderExecResult(&buf, res, "json"); err != nil {
		t.Fatal(err)
	}
	var got ExecResult
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, buf.String())
	}
	if got.SessionID != "code-123" {
		t.Errorf("round-trip lost session id: %+v", got)
	}

	buf.Reset()
	if err := RenderExecResult(&buf, res, "text"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "hello") || !strings.Contains(buf.String(), "code-123") {
		t.Errorf("text render missing content:\n%s", buf.String())
	}
}

func TestParseExecArgsValidation(t *testing.T) {
	if _, err := ParseExecArgs([]string{"--prompt", "a", "--prompt-file", "b"}); err == nil {
		t.Errorf("expected mutually-exclusive error")
	}
	if _, err := ParseExecArgs([]string{"--format", "yaml"}); err == nil {
		t.Errorf("expected unknown format error")
	}
	opts, err := ParseExecArgs([]string{"--prompt", "hi", "--format", "json", "--allow-write"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Prompt != "hi" || opts.Format != "json" || !opts.AllowWrite {
		t.Errorf("bad parse: %+v", opts)
	}
}
