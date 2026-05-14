package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/jabreeflor/conduit/internal/coding"
	"github.com/jabreeflor/conduit/internal/server"
	"github.com/jabreeflor/conduit/internal/tools"
)

// TestServeAgentFullJourney is the end-to-end story of a browser-shaped
// session against `conduit serve`: REST surface walkthrough, WebSocket
// agent prompt/response loop, tool emission, error recovery, and
// per-connection isolation. The journey is intentionally long because
// the GUI's behaviour is the integration of all of these working in
// concert; breaking any one of them breaks the product.
func TestServeAgentFullJourney(t *testing.T) {
	// ── 1. Bootstrap ────────────────────────────────────────────────
	home := newHome(t)

	older := filepath.Join(home.CodingSessions, "code-1-aaa.jsonl")
	newer := filepath.Join(home.CodingSessions, "code-2-bbb.jsonl")
	mustWrite(t, older, `{"role":"user","content":"older prompt body"}`+"\n")
	mustWrite(t, newer, `{"role":"user","content":"newer prompt body"}`+"\n")
	now := time.Now()
	if err := os.Chtimes(older, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatalf("chtimes older: %v", err)
	}
	if err := os.Chtimes(newer, now, now); err != nil {
		t.Fatalf("chtimes newer: %v", err)
	}

	// ── 2. Server construction ──────────────────────────────────────
	// echoBack is the BASE tool we hand to the server. Each connection's
	// factory will receive the *wrapped* slice (tool_use/tool_result
	// instrumented) and we'll drive it from a custom streamer below.
	echoBack := tools.Tool{
		Name:        "echo_back",
		Description: "echo back its raw input",
		Schema:      map[string]any{"type": "object"},
		Run: func(_ context.Context, raw json.RawMessage) (tools.Result, error) {
			return tools.Result{Text: "echoed:" + string(raw)}, nil
		},
	}

	// journeyStreamer is per-connection: it serves scripted text-delta
	// replies, optionally invokes a named wrapped tool, and records each
	// prompt so we can verify per-connection isolation.
	type streamerState struct {
		mu       sync.Mutex
		wrapped  map[string]tools.Tool
		prompts  []string
		queue    []journeyReply
		fallback journeyReply
	}
	makeStream := func(state *streamerState) coding.Streamer {
		return journeyStreamerFunc(func(ctx context.Context, prompt string, onDelta func(string)) (string, string, error) {
			state.mu.Lock()
			state.prompts = append(state.prompts, prompt)
			var r journeyReply
			if len(state.queue) > 0 {
				r = state.queue[0]
				state.queue = state.queue[1:]
			} else {
				r = state.fallback
			}
			state.mu.Unlock()

			var collected strings.Builder
			for _, step := range r.steps {
				if err := ctx.Err(); err != nil {
					return "", "", err
				}
				if step.delta != "" {
					if onDelta != nil {
						onDelta(step.delta)
					}
					collected.WriteString(step.delta)
					continue
				}
				if step.toolName != "" {
					state.mu.Lock()
					tool, ok := state.wrapped[step.toolName]
					state.mu.Unlock()
					if !ok {
						return "", "", fmt.Errorf("scripted tool %q not in wrapped slice", step.toolName)
					}
					if _, err := tool.Run(ctx, json.RawMessage(step.toolIn)); err != nil {
						return "", "", err
					}
				}
			}
			full := r.full
			if full == "" {
				full = collected.String()
			}
			finish := r.finish
			if finish == "" {
				finish = "stop"
			}
			return full, finish, nil
		})
	}

	// One state object per WS connection so we can introspect what the
	// streamer saw. The factory is called once per connection per
	// agent.go's contract.
	var connStates []*streamerState
	var connMu sync.Mutex
	var connCount int32

	factory := func(wrapped []tools.Tool) (coding.Streamer, string, string) {
		idx := atomic.AddInt32(&connCount, 1) - 1
		state := &streamerState{
			wrapped: make(map[string]tools.Tool, len(wrapped)),
			fallback: journeyReply{
				steps:  []journeyStep{{delta: "fallback "}, {delta: "reply"}},
				finish: "stop",
			},
		}
		for _, w := range wrapped {
			state.wrapped[w.Name] = w
		}
		// Conn 0 is the main journey; conn 1 is the parallel-isolation
		// peer. We seed scripted queues per-connection from the outer
		// test; missing entries fall back to the default reply.
		connMu.Lock()
		// Grow connStates as needed.
		for int(idx) >= len(connStates) {
			connStates = append(connStates, nil)
		}
		if connStates[idx] == nil {
			connStates[idx] = state
		} else {
			// A test pre-populated this slot — adopt its queue/fallback
			// but always rebind wrapped to the *current* connection's
			// wrapped slice (so tool_use events flow through the right
			// emitter).
			pre := connStates[idx]
			pre.wrapped = state.wrapped
			state = pre
		}
		connMu.Unlock()
		return makeStream(state), "test-provider", "test-model"
	}

	// Pre-stage connection 0's reply queue: three turns plus a malformed
	// turn (server emits error frame and stays open), then a recovery
	// turn after the error. We pre-create the state so we can write its
	// queue before the connection dials.
	conn0State := &streamerState{
		queue: []journeyReply{
			// Turn 1: simple deltas.
			{steps: []journeyStep{{delta: "Hello"}, {delta: ", "}, {delta: "world!"}}, finish: "stop"},
			// Turn 2: a different deltas pattern.
			{steps: []journeyStep{{delta: "Second "}, {delta: "turn"}}, finish: "stop"},
			// Turn 3: tool use surrounded by deltas.
			{steps: []journeyStep{
				{delta: "calling "},
				{toolName: "echo_back", toolIn: `{"hello":"there"}`},
				{delta: " done"},
			}, finish: "stop"},
			// Turn 4 (recovery after error frame): a simple delta.
			{steps: []journeyStep{{delta: "recovered"}}, finish: "stop"},
		},
		fallback: journeyReply{
			steps: []journeyStep{{delta: "fallback"}}, finish: "stop",
		},
	}
	connMu.Lock()
	connStates = append(connStates, conn0State)
	connMu.Unlock()

	srv := server.New(server.Config{
		Factory:     factory,
		BaseTools:   []tools.Tool{echoBack},
		Provider:    "test-provider",
		Model:       "test-model",
		Version:     "e2e-test",
		HomeDir:     home.Root,
		SessionsDir: home.CodingSessions,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	httpClient := &http.Client{Timeout: 5 * time.Second}

	// ── 3. REST surface walkthrough ─────────────────────────────────
	// /api/info
	{
		resp := mustGET(t, httpClient, ts.URL+"/api/info")
		var got map[string]any
		decodeBody(t, resp, &got)
		if got["provider"] != "test-provider" {
			t.Errorf("/api/info provider: got %v want test-provider", got["provider"])
		}
		if got["model"] != "test-model" {
			t.Errorf("/api/info model: got %v want test-model", got["model"])
		}
		if got["version"] != "e2e-test" {
			t.Errorf("/api/info version: got %v want e2e-test", got["version"])
		}
	}

	// /api/memory
	{
		resp := mustGET(t, httpClient, ts.URL+"/api/memory")
		var got map[string]any
		decodeBody(t, resp, &got)
		soul, _ := got["soul"].(string)
		user, _ := got["user"].(string)
		if !strings.Contains(soul, "You are Conduit.") {
			t.Errorf("/api/memory soul: missing seeded SOUL.md content, got %q", soul)
		}
		if !strings.Contains(user, "concise replies") {
			t.Errorf("/api/memory user: missing seeded USER.md content, got %q", user)
		}
	}

	// /api/sessions
	{
		resp := mustGET(t, httpClient, ts.URL+"/api/sessions")
		var got []map[string]any
		decodeBody(t, resp, &got)
		if len(got) != 2 {
			t.Fatalf("/api/sessions count: got %d want 2 (%+v)", len(got), got)
		}
		if id, _ := got[0]["id"].(string); id != "code-2-bbb" {
			t.Errorf("/api/sessions newest: got %q want code-2-bbb", id)
		}
		if id, _ := got[1]["id"].(string); id != "code-1-aaa" {
			t.Errorf("/api/sessions oldest: got %q want code-1-aaa", id)
		}
	}

	// /api/projects — assert 200 and JSON only.
	{
		resp := mustGET(t, httpClient, ts.URL+"/api/projects")
		if resp.StatusCode/100 == 5 {
			t.Fatalf("/api/projects: server error %d", resp.StatusCode)
		}
		var anyShape any
		decodeBody(t, resp, &anyShape)
		if anyShape == nil {
			t.Errorf("/api/projects: nil body")
		}
	}

	// OPTIONS /api/info — CORS preflight.
	{
		req, err := http.NewRequest(http.MethodOptions, ts.URL+"/api/info", nil)
		if err != nil {
			t.Fatalf("options req: %v", err)
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			t.Fatalf("options do: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("options status: got %d want 204", resp.StatusCode)
		}
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("options CORS: got %q want *", got)
		}
	}

	// ── 4. WebSocket round-trip ─────────────────────────────────────
	wsCtx, wsCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer wsCancel()

	wsURL := strings.Replace(ts.URL, "http", "ws", 1) + "/api/agent"
	c, _, err := websocket.Dial(wsCtx, wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "done")

	// Session frame (unsolicited).
	first := readEvent(t, wsCtx, c)
	if first.Type != "session" {
		t.Fatalf("first frame type: got %q want session (%+v)", first.Type, first)
	}
	if first.Provider != "test-provider" {
		t.Errorf("session provider: got %q want test-provider", first.Provider)
	}
	if first.ID == "" {
		t.Errorf("session id: empty")
	}
	sessionID1 := first.ID

	// Turn 1: send prompt, expect three deltas + end_turn.
	writeJSON(t, wsCtx, c, promptFrame("first message"))
	gotText1 := drainTurn(t, wsCtx, c)
	if gotText1 != "Hello, world!" {
		t.Errorf("turn 1 concatenated deltas: got %q want %q", gotText1, "Hello, world!")
	}

	// Turn 2: another prompt over the SAME connection. Per agent.go,
	// one streamer per connection — both prompts hit the same streamer
	// so its call log should now contain both prompts in order.
	writeJSON(t, wsCtx, c, promptFrame("second message"))
	gotText2 := drainTurn(t, wsCtx, c)
	if gotText2 != "Second turn" {
		t.Errorf("turn 2 concatenated deltas: got %q want %q", gotText2, "Second turn")
	}

	conn0State.mu.Lock()
	gotPrompts := append([]string(nil), conn0State.prompts...)
	conn0State.mu.Unlock()
	if len(gotPrompts) < 2 {
		t.Fatalf("conn0 streamer prompts: got %d want >=2 (%v)", len(gotPrompts), gotPrompts)
	}
	if gotPrompts[0] != "first message" || gotPrompts[1] != "second message" {
		t.Errorf("conn0 streamer prompt order: got %v want [first message, second message]", gotPrompts)
	}

	// ── 5. Tool emission on the wire ────────────────────────────────
	writeJSON(t, wsCtx, c, promptFrame("third message"))

	var sawText, sawToolUse, sawToolResult, sawEnd bool
	var toolUseEv, toolResultEv agentEv
	for !sawEnd {
		ev := readEvent(t, wsCtx, c)
		switch ev.Type {
		case "text_delta":
			sawText = true
		case "tool_use":
			sawToolUse = true
			toolUseEv = ev
		case "tool_result":
			sawToolResult = true
			toolResultEv = ev
		case "end_turn":
			sawEnd = true
		case "error":
			t.Fatalf("turn 3 unexpected error frame: %+v", ev)
		}
	}
	if !sawText {
		t.Errorf("turn 3: never saw text_delta")
	}
	if !sawToolUse {
		t.Fatalf("turn 3: never saw tool_use")
	}
	if toolUseEv.Name != "echo_back" {
		t.Errorf("tool_use name: got %q want echo_back", toolUseEv.Name)
	}
	if !sawToolResult {
		t.Fatalf("turn 3: never saw tool_result")
	}
	if toolResultEv.Name != "echo_back" {
		t.Errorf("tool_result name: got %q want echo_back", toolResultEv.Name)
	}
	if !strings.Contains(toolResultEv.Output, "echoed:") {
		t.Errorf("tool_result output: got %q want substring %q", toolResultEv.Output, "echoed:")
	}

	// ── 6. Error path ───────────────────────────────────────────────
	// Send a malformed prompt frame (wrong type). Expect error frame
	// back, connection stays open.
	bad, _ := json.Marshal(map[string]string{"type": "not-prompt", "text": "x"})
	if err := c.Write(wsCtx, websocket.MessageText, bad); err != nil {
		t.Fatalf("write malformed: %v", err)
	}
	errEv := readEvent(t, wsCtx, c)
	if errEv.Type != "error" {
		t.Fatalf("expected error frame, got %+v", errEv)
	}
	if !strings.Contains(errEv.Message, "expected") {
		t.Errorf("error message: got %q want substring %q", errEv.Message, "expected")
	}

	// Recovery: send a valid prompt; expect normal lifecycle.
	writeJSON(t, wsCtx, c, promptFrame("recovery"))
	recoveryText := drainTurn(t, wsCtx, c)
	if recoveryText != "recovered" {
		t.Errorf("recovery text: got %q want %q", recoveryText, "recovered")
	}

	// ── 7. Concurrent connections are isolated ──────────────────────
	// Pre-stage conn1 and conn2 states. conn0 already holds slot 0.
	conn1State := &streamerState{
		queue: []journeyReply{
			{steps: []journeyStep{{delta: "alpha"}}, finish: "stop"},
		},
		fallback: journeyReply{steps: []journeyStep{{delta: "alpha-fb"}}, finish: "stop"},
	}
	conn2State := &streamerState{
		queue: []journeyReply{
			{steps: []journeyStep{{delta: "beta"}}, finish: "stop"},
		},
		fallback: journeyReply{steps: []journeyStep{{delta: "beta-fb"}}, finish: "stop"},
	}
	connMu.Lock()
	connStates = append(connStates, conn1State, conn2State)
	connMu.Unlock()

	parCtx, parCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer parCancel()

	type connResult struct {
		sessionID string
		text      string
		err       error
	}
	results := make(chan connResult, 2)

	runPeer := func(label, text string) {
		go func() {
			pc, _, dialErr := websocket.Dial(parCtx, wsURL, nil)
			if dialErr != nil {
				results <- connResult{err: fmt.Errorf("%s dial: %w", label, dialErr)}
				return
			}
			defer pc.Close(websocket.StatusNormalClosure, "done")

			sess := readEvent(noopFatal{}, parCtx, pc)
			if sess.Type != "session" {
				results <- connResult{err: fmt.Errorf("%s expected session frame, got %+v", label, sess)}
				return
			}
			payload, _ := json.Marshal(promptMsg{Type: "prompt", Text: text})
			if err := pc.Write(parCtx, websocket.MessageText, payload); err != nil {
				results <- connResult{err: fmt.Errorf("%s write: %w", label, err)}
				return
			}
			var accum strings.Builder
			for {
				ev := readEvent(noopFatal{}, parCtx, pc)
				switch ev.Type {
				case "text_delta":
					accum.WriteString(ev.Text)
				case "end_turn":
					results <- connResult{sessionID: sess.ID, text: accum.String()}
					return
				case "error":
					results <- connResult{err: fmt.Errorf("%s error frame: %s", label, ev.Message)}
					return
				}
			}
		}()
	}

	runPeer("peer-A", "from-A")
	runPeer("peer-B", "from-B")

	var peerResults []connResult
	for i := 0; i < 2; i++ {
		select {
		case r := <-results:
			if r.err != nil {
				t.Fatalf("peer: %v", r.err)
			}
			peerResults = append(peerResults, r)
		case <-time.After(5 * time.Second):
			t.Fatalf("peer timeout waiting for result %d", i)
		}
	}
	if len(peerResults) != 2 {
		t.Fatalf("peer results: got %d want 2", len(peerResults))
	}
	if peerResults[0].sessionID == "" || peerResults[1].sessionID == "" {
		t.Errorf("peer session IDs empty: %+v", peerResults)
	}
	// The session IDs are stamped from time.Now().Unix() in agent.go, so
	// they MAY collide when two dials land in the same second. Don't
	// require distinctness — that's an implementation detail. Do require
	// they got their OWN session frames and own end_turn frames (the
	// fact that we collected two results proves this).
	gotTexts := map[string]bool{peerResults[0].text: true, peerResults[1].text: true}
	if !gotTexts["alpha"] || !gotTexts["beta"] {
		t.Errorf("peer texts: got %v want both alpha and beta", gotTexts)
	}
	// And the main connection's session ID is from a different second
	// (we did several turns since dialing) — sanity check it's still
	// non-empty.
	if sessionID1 == "" {
		t.Errorf("main session id was empty")
	}
}

// ── helpers ─────────────────────────────────────────────────────────

// journeyStep is one action inside a scripted reply: either emit a text
// delta or run a named tool from the wrapped slice.
type journeyStep struct {
	delta    string
	toolName string
	toolIn   string
}

// journeyReply is one full Stream() reply: the steps to play, the
// optional aggregated full string, and the finish reason.
type journeyReply struct {
	steps  []journeyStep
	full   string
	finish string
}

// journeyStreamerFunc adapts a Stream-shaped function into a
// coding.Streamer so each connection can have its own behaviour without
// defining a struct per test.
type journeyStreamerFunc func(ctx context.Context, prompt string, onDelta func(string)) (string, string, error)

func (f journeyStreamerFunc) Stream(ctx context.Context, prompt string, onDelta func(string)) (string, string, error) {
	return f(ctx, prompt, onDelta)
}

var _ coding.Streamer = journeyStreamerFunc(nil)

// agentEv mirrors the server's agentEvent wire shape (unexported in the
// server package, so we redeclare here).
type agentEv struct {
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

type promptMsg struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func promptFrame(text string) promptMsg { return promptMsg{Type: "prompt", Text: text} }

// fatalLogger is the minimum surface we need from *testing.T inside
// helpers — extracted so we can pass a noop variant from goroutines that
// must not call t.Fatalf concurrently.
type fatalLogger interface {
	Fatalf(format string, args ...any)
	Helper()
}

type noopFatal struct{}

func (noopFatal) Fatalf(string, ...any) {}
func (noopFatal) Helper()               {}

func readEvent(t fatalLogger, ctx context.Context, c *websocket.Conn) agentEv {
	t.Helper()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("ws read: %v", err)
		return agentEv{}
	}
	var ev agentEv
	if err := json.Unmarshal(data, &ev); err != nil {
		t.Fatalf("ws decode: %v (raw=%s)", err, string(data))
		return agentEv{}
	}
	return ev
}

func writeJSON(t *testing.T, ctx context.Context, c *websocket.Conn, v any) {
	t.Helper()
	payload, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := c.Write(ctx, websocket.MessageText, payload); err != nil {
		t.Fatalf("ws write: %v", err)
	}
}

// drainTurn reads frames until end_turn, concatenating all text_delta
// payloads. Fails the test on error or premature error frame.
func drainTurn(t *testing.T, ctx context.Context, c *websocket.Conn) string {
	t.Helper()
	var b strings.Builder
	for {
		ev := readEvent(t, ctx, c)
		switch ev.Type {
		case "text_delta":
			b.WriteString(ev.Text)
		case "end_turn":
			return b.String()
		case "error":
			t.Fatalf("drainTurn unexpected error frame: %s", ev.Message)
			return b.String()
		case "tool_use", "tool_result", "session":
			// Ignore — these don't contribute to the concatenated text.
		}
	}
}

func mustGET(t *testing.T, client *http.Client, url string) *http.Response {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", url, resp.StatusCode)
	}
	return resp
}

func decodeBody(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode body: %v", err)
	}
}
