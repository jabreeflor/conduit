package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/jabreeflor/conduit/internal/coding"
	"github.com/jabreeflor/conduit/internal/tools"
)

// scriptedStreamer is a coding.Streamer that replays a pre-recorded
// list of actions per call to Stream. Each scriptStep either emits a
// text delta or invokes a named tool with the provided input — the
// latter exercises the tool_use/tool_result wrapping path.
type scriptedStreamer struct {
	steps  []scriptStep
	finish string
	tools  map[string]tools.Tool
}

type scriptStep struct {
	delta    string
	toolName string
	toolIn   string
}

func (s *scriptedStreamer) Stream(ctx context.Context, prompt string, onDelta func(string)) (string, string, error) {
	var collected strings.Builder
	for _, step := range s.steps {
		if step.delta != "" {
			onDelta(step.delta)
			collected.WriteString(step.delta)
			continue
		}
		if step.toolName != "" {
			if t, ok := s.tools[step.toolName]; ok {
				_, _ = t.Run(ctx, json.RawMessage(step.toolIn))
			}
		}
	}
	finish := s.finish
	if finish == "" {
		finish = "end_turn"
	}
	return collected.String(), finish, nil
}

// TestAgentWebSocket end-to-ends the handshake → session → prompt →
// streamed events → end_turn flow against a real httptest server. The
// scripted streamer also fires a tool against the wrapped slice so we
// confirm tool_use and tool_result land on the wire in order.
func TestAgentWebSocket(t *testing.T) {
	echoTool := tools.Tool{
		Name:        "echo",
		Description: "echo input back",
		Schema:      map[string]any{"type": "object"},
		Run: func(ctx context.Context, raw json.RawMessage) (tools.Result, error) {
			return tools.Result{Text: "echoed:" + string(raw)}, nil
		},
	}

	factory := func(wrapped []tools.Tool) (coding.Streamer, string, string) {
		toolMap := make(map[string]tools.Tool, len(wrapped))
		for _, t := range wrapped {
			toolMap[t.Name] = t
		}
		return &scriptedStreamer{
			steps: []scriptStep{
				{delta: "hello "},
				{delta: "world"},
				{toolName: "echo", toolIn: `{"x":1}`},
				{delta: "done"},
			},
			tools: toolMap,
		}, "test-provider", "test-model"
	}

	srv := New(Config{
		Factory:   factory,
		BaseTools: []tools.Tool{echoTool},
		Provider:  "test-provider",
		Model:     "test-model",
		Version:   "test",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/agent"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "done")

	// session frame is unsolicited.
	var ev agentEvent
	if err := wsjson.Read(ctx, c, &ev); err != nil {
		t.Fatalf("read session: %v", err)
	}
	if ev.Type != "session" || ev.Provider != "test-provider" || ev.Model != "test-model" {
		t.Fatalf("session: %+v", ev)
	}

	// Send the prompt; expect the scripted stream of events back.
	if err := wsjson.Write(ctx, c, promptMsg{Type: "prompt", Text: "hi"}); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	expected := []agentEvent{
		{Type: "text_delta", Text: "hello "},
		{Type: "text_delta", Text: "world"},
		{Type: "tool_use", Name: "echo", Input: `{"x":1}`},
		{Type: "tool_result", Name: "echo", Output: `echoed:{"x":1}`},
		{Type: "text_delta", Text: "done"},
		{Type: "end_turn"},
	}
	for i, want := range expected {
		var got agentEvent
		if err := wsjson.Read(ctx, c, &got); err != nil {
			t.Fatalf("read event %d: %v", i, err)
		}
		if got.Type != want.Type {
			t.Fatalf("event %d type: got %q want %q (full=%+v)", i, got.Type, want.Type, got)
		}
		if want.Text != "" && got.Text != want.Text {
			t.Errorf("event %d text: got %q want %q", i, got.Text, want.Text)
		}
		if want.Name != "" && got.Name != want.Name {
			t.Errorf("event %d name: got %q want %q", i, got.Name, want.Name)
		}
		if want.Input != "" && got.Input != want.Input {
			t.Errorf("event %d input: got %q want %q", i, got.Input, want.Input)
		}
		if want.Output != "" && got.Output != want.Output {
			t.Errorf("event %d output: got %q want %q", i, got.Output, want.Output)
		}
	}
}

// TestAgentInvalidPrompt confirms the handler emits an error frame and
// stays open when the client sends junk, instead of dropping the
// connection.
func TestAgentInvalidPrompt(t *testing.T) {
	factory := func([]tools.Tool) (coding.Streamer, string, string) {
		return &scriptedStreamer{steps: []scriptStep{{delta: "ok"}}}, "p", "m"
	}
	srv := New(Config{Factory: factory})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/agent"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "done")

	var ev agentEvent
	_ = wsjson.Read(ctx, c, &ev) // discard session frame

	if err := c.Write(ctx, websocket.MessageText, []byte(`not json`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := wsjson.Read(ctx, c, &ev); err != nil {
		t.Fatalf("read err: %v", err)
	}
	if ev.Type != "error" {
		t.Errorf("expected error frame, got %+v", ev)
	}

	// Recovery: a valid prompt should still work after the error.
	if err := wsjson.Write(ctx, c, promptMsg{Type: "prompt", Text: "hi"}); err != nil {
		t.Fatalf("write recover: %v", err)
	}
	if err := wsjson.Read(ctx, c, &ev); err != nil {
		t.Fatalf("read delta: %v", err)
	}
	if ev.Type != "text_delta" || ev.Text != "ok" {
		t.Errorf("recover frame: %+v", ev)
	}
}
