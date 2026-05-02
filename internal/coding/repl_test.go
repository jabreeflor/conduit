package coding

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// fakeStreamer returns a configured sequence of finishReasons so tests can
// assert auto-continuation behavior. Each Stream call advances the cursor
// by one; the last entry repeats if the REPL keeps calling.
type fakeStreamer struct {
	calls    int
	reasons  []string
	prompts  []string
	response string
}

func (f *fakeStreamer) Stream(_ context.Context, prompt string, onDelta func(string)) (string, string, error) {
	idx := f.calls
	if idx >= len(f.reasons) {
		idx = len(f.reasons) - 1
	}
	reason := f.reasons[idx]
	f.calls++
	f.prompts = append(f.prompts, prompt)
	if onDelta != nil {
		onDelta(f.response)
	}
	return f.response, reason, nil
}

func TestREPLAutoContinuesOnLength(t *testing.T) {
	home := t.TempDir()
	session, err := NewSession(home)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	streamer := &fakeStreamer{
		reasons:  []string{"length", "length", "stop"},
		response: "chunk",
	}
	in := bytes.NewBufferString("hello\n")
	out := &bytes.Buffer{}

	repl := &REPL{
		Session:   session,
		Budget:    NewBudget(1_000_000),
		Streamer:  streamer,
		Continuer: DefaultContinuer{},
		In:        in,
		Out:       out,
	}
	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if streamer.calls != 3 {
		t.Errorf("expected 3 streamer calls (initial + 2 continues), got %d", streamer.calls)
	}
	if len(streamer.prompts) < 2 || streamer.prompts[1] != "continue" {
		t.Errorf("expected second prompt to be \"continue\", got %v", streamer.prompts)
	}
	// 1 user + 3 assistant turns persisted.
	if len(session.Turns) != 4 {
		t.Errorf("expected 4 turns, got %d", len(session.Turns))
	}
	if session.Turns[0].Role != "user" {
		t.Errorf("first turn role: %q", session.Turns[0].Role)
	}
	for _, tr := range session.Turns[1:] {
		if tr.Role != "assistant" {
			t.Errorf("expected assistant turn, got %q", tr.Role)
		}
	}
}

func TestREPLEndsOnStop(t *testing.T) {
	home := t.TempDir()
	session, err := NewSession(home)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	streamer := &fakeStreamer{reasons: []string{"stop"}, response: "ok"}
	repl := &REPL{
		Session:  session,
		Streamer: streamer,
		In:       bytes.NewBufferString("hi\n"),
		Out:      &bytes.Buffer{},
	}
	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if streamer.calls != 1 {
		t.Errorf("expected 1 call, got %d", streamer.calls)
	}
	if len(session.Turns) != 2 {
		t.Errorf("expected 2 turns (user+assistant), got %d", len(session.Turns))
	}
}

func TestREPLEOFExitsCleanly(t *testing.T) {
	home := t.TempDir()
	session, err := NewSession(home)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	repl := &REPL{
		Session:  session,
		Streamer: &fakeStreamer{reasons: []string{"stop"}, response: "x"},
		In:       strings.NewReader(""),
		Out:      &bytes.Buffer{},
	}
	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run on empty input: %v", err)
	}
}

func TestDefaultContinuer(t *testing.T) {
	c := DefaultContinuer{}
	for _, reason := range []string{"length", "max_tokens", "MAX_TOKENS", "Length"} {
		if !c.ShouldContinue(reason) {
			t.Errorf("expected continue on %q", reason)
		}
	}
	for _, reason := range []string{"stop", "end_turn", "", "tool_use"} {
		if c.ShouldContinue(reason) {
			t.Errorf("did not expect continue on %q", reason)
		}
	}
}
