package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeResponsesStream emulates the SSE shape codex-rs/codex-api/src/sse/responses.rs
// emits for a turn that produces text, then a function_call, then completes.
const fakeResponsesStream = `event: response.created
data: {"type":"response.created"}

event: response.output_item.added
data: {"type":"response.output_item.added","item":{"id":"msg_1","type":"message","role":"assistant","content":[]}}

event: response.output_text.delta
data: {"type":"response.output_text.delta","item_id":"msg_1","delta":"I'll read "}

event: response.output_text.delta
data: {"type":"response.output_text.delta","item_id":"msg_1","delta":"the file."}

event: response.output_item.done
data: {"type":"response.output_item.done","item":{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"I'll read the file."}]}}

event: response.output_item.added
data: {"type":"response.output_item.added","item":{"id":"call_1","type":"function_call","call_id":"call_1","name":"read_file","arguments":""}}

event: response.function_call_arguments.delta
data: {"type":"response.function_call_arguments.delta","item_id":"call_1","delta":"{\"path\":"}

event: response.function_call_arguments.delta
data: {"type":"response.function_call_arguments.delta","item_id":"call_1","delta":"\"/tmp/x\"}"}

event: response.output_item.done
data: {"type":"response.output_item.done","item":{"id":"call_1","type":"function_call","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"/tmp/x\"}"}}

event: response.completed
data: {"type":"response.completed"}

`

func newTestAuth(t *testing.T) *Auth {
	t.Helper()
	dir := t.TempDir()
	tok := makeJWT(time.Now().Add(time.Hour))
	path := filepath.Join(dir, "auth.json")
	body, _ := json.Marshal(authFile{
		AuthMode: "Chatgpt",
		Tokens:   &Tokens{AccessToken: tok, RefreshToken: "r", AccountID: "acc-1"},
	})
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := LoadAuthFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestClient_Stream_TextThenFunctionCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("path: got %q want /responses", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization header")
		}
		if r.Header.Get("ChatGPT-Account-ID") != "acc-1" {
			t.Errorf("ChatGPT-Account-ID: got %q", r.Header.Get("ChatGPT-Account-ID"))
		}
		if r.Header.Get("originator") != originator {
			t.Errorf("originator: got %q want %q", r.Header.Get("originator"), originator)
		}
		var got requestBody
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got.Model != "gpt-5-codex" {
			t.Errorf("model: got %q", got.Model)
		}
		if !got.Stream {
			t.Error("expected stream=true")
		}
		if got.Instructions != "you are codex" {
			t.Errorf("instructions: got %q", got.Instructions)
		}
		if len(got.Tools) != 1 || got.Tools[0].Name != "read_file" {
			t.Errorf("tools: got %+v", got.Tools)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, fakeResponsesStream)
	}))
	defer srv.Close()

	c := NewClient(newTestAuth(t), "gpt-5-codex")
	c.BaseURL = srv.URL

	var streamed strings.Builder
	tools := []Tool{{Type: "function", Name: "read_file", Description: "read a file", Parameters: map[string]any{"type": "object"}}}
	input := []Item{{Type: "message", Role: "user", Content: []ItemContentBlock{{Type: "input_text", Text: "read /tmp/x"}}}}

	items, err := c.Stream(context.Background(), "you are codex", input, tools, "thread-1", func(ev Event) {
		if ev.Type == "text_delta" {
			streamed.WriteString(ev.Text)
		}
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if streamed.String() != "I'll read the file." {
		t.Errorf("streamed: got %q", streamed.String())
	}
	if len(items) != 2 {
		t.Fatalf("items: got %d want 2: %+v", len(items), items)
	}
	if items[0].Type != "message" {
		t.Errorf("items[0].Type: got %q want message", items[0].Type)
	}
	if items[1].Type != "function_call" || items[1].Name != "read_file" {
		t.Errorf("items[1]: got %+v", items[1])
	}
	if items[1].Arguments != `{"path":"/tmp/x"}` {
		t.Errorf("items[1].Arguments: got %q", items[1].Arguments)
	}
	if items[1].CallID != "call_1" {
		t.Errorf("items[1].CallID: got %q want call_1", items[1].CallID)
	}
}

func TestClient_Stream_FailedSurfacesError(t *testing.T) {
	const body = `event: response.failed
data: {"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","message":"too fast"}}}

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	c := NewClient(newTestAuth(t), "gpt-5-codex")
	c.BaseURL = srv.URL
	_, err := c.Stream(context.Background(), "", nil, nil, "", nil)
	if err == nil || !strings.Contains(err.Error(), "too fast") {
		t.Fatalf("want failed error, got %v", err)
	}
}

func TestClient_Stream_PropagatesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"invalid_token"}`)
	}))
	defer srv.Close()
	c := NewClient(newTestAuth(t), "m")
	c.BaseURL = srv.URL
	_, err := c.Stream(context.Background(), "", nil, nil, "", nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want 401, got %v", err)
	}
}
