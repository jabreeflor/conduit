package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/jabreeflor/conduit/internal/surface/coding"
	"github.com/jabreeflor/conduit/internal/tools"
)

// agentEvent is the wire shape the GUI consumes. Only fields relevant to
// a given event type are populated; omitempty keeps the JSON minimal.
type agentEvent struct {
	Type     string `json:"type"`
	ID       string `json:"id,omitempty"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Name     string `json:"name,omitempty"`
	Text     string `json:"text,omitempty"`
	Input    string `json:"input,omitempty"`
	Output   string `json:"output,omitempty"`
	IsError  bool   `json:"isError,omitempty"`
	Message  string `json:"message,omitempty"`
}

// promptMsg is the only client→server frame the agent endpoint
// currently understands.
type promptMsg struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// handleAgent implements the WebSocket protocol documented in the
// package README. One streamer + one wrapped tool slice are bound per
// connection so concurrent sessions don't share conversation history
// or tool-event channels.
func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	if s.factory == nil {
		http.Error(w, "agent: no streamer factory configured", http.StatusServiceUnavailable)
		return
	}

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Tests and curl-based smoke checks may come from arbitrary
		// origins; the server is intended to be loopback-only and
		// gated by --addr, so origin enforcement adds friction without
		// security benefit. Lock down at the listener instead.
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer c.Close(websocket.StatusInternalError, "closing")

	ctx := r.Context()
	emitter := newEventEmitter(ctx, c)

	// Wrap the base tool slice so every Run is sandwiched between a
	// tool_use and a tool_result event on the wire.
	wrappedTools := wrapTools(s.baseTools, emitter)

	streamer, providerName, modelName := s.factory(wrappedTools)
	sessionID := fmt.Sprintf("code-%d", time.Now().UTC().Unix())

	if err := emitter.send(agentEvent{
		Type:     "session",
		ID:       sessionID,
		Provider: providerName,
		Model:    modelName,
	}); err != nil {
		return
	}

	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		var msg promptMsg
		if err := json.Unmarshal(data, &msg); err != nil || msg.Type != "prompt" {
			_ = emitter.send(agentEvent{Type: "error", Message: "expected {type:\"prompt\", text:string}"})
			continue
		}
		runTurn(ctx, streamer, msg.Text, emitter)
	}
}

// runTurn drives one user→assistant exchange. text_delta callbacks are
// translated 1:1; the streamer's terminal return becomes either an
// end_turn or error frame. We never close the connection on an error —
// the GUI is expected to recover by sending the next prompt.
func runTurn(ctx context.Context, streamer coding.Streamer, prompt string, em *eventEmitter) {
	_, _, err := streamer.Stream(ctx, prompt, func(delta string) {
		_ = em.send(agentEvent{Type: "text_delta", Text: delta})
	})
	if err != nil {
		_ = em.send(agentEvent{Type: "error", Message: err.Error()})
		return
	}
	_ = em.send(agentEvent{Type: "end_turn"})
}

// eventEmitter serializes writes to the WebSocket. coder/websocket does
// not serialize Write internally, and our tool-result and text_delta
// callbacks may fire from goroutines spawned by the pipeline wrapper,
// so we guard with a mutex.
type eventEmitter struct {
	ctx context.Context
	c   *websocket.Conn
	mu  sync.Mutex
}

func newEventEmitter(ctx context.Context, c *websocket.Conn) *eventEmitter {
	return &eventEmitter{ctx: ctx, c: c}
}

func (e *eventEmitter) send(ev agentEvent) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.c.Write(e.ctx, websocket.MessageText, payload)
}

// wrapTools returns a copy of the input slice with each tool's Run
// instrumented to emit tool_use before invocation and tool_result after.
// The tool's own Result/error path is preserved exactly so the streamer
// continues to see authentic provider behaviour.
func wrapTools(base []tools.Tool, em *eventEmitter) []tools.Tool {
	out := make([]tools.Tool, len(base))
	for i, t := range base {
		t := t // capture
		original := t.Run
		t.Run = func(ctx context.Context, raw json.RawMessage) (tools.Result, error) {
			callID := fmt.Sprintf("call_%d", time.Now().UnixNano())
			input := string(raw)
			if input == "" {
				input = "{}"
			}
			_ = em.send(agentEvent{Type: "tool_use", ID: callID, Name: t.Name, Input: input})
			res, err := original(ctx, raw)
			out := res.Text
			isError := res.IsError
			if err != nil {
				out = err.Error()
				isError = true
			}
			_ = em.send(agentEvent{Type: "tool_result", ID: callID, Name: t.Name, Output: out, IsError: isError})
			return res, err
		}
		out[i] = t
	}
	return out
}
