package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeStream returns an SSE body that exercises text_delta + tool_use streaming
// in the same response (mirrors what the live API emits when the model both
// thinks aloud and decides to call a tool).
const fakeStream = `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","role":"assistant"}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Reading "}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"the file."}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"read_file","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"/tmp/x.txt\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}

event: message_stop
data: {"type":"message_stop"}

`

func TestClient_Stream_TextThenToolUse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("missing/wrong x-api-key: %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != apiVersion {
			t.Errorf("wrong anthropic-version: %q", r.Header.Get("anthropic-version"))
		}
		var got streamRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if !got.Stream {
			t.Errorf("expected stream=true")
		}
		if got.Model != "claude-test" {
			t.Errorf("model: got %q want claude-test", got.Model)
		}
		if got.System != "you are a coding agent" {
			t.Errorf("system: got %q", got.System)
		}
		if len(got.Tools) != 1 || got.Tools[0].Name != "read_file" {
			t.Errorf("tools: got %+v", got.Tools)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, fakeStream)
	}))
	defer srv.Close()

	c := New("test-key", "claude-test")
	c.BaseURL = srv.URL

	var collected strings.Builder
	tools := []Tool{{
		Name:        "read_file",
		Description: "read a file",
		InputSchema: map[string]any{"type": "object"},
	}}
	msgs := []Message{{Role: "user", Content: []ContentBlock{{Type: "text", Text: "read /tmp/x.txt"}}}}

	blocks, stop, err := c.Stream(context.Background(), "you are a coding agent", msgs, tools, func(ev Event) {
		if ev.Type == "text_delta" {
			collected.WriteString(ev.Text)
		}
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got, want := collected.String(), "Reading the file."; got != want {
		t.Errorf("text deltas: got %q want %q", got, want)
	}
	if stop != "tool_use" {
		t.Errorf("stop reason: got %q want tool_use", stop)
	}
	if len(blocks) != 2 {
		t.Fatalf("blocks: got %d want 2: %+v", len(blocks), blocks)
	}
	if blocks[0].Type != "text" || blocks[0].Text != "Reading the file." {
		t.Errorf("block[0]: got %+v", blocks[0])
	}
	if blocks[1].Type != "tool_use" || blocks[1].Name != "read_file" {
		t.Errorf("block[1]: got %+v", blocks[1])
	}
	var input map[string]string
	if err := json.Unmarshal(blocks[1].Input, &input); err != nil {
		t.Fatalf("decode tool input: %v (raw=%s)", err, string(blocks[1].Input))
	}
	if input["path"] != "/tmp/x.txt" {
		t.Errorf("tool input path: got %q want /tmp/x.txt", input["path"])
	}
}

func TestClient_Stream_PropagatesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"invalid api key"}}`)
	}))
	defer srv.Close()

	c := New("bad", "m")
	c.BaseURL = srv.URL
	_, _, err := c.Stream(context.Background(), "", []Message{{Role: "user", Content: []ContentBlock{{Type: "text", Text: "hi"}}}}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want 401 error, got %v", err)
	}
}

func TestClient_Stream_RequiresAPIKeyAndModel(t *testing.T) {
	c := &Client{}
	if _, _, err := c.Stream(context.Background(), "", nil, nil, nil); err == nil {
		t.Error("expected error for missing APIKey")
	}
	c.APIKey = "x"
	if _, _, err := c.Stream(context.Background(), "", nil, nil, nil); err == nil {
		t.Error("expected error for missing Model")
	}
}
