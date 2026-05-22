package endpoint

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jabreeflor/conduit/internal/router"
)

// ── Validate ─────────────────────────────────────────────────────────────────

func TestValidateOpenAICompatOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer key123" {
			t.Errorf("missing auth header")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := Validate(context.Background(), Config{
		Type:    TypeOpenAICompat,
		BaseURL: srv.URL,
		APIKey:  "key123",
	})
	if err != nil {
		t.Fatalf("Validate returned unexpected error: %v", err)
	}
}

func TestValidateOpenAICompatAuthFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := Validate(context.Background(), Config{
		Type:    TypeOpenAICompat,
		BaseURL: srv.URL,
		APIKey:  "bad-key",
	})
	if err == nil {
		t.Fatal("expected auth error, got nil")
	}
}

func TestValidateOpenAICompatConnFail(t *testing.T) {
	err := Validate(context.Background(), Config{
		Type:    TypeOpenAICompat,
		BaseURL: "http://127.0.0.1:19999", // nothing listening
		APIKey:  "any",
	})
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
}

func TestValidateAnthropicOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "ant-key" {
			t.Errorf("missing x-api-key header")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := Validate(context.Background(), Config{
		Type:    TypeAnthropic,
		BaseURL: srv.URL,
		APIKey:  "ant-key",
	})
	if err != nil {
		t.Fatalf("Validate returned unexpected error: %v", err)
	}
}

// ── Provider.Infer (OpenAI-compatible) ───────────────────────────────────────

func TestProviderInferOpenAICompat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{{Message: struct {
				Content string `json:"content"`
			}{Content: "hello from openai"}}},
			Usage: struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			}{PromptTokens: 10, CompletionTokens: 5},
		})
	}))
	defer srv.Close()

	p := New(Config{
		Name:    "test-openai",
		Type:    TypeOpenAICompat,
		BaseURL: srv.URL,
		APIKey:  "key",
		Model:   "gpt-4o",
	})

	// Temporarily replace buildOpenAIMessages with a working stub so this test
	// passes before the user implements the real function.
	orig := buildOpenAIMessages
	buildOpenAIMessages = func(req router.Request) []openAIMessage {
		return []openAIMessage{{Role: "user", Content: req.Prompt}}
	}
	defer func() { buildOpenAIMessages = orig }()

	resp, err := p.Infer(context.Background(), router.Request{Prompt: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "hello from openai" {
		t.Fatalf("Text = %q, want 'hello from openai'", resp.Text)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 {
		t.Fatalf("Usage = %+v, want {10 5}", resp.Usage)
	}
}

// ── Provider.Infer (Anthropic) ────────────────────────────────────────────────

func TestProviderInferAnthropic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "ant-key" {
			t.Errorf("missing x-api-key")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicResponse{
			Content: []struct {
				Text string `json:"text"`
			}{{Text: "hello from anthropic"}},
			Usage: struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			}{InputTokens: 8, OutputTokens: 4},
		})
	}))
	defer srv.Close()

	p := New(Config{
		Name:    "test-anthropic",
		Type:    TypeAnthropic,
		BaseURL: srv.URL,
		APIKey:  "ant-key",
		Model:   "claude-opus-4-6",
	})

	resp, err := p.Infer(context.Background(), router.Request{Prompt: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "hello from anthropic" {
		t.Fatalf("Text = %q, want 'hello from anthropic'", resp.Text)
	}
	if resp.Usage.InputTokens != 8 || resp.Usage.OutputTokens != 4 {
		t.Fatalf("Usage = %+v, want {8 4}", resp.Usage)
	}
}

// ── buildOpenAIMessages spec ──────────────────────────────────────────────────
//
// These tests define the required behaviour. They will fail until you implement
// buildOpenAIMessages in provider.go.

func TestBuildOpenAIMessages(t *testing.T) {
	t.Run("prompt only produces single user message", func(t *testing.T) {
		msgs := buildOpenAIMessages(router.Request{Prompt: "hello"})
		if len(msgs) != 1 {
			t.Fatalf("want 1 message, got %d", len(msgs))
		}
		if msgs[0].Role != "user" {
			t.Fatalf("Role = %q, want user", msgs[0].Role)
		}
		if msgs[0].Content != "hello" {
			t.Fatalf("Content = %q, want 'hello'", msgs[0].Content)
		}
	})

	t.Run("empty prompt still returns one user message", func(t *testing.T) {
		msgs := buildOpenAIMessages(router.Request{})
		if len(msgs) == 0 {
			t.Fatal("want at least one message for empty prompt")
		}
		if msgs[len(msgs)-1].Role != "user" {
			t.Fatalf("last message Role = %q, want user", msgs[len(msgs)-1].Role)
		}
	})

	t.Run("text inputs are prepended before the prompt", func(t *testing.T) {
		req := router.Request{
			Inputs: []router.Input{
				{Type: router.InputText, Text: "context A"},
				{Type: router.InputText, Text: "context B"},
			},
			Prompt: "my question",
		}
		msgs := buildOpenAIMessages(req)
		// Must have at least the two inputs + the prompt.
		if len(msgs) < 3 {
			t.Fatalf("want ≥3 messages (2 inputs + prompt), got %d: %+v", len(msgs), msgs)
		}
		last := msgs[len(msgs)-1]
		if last.Content != "my question" {
			t.Fatalf("last message Content = %q, want 'my question'", last.Content)
		}
	})
}
