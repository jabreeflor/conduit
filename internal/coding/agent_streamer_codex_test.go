package coding

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/provider/codex"
	"github.com/jabreeflor/conduit/internal/tools"
)

// makeJWT mirrors the helper in the codex package — a JWT with the given exp.
func makeJWT(exp time.Time) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, _ := json.Marshal(map[string]any{"exp": exp.Unix()})
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

// writeAuth builds an auth.json file in a temp dir and returns the path.
func writeAuth(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	tok := makeJWT(time.Now().Add(time.Hour))
	body, _ := json.Marshal(map[string]any{
		"auth_mode": "Chatgpt",
		"tokens": map[string]any{
			"id_token":      "id",
			"access_token":  tok,
			"refresh_token": "r",
			"account_id":    "acc-1",
		},
	})
	path := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestCodexAgentStreamer_RoundtripsToolCall drives the full loop:
// turn 1 — model emits function_call(read_file, {path:/x})
// turn 2 — model receives function_call_output, emits text, completes.
func TestCodexAgentStreamer_RoundtripsToolCall(t *testing.T) {
	const turn1 = `event: response.output_item.added
data: {"type":"response.output_item.added","item":{"id":"call_1","type":"function_call","call_id":"call_1","name":"read_file","arguments":""}}

event: response.function_call_arguments.delta
data: {"type":"response.function_call_arguments.delta","item_id":"call_1","delta":"{\"path\":\"/x\"}"}

event: response.output_item.done
data: {"type":"response.output_item.done","item":{"id":"call_1","type":"function_call","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"/x\"}"}}

event: response.completed
data: {"type":"response.completed"}

`
	const turn2 = `event: response.output_item.added
data: {"type":"response.output_item.added","item":{"id":"msg_1","type":"message","role":"assistant","content":[]}}

event: response.output_text.delta
data: {"type":"response.output_text.delta","item_id":"msg_1","delta":"the file says hello"}

event: response.output_item.done
data: {"type":"response.output_item.done","item":{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"the file says hello"}]}}

event: response.completed
data: {"type":"response.completed"}

`
	var calls int32
	var sawToolOutput bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 2 {
			var req struct {
				Input []json.RawMessage `json:"input"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			for _, it := range req.Input {
				if strings.Contains(string(it), `"function_call_output"`) && strings.Contains(string(it), `"file contents"`) {
					sawToolOutput = true
				}
			}
			fmt.Fprint(w, turn2)
			return
		}
		fmt.Fprint(w, turn1)
	}))
	defer srv.Close()

	auth, err := codex.LoadAuthFromPath(writeAuth(t))
	if err != nil {
		t.Fatal(err)
	}
	client := codex.NewClient(auth, "gpt-5-codex")
	client.BaseURL = srv.URL

	var ranTool int32
	readFile := tools.Tool{
		Name:        "read_file",
		Description: "read a file",
		Schema:      map[string]any{"type": "object"},
		Run: func(_ context.Context, in json.RawMessage) (tools.Result, error) {
			atomic.AddInt32(&ranTool, 1)
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

	a := NewCodexAgentStreamer(client, []tools.Tool{readFile}, "system")
	var streamed strings.Builder
	full, stop, err := a.Stream(context.Background(), "read /x", func(d string) {
		streamed.WriteString(d)
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if full != "the file says hello" {
		t.Errorf("final text: got %q", full)
	}
	if streamed.String() != "the file says hello" {
		t.Errorf("streamed: got %q", streamed.String())
	}
	if stop != "completed" {
		t.Errorf("stop: got %q want completed", stop)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("provider calls: got %d want 2", calls)
	}
	if atomic.LoadInt32(&ranTool) != 1 {
		t.Errorf("tool runs: got %d want 1", ranTool)
	}
	if !sawToolOutput {
		t.Error("second request did not include function_call_output")
	}
}

func TestCodexAgentStreamer_HandlesUnknownTool(t *testing.T) {
	const turn1 = `event: response.output_item.added
data: {"type":"response.output_item.added","item":{"id":"call_1","type":"function_call","call_id":"call_1","name":"nope","arguments":""}}

event: response.output_item.done
data: {"type":"response.output_item.done","item":{"id":"call_1","type":"function_call","call_id":"call_1","name":"nope","arguments":"{}"}}

event: response.completed
data: {"type":"response.completed"}

`
	const turn2 = `event: response.output_item.added
data: {"type":"response.output_item.added","item":{"id":"msg","type":"message","role":"assistant","content":[]}}

event: response.output_text.delta
data: {"type":"response.output_text.delta","item_id":"msg","delta":"sorry"}

event: response.output_item.done
data: {"type":"response.output_item.done","item":{"id":"msg","type":"message","role":"assistant","content":[{"type":"output_text","text":"sorry"}]}}

event: response.completed
data: {"type":"response.completed"}

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

	auth, _ := codex.LoadAuthFromPath(writeAuth(t))
	c := codex.NewClient(auth, "gpt-5-codex")
	c.BaseURL = srv.URL
	a := NewCodexAgentStreamer(c, nil, "")
	full, stop, err := a.Stream(context.Background(), "go", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if full != "sorry" || stop != "completed" {
		t.Errorf("full=%q stop=%q", full, stop)
	}
}
