package endpoint

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/router"
)

// Provider implements router.Provider for an external API endpoint.
type Provider struct {
	cfg    Config
	client *http.Client
}

// New creates a Provider for the given external endpoint config.
func New(cfg Config) *Provider {
	return &Provider{
		cfg:    cfg,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// Name returns the provider identifier used by the router.
func (p *Provider) Name() string { return p.cfg.Name }

// Infer dispatches the request to the external endpoint and returns the first
// choice as a normalized router.Response.
func (p *Provider) Infer(ctx context.Context, req router.Request) (router.Response, error) {
	if p.cfg.Type == TypeAnthropic {
		return p.callAnthropic(ctx, req)
	}
	return p.callOpenAICompat(ctx, req)
}

// ── OpenAI-compatible ────────────────────────────────────────────────────────

type openAIRequest struct {
	Model     string          `json:"model"`
	Messages  []openAIMessage `json:"messages"`
	MaxTokens int             `json:"max_tokens,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// buildOpenAIMessages maps a router.Request to the OpenAI chat messages array.
//
// Design choices to consider:
//   - Text Inputs (Type == router.InputText) carry additional context. Should they
//     become separate "user" messages prepended before the Prompt, or be
//     concatenated into a single message?
//   - Should TaskType (e.g. TaskCode) produce a "system" role message that
//     primes the model's behaviour, or is that unnecessary overhead?
//   - An empty Prompt is valid — always return at least one user message so
//     callOpenAICompat never sends an empty messages array.
//
// The test at TestBuildOpenAIMessages specifies the required cases.
//
// Declared as a variable so tests can substitute a stub before the real
// implementation is written.
//
// TODO: replace the nil body with your implementation (5–10 lines).
var buildOpenAIMessages = func(req router.Request) []openAIMessage {
	var msgs []openAIMessage
	for _, input := range req.Inputs {
		if input.Type == router.InputText && input.Text != "" {
			msgs = append(msgs, openAIMessage{Role: "user", Content: input.Text})
		}
	}
	msgs = append(msgs, openAIMessage{Role: "user", Content: req.Prompt})
	return msgs
}

func (p *Provider) callOpenAICompat(ctx context.Context, req router.Request) (router.Response, error) {
	msgs := buildOpenAIMessages(req)
	if len(msgs) == 0 {
		return router.Response{}, fmt.Errorf("buildOpenAIMessages returned no messages")
	}

	payload := openAIRequest{
		Model:     p.cfg.Model,
		Messages:  msgs,
		MaxTokens: 4096,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return router.Response{}, err
	}

	url := strings.TrimRight(p.cfg.BaseURL, "/") + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return router.Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return router.Response{}, fmt.Errorf("call %s: %w", url, err)
	}
	defer httpResp.Body.Close()

	if err := interpretStatus(p.cfg.BaseURL, httpResp.StatusCode); err != nil {
		return router.Response{}, err
	}

	var result openAIResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&result); err != nil {
		return router.Response{}, fmt.Errorf("decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return router.Response{}, fmt.Errorf("provider returned empty choices")
	}
	return router.Response{
		Provider: p.cfg.Name,
		Model:    p.cfg.Model,
		Text:     result.Choices[0].Message.Content,
		Usage: router.Usage{
			InputTokens:  result.Usage.PromptTokens,
			OutputTokens: result.Usage.CompletionTokens,
		},
	}, nil
}

// ── Anthropic ────────────────────────────────────────────────────────────────

type anthropicRequest struct {
	Model     string             `json:"model"`
	Messages  []anthropicMessage `json:"messages"`
	MaxTokens int                `json:"max_tokens"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (p *Provider) callAnthropic(ctx context.Context, req router.Request) (router.Response, error) {
	base := p.cfg.BaseURL
	if base == "" {
		base = "https://api.anthropic.com"
	}

	payload := anthropicRequest{
		Model:     p.cfg.Model,
		Messages:  []anthropicMessage{{Role: "user", Content: req.Prompt}},
		MaxTokens: 4096,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return router.Response{}, err
	}

	url := strings.TrimRight(base, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return router.Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return router.Response{}, fmt.Errorf("call %s: %w", url, err)
	}
	defer httpResp.Body.Close()

	if err := interpretStatus(base, httpResp.StatusCode); err != nil {
		return router.Response{}, err
	}

	var result anthropicResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&result); err != nil {
		return router.Response{}, fmt.Errorf("decode response: %w", err)
	}
	if len(result.Content) == 0 {
		return router.Response{}, fmt.Errorf("provider returned empty content")
	}
	return router.Response{
		Provider: p.cfg.Name,
		Model:    p.cfg.Model,
		Text:     result.Content[0].Text,
		Usage: router.Usage{
			InputTokens:  result.Usage.InputTokens,
			OutputTokens: result.Usage.OutputTokens,
		},
	}, nil
}
