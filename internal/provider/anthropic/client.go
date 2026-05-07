// Package anthropic is a streaming Messages API client with tool support.
//
// Scope is intentionally narrow: it covers what the coding REPL needs —
// streaming text deltas, tool_use blocks, and the tool_result roundtrip — and
// nothing else. A separate non-streaming client lives in internal/endpoint for
// the router's one-shot inference path.
package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL    = "https://api.anthropic.com"
	apiVersion        = "2023-06-01"
	defaultMaxTokens  = 4096
	defaultHTTPMaxAge = 5 * time.Minute
)

// Client posts to /v1/messages with stream=true.
type Client struct {
	APIKey     string
	Model      string
	BaseURL    string // optional; defaults to https://api.anthropic.com
	MaxTokens  int    // optional; defaults to 4096
	HTTPClient *http.Client
}

// New returns a Client with sane defaults; APIKey and Model must be set.
func New(apiKey, model string) *Client {
	return &Client{
		APIKey:    apiKey,
		Model:     model,
		MaxTokens: defaultMaxTokens,
		HTTPClient: &http.Client{
			Timeout: defaultHTTPMaxAge,
		},
	}
}

// Tool is a tool definition surfaced to the model. InputSchema must be a valid
// JSON Schema object describing the tool's input.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// ContentBlock is the unified shape for both request messages and response
// content. The Type field discriminates which fields are populated.
//
//   - "text":         Text
//   - "tool_use":     ID, Name, Input
//   - "tool_result":  ToolUseID, Content, IsError
type ContentBlock struct {
	Type string `json:"type"`

	// text
	Text string `json:"text,omitempty"`

	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// Message is one turn in the conversation. Content may carry multiple blocks
// (e.g. assistant turns mixing text + tool_use, or user turns carrying
// multiple tool_result blocks).
type Message struct {
	Role    string         `json:"role"` // "user" | "assistant"
	Content []ContentBlock `json:"content"`
}

// Event is what Stream yields to its callback as the SSE response decodes.
type Event struct {
	Type       string // "text_delta" | "tool_use_start" | "stop"
	Text       string // text_delta payload
	ToolUse    *ContentBlock
	StopReason string // populated on Type=="stop"
}

// streamRequest is the wire shape for POST /v1/messages.
type streamRequest struct {
	Model     string    `json:"model"`
	System    string    `json:"system,omitempty"`
	Messages  []Message `json:"messages"`
	MaxTokens int       `json:"max_tokens"`
	Stream    bool      `json:"stream"`
	Tools     []Tool    `json:"tools,omitempty"`
}

// Stream POSTs the request and decodes the SSE response, calling onEvent for
// each text delta and tool_use block. It returns the assembled assistant
// content (text + tool_use blocks in order) and the model's stop reason.
func (c *Client) Stream(
	ctx context.Context,
	system string,
	messages []Message,
	tools []Tool,
	onEvent func(Event),
) ([]ContentBlock, string, error) {
	if c.APIKey == "" {
		return nil, "", fmt.Errorf("anthropic: APIKey is required (set ANTHROPIC_API_KEY)")
	}
	if c.Model == "" {
		return nil, "", fmt.Errorf("anthropic: Model is required")
	}

	maxTokens := c.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	body, err := json.Marshal(streamRequest{
		Model:     c.Model,
		System:    system,
		Messages:  messages,
		MaxTokens: maxTokens,
		Stream:    true,
		Tools:     tools,
	})
	if err != nil {
		return nil, "", fmt.Errorf("anthropic: marshal request: %w", err)
	}

	base := c.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	url := strings.TrimRight(base, "/") + "/v1/messages"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("anthropic: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", apiVersion)

	httpc := c.HTTPClient
	if httpc == nil {
		httpc = http.DefaultClient
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("anthropic: POST %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := readAll(resp.Body)
		return nil, "", fmt.Errorf("anthropic: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(errBody))
	}

	return parseSSE(resp.Body, onEvent)
}

// parseSSE consumes the SSE stream until message_stop, accumulating
// content blocks (text + tool_use) and returning them in arrival order.
func parseSSE(r interface {
	Read(p []byte) (int, error)
}, onEvent func(Event)) ([]ContentBlock, string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	type pending struct {
		block     ContentBlock
		jsonChunk strings.Builder
	}
	blocks := map[int]*pending{}
	order := []int{}

	stopReason := ""

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}

		var env struct {
			Type         string          `json:"type"`
			Index        int             `json:"index"`
			ContentBlock json.RawMessage `json:"content_block,omitempty"`
			Delta        json.RawMessage `json:"delta,omitempty"`
		}
		if err := json.Unmarshal([]byte(payload), &env); err != nil {
			continue
		}

		switch env.Type {
		case "content_block_start":
			var cb ContentBlock
			if err := json.Unmarshal(env.ContentBlock, &cb); err == nil {
				blocks[env.Index] = &pending{block: cb}
				order = append(order, env.Index)
				if cb.Type == "tool_use" && onEvent != nil {
					b := cb
					onEvent(Event{Type: "tool_use_start", ToolUse: &b})
				}
			}
		case "content_block_delta":
			p, ok := blocks[env.Index]
			if !ok {
				continue
			}
			var d struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
			}
			if err := json.Unmarshal(env.Delta, &d); err != nil {
				continue
			}
			switch d.Type {
			case "text_delta":
				p.block.Text += d.Text
				if onEvent != nil {
					onEvent(Event{Type: "text_delta", Text: d.Text})
				}
			case "input_json_delta":
				p.jsonChunk.WriteString(d.PartialJSON)
			}
		case "content_block_stop":
			p, ok := blocks[env.Index]
			if !ok {
				continue
			}
			if p.block.Type == "tool_use" && p.jsonChunk.Len() > 0 {
				p.block.Input = json.RawMessage(p.jsonChunk.String())
			}
		case "message_delta":
			var d struct {
				StopReason string `json:"stop_reason"`
			}
			if err := json.Unmarshal(env.Delta, &d); err == nil && d.StopReason != "" {
				stopReason = d.StopReason
			}
		case "message_stop":
			if onEvent != nil {
				onEvent(Event{Type: "stop", StopReason: stopReason})
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, "", fmt.Errorf("anthropic: read SSE: %w", err)
	}

	out := make([]ContentBlock, 0, len(order))
	for _, idx := range order {
		out = append(out, blocks[idx].block)
	}
	return out, stopReason, nil
}

func readAll(r interface {
	Read(p []byte) (int, error)
}) (string, error) {
	var buf bytes.Buffer
	tmp := make([]byte, 4096)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if err != nil {
			if err.Error() == "EOF" {
				return buf.String(), nil
			}
			return buf.String(), err
		}
	}
}
