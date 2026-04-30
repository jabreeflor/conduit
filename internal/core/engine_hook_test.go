package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jabreeflor/conduit/internal/hooks"
)

func scriptDispatcher(t *testing.T, event hooks.Event, output string) *hooks.Dispatcher {
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

func TestEngineFireSessionStartReturnsHookDecision(t *testing.T) {
	d := scriptDispatcher(t, hooks.EventOnSessionStart, `{"decision":"block","reason":"session blocked"}`)
	engine := New("test").WithHooks(d)

	out := engine.FireSessionStart(context.Background())

	if out.Decision != hooks.DecisionBlock {
		t.Fatalf("Decision = %q, want block", out.Decision)
	}
	if out.Reason != "session blocked" {
		t.Fatalf("Reason = %q, want 'session blocked'", out.Reason)
	}
}

func TestEngineFireSessionEndReturnsHookDecision(t *testing.T) {
	d := scriptDispatcher(t, hooks.EventOnSessionEnd, `{"decision":"allow"}`)
	engine := New("test").WithHooks(d)

	out := engine.FireSessionEnd(context.Background())

	if out.Decision != hooks.DecisionAllow {
		t.Fatalf("Decision = %q, want allow", out.Decision)
	}
}

func TestEngineFirePreLLMCallReturnsHookDecision(t *testing.T) {
	d := scriptDispatcher(t, hooks.EventPreLLMCall, `{"decision":"block","reason":"cost limit"}`)
	engine := New("test").WithHooks(d)

	out := engine.FirePreLLMCall(context.Background())

	if out.Decision != hooks.DecisionBlock {
		t.Fatalf("Decision = %q, want block", out.Decision)
	}
}

func TestEngineFirePostLLMCallReturnsHookDecision(t *testing.T) {
	d := scriptDispatcher(t, hooks.EventPostLLMCall, `{"decision":"allow"}`)
	engine := New("test").WithHooks(d)

	out := engine.FirePostLLMCall(context.Background())

	if out.Decision != hooks.DecisionAllow {
		t.Fatalf("Decision = %q, want allow", out.Decision)
	}
}

func TestEngineFireMemoryWriteReturnsHookDecision(t *testing.T) {
	d := scriptDispatcher(t, hooks.EventOnMemoryWrite, `{"decision":"block","reason":"memory audit failed"}`)
	engine := New("test").WithHooks(d)

	out := engine.FireMemoryWrite(context.Background(), map[string]any{"key": "value"})

	if out.Decision != hooks.DecisionBlock {
		t.Fatalf("Decision = %q, want block", out.Decision)
	}
}

func TestEngineWithNoHooksAllowsAll(t *testing.T) {
	engine := New("test") // no WithHooks call

	for _, fn := range []func() hooks.Output{
		func() hooks.Output { return engine.FireSessionStart(context.Background()) },
		func() hooks.Output { return engine.FireSessionEnd(context.Background()) },
		func() hooks.Output { return engine.FirePreLLMCall(context.Background()) },
		func() hooks.Output { return engine.FirePostLLMCall(context.Background()) },
		func() hooks.Output { return engine.FireMemoryWrite(context.Background(), nil) },
	} {
		out := fn()
		if out.Decision != hooks.DecisionAllow {
			t.Fatalf("Decision = %q, want allow (nil dispatcher)", out.Decision)
		}
	}
}
