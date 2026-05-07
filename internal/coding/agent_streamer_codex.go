package coding

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/jabreeflor/conduit/internal/provider/codex"
	"github.com/jabreeflor/conduit/internal/tools"
)

// CodexAgentStreamer is a Streamer that talks to OpenAI's Codex backend
// (Responses API) using ChatGPT-account credentials. It mirrors the
// Anthropic AgentStreamer's loop but adapts to the Responses API shape:
// conversation is a flat slice of items (messages, function_calls,
// function_call_outputs) replayed each turn.
type CodexAgentStreamer struct {
	Client *codex.Client
	Tools  []tools.Tool
	System string

	// MaxToolIterations bounds the inner tool-dispatch loop within a single
	// user turn so a confused agent can't infinite-loop on tool calls.
	MaxToolIterations int

	threadID string
	input    []codex.Item
}

// NewCodexAgentStreamer returns a streamer ready for the REPL.
func NewCodexAgentStreamer(client *codex.Client, tools []tools.Tool, system string) *CodexAgentStreamer {
	return &CodexAgentStreamer{
		Client:            client,
		Tools:             tools,
		System:            system,
		MaxToolIterations: 12,
		threadID:          uuid.NewString(),
	}
}

// Stream satisfies coding.Streamer. Appends the user prompt to the input
// list, runs the model, dispatches any function_call items against r.Tools,
// loops until the model returns no further tool calls (or iteration cap).
func (a *CodexAgentStreamer) Stream(ctx context.Context, prompt string, onDelta func(string)) (string, string, error) {
	if a.Client == nil {
		return "", "", fmt.Errorf("codex agent: no client configured")
	}

	a.input = append(a.input, codex.Item{
		Type: "message",
		Role: "user",
		Content: []codex.ItemContentBlock{{
			Type: "input_text",
			Text: prompt,
		}},
	})

	codexTools := toCodexTools(a.Tools)
	toolByName := make(map[string]tools.Tool, len(a.Tools))
	for _, t := range a.Tools {
		toolByName[t.Name] = t
	}

	maxIter := a.MaxToolIterations
	if maxIter <= 0 {
		maxIter = 12
	}

	var finalText strings.Builder

	for iter := 0; iter < maxIter; iter++ {
		items, err := a.Client.Stream(ctx, a.System, a.input, codexTools, a.threadID, func(ev codex.Event) {
			if ev.Type == "text_delta" && onDelta != nil {
				onDelta(ev.Text)
			}
		})
		if err != nil {
			return finalText.String(), "error", err
		}

		// Append assistant items so the next request sees the same history
		// the server emitted.
		a.input = append(a.input, items...)

		// Surface text from message items into the REPL's return value.
		for _, it := range items {
			if it.Type == "message" {
				for _, c := range it.Content {
					if c.Type == "output_text" {
						finalText.WriteString(c.Text)
					}
				}
			}
		}

		// Collect function_call items to dispatch.
		var calls []codex.Item
		for _, it := range items {
			if it.Type == "function_call" {
				calls = append(calls, it)
			}
		}
		if len(calls) == 0 {
			return finalText.String(), "completed", nil
		}

		// Run each call and append a function_call_output item per result.
		for _, call := range calls {
			tool, ok := toolByName[call.Name]
			if !ok {
				a.input = append(a.input, codex.Item{
					Type:   "function_call_output",
					CallID: call.CallID,
					Output: fmt.Sprintf("tool %q is not registered", call.Name),
				})
				continue
			}
			res, runErr := tool.Run(ctx, json.RawMessage(call.Arguments))
			out := res.Text
			if runErr != nil {
				out = runErr.Error()
			}
			if res.IsError && out == "" {
				out = "tool reported an error"
			}
			a.input = append(a.input, codex.Item{
				Type:   "function_call_output",
				CallID: call.CallID,
				Output: out,
			})
		}
	}

	return finalText.String(), "max_tool_iterations", nil
}

// toCodexTools converts the internal tools.Tool slice into the Responses API
// function-tool shape. Schema is passed straight through — JSON Schema is
// interchangeable across providers.
func toCodexTools(ts []tools.Tool) []codex.Tool {
	out := make([]codex.Tool, 0, len(ts))
	for _, t := range ts {
		schema := t.Schema
		if schema == nil {
			schema = map[string]any{"type": "object"}
		}
		out = append(out, codex.Tool{
			Type:        "function",
			Name:        t.Name,
			Description: t.Description,
			Parameters:  schema,
		})
	}
	return out
}
