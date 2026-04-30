package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jabreeflor/conduit/internal/hooks"
)

func hookDispatcher(t *testing.T, event hooks.Event, output string) *hooks.Dispatcher {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "hook.sh")
	content := fmt.Sprintf("#!/bin/sh\nprintf '%%s' '%s'\n", output)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write hook script: %v", err)
	}
	d, err := hooks.New(hooks.Config{Hooks: []hooks.HookDef{
		{Event: event, Command: path},
	}})
	if err != nil {
		t.Fatalf("hooks.New: %v", err)
	}
	return d
}

func echoTool() Tool {
	return Tool{
		Name: "echo",
		Run: func(_ context.Context, input json.RawMessage) (Result, error) {
			return Result{Text: string(input)}, nil
		},
	}
}

func TestPreToolCallHookBlocksExecution(t *testing.T) {
	d := hookDispatcher(t, hooks.EventPreToolCall, `{"decision":"block","reason":"audit deny"}`)
	pipeline := NewPipeline([]Tool{echoTool()}, PolicyConfig{}).
		WithHooks(d, "sess-1", "/tmp")

	_, _, err := pipeline.Execute(context.Background(), Call{ToolName: "echo", Input: map[string]any{"msg": "hi"}}, AgentOverrides{})

	if err == nil {
		t.Fatal("want error from blocked pre_tool_call hook, got nil")
	}
	if err.Error() != "pre_tool_call hook blocked: audit deny" {
		t.Fatalf("err = %q, want 'pre_tool_call hook blocked: audit deny'", err)
	}
}

func TestPreToolCallHookAllowsExecution(t *testing.T) {
	d := hookDispatcher(t, hooks.EventPreToolCall, `{"decision":"allow"}`)
	pipeline := NewPipeline([]Tool{echoTool()}, PolicyConfig{}).
		WithHooks(d, "sess-1", "/tmp")

	result, _, err := pipeline.Execute(context.Background(), Call{ToolName: "echo", Input: map[string]any{"msg": "hi"}}, AgentOverrides{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text == "" {
		t.Fatal("want non-empty result after allowed hook")
	}
}

func TestPostToolCallHookFiresAfterExecution(t *testing.T) {
	dir := t.TempDir()
	flagFile := filepath.Join(dir, "fired")
	script := filepath.Join(dir, "hook.sh")
	content := fmt.Sprintf("#!/bin/sh\ntouch %s\nprintf '{\"decision\":\"allow\"}'\n", flagFile)
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write hook script: %v", err)
	}

	d, _ := hooks.New(hooks.Config{Hooks: []hooks.HookDef{
		{Event: hooks.EventPostToolCall, Command: script},
	}})
	pipeline := NewPipeline([]Tool{echoTool()}, PolicyConfig{}).
		WithHooks(d, "sess-1", "/tmp")

	_, _, err := pipeline.Execute(context.Background(), Call{ToolName: "echo", Input: map[string]any{}}, AgentOverrides{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, statErr := os.Stat(flagFile); statErr != nil {
		t.Fatal("post_tool_call hook did not fire after execution")
	}
}

func TestPreToolCallHookRegexMatchesToolName(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "hook.sh")
	os.WriteFile(script, []byte("#!/bin/sh\nprintf '{\"decision\":\"block\"}'\n"), 0o755)

	d, _ := hooks.New(hooks.Config{Hooks: []hooks.HookDef{
		{Event: hooks.EventPreToolCall, Command: script, Matcher: "^exec$"},
	}})
	pipeline := NewPipeline([]Tool{echoTool()}, PolicyConfig{}).
		WithHooks(d, "sess-1", "/tmp")

	// "echo" does not match "^exec$" → hook skipped → execution succeeds
	_, _, err := pipeline.Execute(context.Background(), Call{ToolName: "echo", Input: map[string]any{}}, AgentOverrides{})
	if err != nil {
		t.Fatalf("unexpected error for non-matching tool: %v", err)
	}
}

func TestPipelineWithNoHooksExecutesNormally(t *testing.T) {
	pipeline := NewPipeline([]Tool{echoTool()}, PolicyConfig{})

	result, _, err := pipeline.Execute(context.Background(), Call{ToolName: "echo", Input: map[string]any{"x": 1}}, AgentOverrides{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text == "" {
		t.Fatal("want non-empty result")
	}
}
