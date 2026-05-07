package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"

	"github.com/google/uuid"
)

const (
	chatGPTBaseURL = "https://chatgpt.com/backend-api/codex"
	apiKeyBaseURL  = "https://api.openai.com/v1"

	// originator identifies the client to the backend; matches DEFAULT_ORIGINATOR
	// in codex-rs/login/src/auth/default_client.rs. The backend treats certain
	// originator values as first-party — we register conduit as one but fall
	// through to codex_cli_rs since our shape is closer to the CLI than to the
	// other surfaces.
	originator = "codex_cli_rs"
)

// Tool is the Responses API function-tool definition.
type Tool struct {
	Type        string         `json:"type"`        // always "function"
	Name        string         `json:"name"`        // function name
	Description string         `json:"description"` // human description
	Parameters  map[string]any `json:"parameters"`  // JSON Schema
}

// Item is one entry in the Responses API conversation array. The Type field
// discriminates which other fields are populated. We model only the items we
// produce or consume (message, function_call, function_call_output) and pass
// any unknown items through opaquely so we don't lose server-side state.
//
// Message: Role + Content (Content is an array of {type:"input_text"|"output_text", text})
// FunctionCall: CallID + Name + Arguments (raw JSON string)
// FunctionCallOutput: CallID + Output (the tool result)
type Item struct {
	Type string `json:"type"`

	// message
	Role    string             `json:"role,omitempty"`
	Content []ItemContentBlock `json:"content,omitempty"`

	// function_call
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`

	// function_call_output
	Output string `json:"output,omitempty"`

	// reasoning / unknown items — kept opaque so we can echo back unchanged.
	Raw json.RawMessage `json:"-"`
}

// ItemContentBlock is one content entry inside a message item. Responses uses
// "input_text" for user messages and "output_text" for assistant messages.
type ItemContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// MarshalJSON re-emits the original payload when Raw is set, so server-issued
// items (e.g. reasoning blocks we don't fully model) survive a round trip.
func (i Item) MarshalJSON() ([]byte, error) {
	if len(i.Raw) > 0 {
		return i.Raw, nil
	}
	type alias Item
	return json.Marshal(alias(i))
}

// Event is what Stream yields to its callback as the SSE response decodes.
type Event struct {
	Type         string // "text_delta" | "function_call_done" | "completed" | "failed"
	Text         string // for text_delta
	FunctionCall *Item  // for function_call_done — fully populated
	Error        string // for failed
}

// Client posts to the Codex backend's POST /responses with stream=true.
type Client struct {
	Auth       *Auth
	Model      string
	HTTPClient *http.Client

	// BaseURL overrides the default backend URL — useful for tests.
	BaseURL string
}

// NewClient builds a Client. Auth must already have a usable token.
func NewClient(auth *Auth, model string) *Client {
	return &Client{
		Auth:       auth,
		Model:      model,
		HTTPClient: &http.Client{Timeout: 5 * 60_000_000_000}, // 5 min, matches Anthropic
	}
}

// requestBody is the wire shape for POST /responses.
type requestBody struct {
	Model             string `json:"model"`
	Instructions      string `json:"instructions,omitempty"`
	Input             []Item `json:"input"`
	Tools             []Tool `json:"tools,omitempty"`
	ToolChoice        string `json:"tool_choice,omitempty"`
	ParallelToolCalls bool   `json:"parallel_tool_calls"`
	Stream            bool   `json:"stream"`
	Store             bool   `json:"store"`
	PromptCacheKey    string `json:"prompt_cache_key,omitempty"`
}

// Stream POSTs the Responses request and decodes the SSE response, calling
// onEvent for each text delta, finished function_call, and the final
// completed/failed event. It returns the items the assistant emitted (text
// messages + function_call items) so the caller can append them to the
// conversation history for the next turn.
func (c *Client) Stream(
	ctx context.Context,
	system string,
	input []Item,
	tools []Tool,
	threadID string,
	onEvent func(Event),
) ([]Item, error) {
	if c.Auth == nil {
		return nil, fmt.Errorf("codex: no Auth configured")
	}
	if c.Model == "" {
		return nil, fmt.Errorf("codex: Model is required")
	}
	token, err := c.Auth.AccessToken(ctx)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(requestBody{
		Model:             c.Model,
		Instructions:      system,
		Input:             input,
		Tools:             tools,
		ToolChoice:        "auto",
		ParallelToolCalls: true,
		Stream:            true,
		Store:             false,
		PromptCacheKey:    threadID,
	})
	if err != nil {
		return nil, fmt.Errorf("codex: marshal request: %w", err)
	}

	base := c.BaseURL
	if base == "" {
		if c.Auth.AuthMode() == "ApiKey" {
			base = apiKeyBaseURL
		} else {
			base = chatGPTBaseURL
		}
	}
	url := strings.TrimRight(base, "/") + "/responses"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("codex: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	if accID := c.Auth.AccountID(); accID != "" {
		req.Header.Set("ChatGPT-Account-ID", accID)
	}
	req.Header.Set("originator", originator)
	req.Header.Set("session_id", uuidString())
	if threadID != "" {
		req.Header.Set("x-client-request-id", threadID)
	}
	req.Header.Set("User-Agent", userAgent())

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("codex: POST %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("codex: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}

	return parseResponsesSSE(resp.Body, onEvent)
}

// pending tracks an in-flight item while its argument or text deltas stream
// in. Hoisted to package scope so collectItems can reference the type.
type pending struct {
	item    Item
	argsBuf strings.Builder
	textBuf strings.Builder
}

// parseResponsesSSE consumes the SSE stream until response.completed or
// response.failed, accumulating the assistant items in arrival order. Tool
// call argument deltas are buffered and only surfaced once the corresponding
// output_item.done arrives.
func parseResponsesSSE(r io.Reader, onEvent func(Event)) ([]Item, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	pendings := map[string]*pending{}
	var ordered []*pending

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
			Type     string          `json:"type"`
			ItemID   string          `json:"item_id"`
			Item     json.RawMessage `json:"item,omitempty"`
			Delta    string          `json:"delta,omitempty"`
			Response struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error,omitempty"`
			} `json:"response,omitempty"`
		}
		if err := json.Unmarshal([]byte(payload), &env); err != nil {
			continue
		}

		switch env.Type {
		case "response.output_item.added":
			var it Item
			if err := json.Unmarshal(env.Item, &it); err != nil {
				continue
			}
			it.Raw = append(json.RawMessage{}, env.Item...)
			id := itemIDFromRaw(env.Item)
			p := &pending{item: it}
			pendings[id] = p
			ordered = append(ordered, p)

		case "response.output_text.delta":
			p := pendings[env.ItemID]
			if p == nil {
				continue
			}
			p.textBuf.WriteString(env.Delta)
			if onEvent != nil {
				onEvent(Event{Type: "text_delta", Text: env.Delta})
			}

		case "response.function_call_arguments.delta", "response.custom_tool_call_input.delta":
			p := pendings[env.ItemID]
			if p == nil {
				continue
			}
			p.argsBuf.WriteString(env.Delta)

		case "response.output_item.done":
			var it Item
			if err := json.Unmarshal(env.Item, &it); err != nil {
				continue
			}
			id := itemIDFromRaw(env.Item)
			p := pendings[id]
			if p == nil {
				p = &pending{}
				ordered = append(ordered, p)
			}
			it.Raw = append(json.RawMessage{}, env.Item...)
			// If we accumulated arg deltas the server didn't include in the
			// final item, prefer the buffered version (defensive).
			if it.Type == "function_call" && it.Arguments == "" && p.argsBuf.Len() > 0 {
				it.Arguments = p.argsBuf.String()
			}
			// Same for text content.
			if it.Type == "message" && len(it.Content) == 0 && p.textBuf.Len() > 0 {
				it.Content = []ItemContentBlock{{Type: "output_text", Text: p.textBuf.String()}}
			}
			p.item = it
			if it.Type == "function_call" && onEvent != nil {
				cp := it
				onEvent(Event{Type: "function_call_done", FunctionCall: &cp})
			}

		case "response.completed":
			if onEvent != nil {
				onEvent(Event{Type: "completed"})
			}
		case "response.failed":
			msg := env.Response.Error.Message
			if msg == "" {
				msg = "response.failed"
			}
			if onEvent != nil {
				onEvent(Event{Type: "failed", Error: msg})
			}
			return collectItems(ordered), fmt.Errorf("codex: %s", msg)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("codex: read SSE: %w", err)
	}
	return collectItems(ordered), nil
}

func collectItems(p []*pending) []Item {
	out := make([]Item, 0, len(p))
	for _, x := range p {
		out = append(out, x.item)
	}
	return out
}

// itemIDFromRaw pulls the "id" field out of an item payload without fully
// reparsing it. Returns "" if the field isn't present.
func itemIDFromRaw(raw json.RawMessage) string {
	var probe struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &probe)
	return probe.ID
}

func uuidString() string {
	return uuid.NewString()
}

func userAgent() string {
	return fmt.Sprintf("conduit-codex/0.1 (%s %s)", runtime.GOOS, runtime.GOARCH)
}
