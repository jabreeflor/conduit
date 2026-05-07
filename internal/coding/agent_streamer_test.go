package coding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jabreeflor/conduit/internal/provider/anthropic"
	"github.com/jabreeflor/conduit/internal/tools"
)

// TestAgentStreamer_RoundtripsToolUse drives the full loop:
// turn 1 — model emits tool_use(read_file)
// turn 2 — model receives tool_result, emits final text, end_turn
//
// The fake server flips its response based on call count so we don't need
// any real provider.
func TestAgentStreamer_RoundtripsToolUse(t *testing.T) {
	const turn1 = `event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"read_file","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"/x\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}

event: message_stop
data: {"type":"message_stop"}

`
	const turn2 = `event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"the file says hello"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}

event: message_stop
data: {"type":"message_stop"}

`

	var calls int32
	var sawToolResult bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		// Inspect the second request: it must include the tool_result.
		if n == 2 {
			var req struct {
				Messages []json.RawMessage `json:"messages"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			for _, m := range req.Messages {
				if strings.Contains(string(m), `"tool_result"`) && strings.Contains(string(m), `"file contents"`) {
					sawToolResult = true
				}
			}
			fmt.Fprint(w, turn2)
			return
		}
		fmt.Fprint(w, turn1)
	}))
	defer srv.Close()

	client := anthropic.New("k", "claude-test")
	client.BaseURL = srv.URL

	var toolRan int32
	readFile := tools.Tool{
		Name:        "read_file",
		Description: "read a file",
		Schema:      map[string]any{"type": "object"},
		Run: func(_ context.Context, in json.RawMessage) (tools.Result, error) {
			atomic.AddInt32(&toolRan, 1)
			var args struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(in, &args); err != nil {
				return tools.Result{}, err
			}
			if args.Path != "/x" {
				return tools.Result{}, fmt.Errorf("wrong path: %q", args.Path)
			}
			return tools.Result{Text: "file contents"}, nil
		},
	}

	a := NewAgentStreamer(client, []tools.Tool{readFile}, "system")

	var streamed strings.Builder
	full, stop, err := a.Stream(context.Background(), "read /x", func(d string) {
		streamed.WriteString(d)
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if full != "the file says hello" {
		t.Errorf("final text: got %q want %q", full, "the file says hello")
	}
	if streamed.String() != "the file says hello" {
		t.Errorf("streamed: got %q", streamed.String())
	}
	if stop != "end_turn" {
		t.Errorf("stop reason: got %q want end_turn", stop)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("provider calls: got %d want 2", calls)
	}
	if atomic.LoadInt32(&toolRan) != 1 {
		t.Errorf("tool runs: got %d want 1", toolRan)
	}
	if !sawToolResult {
		t.Error("second provider call did not include the tool_result block")
	}
}

func TestAgentStreamer_HandlesUnknownTool(t *testing.T) {
	const turn1 = `event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"nope","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}

event: message_stop
data: {"type":"message_stop"}

`
	const turn2 = `event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"sorry"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}

event: message_stop
data: {"type":"message_stop"}

`
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			fmt.Fprint(w, turn1)
		} else {
			fmt.Fprint(w, turn2)
		}
	}))
	defer srv.Close()

	c := anthropic.New("k", "m")
	c.BaseURL = srv.URL
	a := NewAgentStreamer(c, nil, "")
	full, stop, err := a.Stream(context.Background(), "go", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if full != "sorry" || stop != "end_turn" {
		t.Errorf("full=%q stop=%q", full, stop)
	}
}
