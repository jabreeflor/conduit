package coding

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/replytags"
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

// chunkStreamer emits a fixed sequence of deltas so we can test that the
// REPL's reply-tag parsing tolerates tags split across streamed chunks.
type chunkStreamer struct {
	chunks []string
	finish string
}

func (c *chunkStreamer) Stream(_ context.Context, _ string, onDelta func(string)) (string, string, error) {
	var sb strings.Builder
	for _, ch := range c.chunks {
		sb.WriteString(ch)
		if onDelta != nil {
			onDelta(ch)
		}
	}
	finish := c.finish
	if finish == "" {
		finish = "stop"
	}
	return sb.String(), finish, nil
}

func TestREPL_TagSinkFiltersTagsFromOutput(t *testing.T) {
	home := t.TempDir()
	session, err := NewSession(home)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	// Tag split across multiple chunks to also exercise streaming tolerance.
	streamer := &chunkStreamer{chunks: []string{
		"answer: 42 ", "[", "[voice: ", "say it]] done",
	}}

	var events []replytags.Event
	out := &bytes.Buffer{}
	repl := &REPL{
		Session:  session,
		Streamer: streamer,
		In:       bytes.NewBufferString("hi\n"),
		Out:      out,
		TagSink:  func(ev replytags.Event) { events = append(events, ev) },
	}
	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(out.String(), "answer: 42  done") {
		t.Errorf("output missing filtered text; got %q", out.String())
	}
	if strings.Contains(out.String(), "[[voice") {
		t.Errorf("output leaked tag bytes; got %q", out.String())
	}
	if len(events) != 1 || events[0].Kind != replytags.KindVoice ||
		events[0].Body != "say it" {
		t.Errorf("events = %+v, want one voice event with body \"say it\"", events)
	}

	// Session journal must preserve the raw, unfiltered assistant turn so
	// replays can re-dispatch tags downstream.
	if len(session.Turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(session.Turns))
	}
	if !strings.Contains(session.Turns[1].Content, "[[voice: say it]]") {
		t.Errorf("assistant turn lost tag bytes: %q", session.Turns[1].Content)
	}
}

func TestREPL_TagSinkSilentSuppressesOutput(t *testing.T) {
	home := t.TempDir()
	session, err := NewSession(home)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	streamer := &chunkStreamer{chunks: []string{"[[silent]]hidden trailing text"}}
	out := &bytes.Buffer{}
	var events []replytags.Event
	repl := &REPL{
		Session:  session,
		Streamer: streamer,
		In:       bytes.NewBufferString("hi\n"),
		Out:      out,
		TagSink:  func(ev replytags.Event) { events = append(events, ev) },
	}
	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(out.String(), "hidden trailing text") {
		t.Errorf("expected post-[[silent]] text suppressed, got %q", out.String())
	}
	if len(events) != 1 || events[0].Kind != replytags.KindSilent {
		t.Errorf("events = %+v, want one silent event", events)
	}
}

func TestREPL_NoTagSinkPreservesLegacyPassthrough(t *testing.T) {
	// Regression guard: when TagSink is nil, the REPL must not parse or
	// rewrite assistant output. Tag bytes are written verbatim.
	home := t.TempDir()
	session, err := NewSession(home)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	streamer := &chunkStreamer{chunks: []string{"plain [[voice: hi]] text"}}
	out := &bytes.Buffer{}
	repl := &REPL{
		Session:  session,
		Streamer: streamer,
		In:       bytes.NewBufferString("hi\n"),
		Out:      out,
	}
	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "[[voice: hi]]") {
		t.Errorf("expected legacy passthrough; got %q", out.String())
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
