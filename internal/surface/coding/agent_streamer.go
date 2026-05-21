package coding

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jabreeflor/conduit/internal/provider/anthropic"
	"github.com/jabreeflor/conduit/internal/tools"
)

// AgentStreamer is a Streamer that talks to Anthropic with tools, dispatching
// tool_use blocks back to the registered tools.Tool runners and feeding the
// results into the next provider call until the model stops.
//
// It maintains its own conversation history across Stream() calls so multi-turn
// user-assistant dialog accumulates the way the API expects. The REPL's own
// session.Append call records the same exchange in the journal — the two
// histories are intentionally parallel.
type AgentStreamer struct {
	Client *anthropic.Client
	Tools  []tools.Tool
	System string

	// MaxToolIterations bounds the inner tool-dispatch loop within a single
	// user turn so a confused agent can't infinite-loop on tool calls. The
	// REPL's own MaxAutoContinue caps continuation across turns.
	MaxToolIterations int

	history []anthropic.Message
}

// NewAgentStreamer returns a streamer ready for the REPL.
func NewAgentStreamer(client *anthropic.Client, tools []tools.Tool, system string) *AgentStreamer {
	return &AgentStreamer{
		Client:            client,
		Tools:             tools,
		System:            system,
		MaxToolIterations: 12,
	}
}

// Stream satisfies the coding.Streamer interface. It appends the user prompt
// to the conversation history, runs the model, dispatches any tool_use blocks
// against r.Tools, and continues until the model returns stop_reason != "tool_use"
// (or the iteration cap is hit).
func (a *AgentStreamer) Stream(ctx context.Context, prompt string, onDelta func(string)) (string, string, error) {
	if a.Client == nil {
		return "", "", fmt.Errorf("agent: no provider client configured")
	}

	a.history = append(a.history, anthropic.Message{
		Role:    "user",
		Content: []anthropic.ContentBlock{{Type: "text", Text: prompt}},
	})

	toolDefs := toAnthropicTools(a.Tools)
	toolByName := make(map[string]tools.Tool, len(a.Tools))
	for _, t := range a.Tools {
		toolByName[t.Name] = t
	}

	maxIter := a.MaxToolIterations
	if maxIter <= 0 {
		maxIter = 12
	}

	var finalText strings.Builder
	stopReason := "end_turn"

	for iter := 0; iter < maxIter; iter++ {
		blocks, stop, err := a.Client.Stream(ctx, a.System, a.history, toolDefs, func(ev anthropic.Event) {
			if ev.Type == "text_delta" && onDelta != nil {
				onDelta(ev.Text)
			}
		})
		if err != nil {
			return finalText.String(), "error", err
		}

		// Append the assistant turn to history exactly as received so the
		// model sees its own tool_use ids on the next call.
		a.history = append(a.history, anthropic.Message{
			Role:    "assistant",
			Content: blocks,
		})

		// Capture text for the REPL's return value.
		for _, b := range blocks {
			if b.Type == "text" {
				finalText.WriteString(b.Text)
			}
		}

		// If the model didn't call tools, we're done with this user turn.
		if stop != "tool_use" {
			stopReason = stop
			break
		}

		// Dispatch each tool_use block and pack the results into a single
		// user message — this is the shape Anthropic requires.
		var results []anthropic.ContentBlock
		for _, b := range blocks {
			if b.Type != "tool_use" {
				continue
			}
			tool, ok := toolByName[b.Name]
			if !ok {
				results = append(results, anthropic.ContentBlock{
					Type:      "tool_result",
					ToolUseID: b.ID,
					Content:   fmt.Sprintf("tool %q is not registered", b.Name),
					IsError:   true,
				})
				continue
			}
			res, runErr := tool.Run(ctx, b.Input)
			content := res.Text
			isError := res.IsError
			if runErr != nil {
				content = runErr.Error()
				isError = true
			}
			results = append(results, anthropic.ContentBlock{
				Type:      "tool_result",
				ToolUseID: b.ID,
				Content:   content,
				IsError:   isError,
			})
		}
		a.history = append(a.history, anthropic.Message{
			Role:    "user",
			Content: results,
		})
		stopReason = "tool_use"
	}

	return finalText.String(), stopReason, nil
}

// toAnthropicTools converts the internal tools.Tool slice into the schema
// shape Anthropic expects. Schema is passed straight through — the tools
// package already stores it as JSON-Schema-compatible map[string]any.
func toAnthropicTools(ts []tools.Tool) []anthropic.Tool {
	out := make([]anthropic.Tool, 0, len(ts))
	for _, t := range ts {
		schema := t.Schema
		if schema == nil {
			schema = map[string]any{"type": "object"}
		}
		out = append(out, anthropic.Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		})
	}
	return out
}

// inputAsString is a convenience for tests/debug — turns a tool_use Input
// blob into a stable string.
func inputAsString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	return string(raw)
}
