package hooks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func hookScript(t *testing.T, output string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "hook.sh")
	content := fmt.Sprintf("#!/bin/sh\nprintf '%%s' '%s'\n", output)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write hook script: %v", err)
	}
	return path
}

func TestAllSevenEventConstantsDefined(t *testing.T) {
	events := []Event{
		EventOnSessionStart,
		EventOnSessionEnd,
		EventPreLLMCall,
		EventPostLLMCall,
		EventPreToolCall,
		EventPostToolCall,
		EventOnMemoryWrite,
	}
	if len(events) != 7 {
		t.Fatalf("want 7 event constants, got %d", len(events))
	}
}

func TestDispatcherAllowByDefault(t *testing.T) {
	d, _ := New(Config{})
	out := d.Dispatch(context.Background(), Input{Event: EventOnSessionStart, SessionID: "s1"})
	if out.Decision != DecisionAllow {
		t.Fatalf("Decision = %q, want allow", out.Decision)
	}
}

func TestNilDispatcherReturnsAllow(t *testing.T) {
	var d *Dispatcher
	out := d.Dispatch(context.Background(), Input{Event: EventPreToolCall, SessionID: "s1"})
	if out.Decision != DecisionAllow {
		t.Fatalf("nil dispatcher: Decision = %q, want allow", out.Decision)
	}
}

func TestDispatcherFiresMatchingEvent(t *testing.T) {
	script := hookScript(t, `{"decision":"block","reason":"test"}`)
	d, err := New(Config{Hooks: []HookDef{{Event: EventPreToolCall, Command: script}}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	out := d.Dispatch(context.Background(), Input{Event: EventPreToolCall, SessionID: "s1", ToolName: "exec"})
	if out.Decision != DecisionBlock {
		t.Fatalf("Decision = %q, want block", out.Decision)
	}
	if out.Reason != "test" {
		t.Fatalf("Reason = %q, want test", out.Reason)
	}
}

func TestDispatcherSkipsNonMatchingEvent(t *testing.T) {
	script := hookScript(t, `{"decision":"block"}`)
	d, err := New(Config{Hooks: []HookDef{{Event: EventPreToolCall, Command: script}}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	out := d.Dispatch(context.Background(), Input{Event: EventPostToolCall, SessionID: "s1", ToolName: "exec"})
	if out.Decision != DecisionAllow {
		t.Fatalf("Decision = %q, want allow (wrong event skipped)", out.Decision)
	}
}

func TestDispatcherRegexMatcherFiltersToolName(t *testing.T) {
	script := hookScript(t, `{"decision":"block"}`)
	d, err := New(Config{Hooks: []HookDef{{
		Event: EventPreToolCall, Command: script, Matcher: "^exec$",
	}}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	blocked := d.Dispatch(context.Background(), Input{Event: EventPreToolCall, SessionID: "s1", ToolName: "exec"})
	if blocked.Decision != DecisionBlock {
		t.Fatalf("Decision = %q, want block for matching tool", blocked.Decision)
	}

	allowed := d.Dispatch(context.Background(), Input{Event: EventPreToolCall, SessionID: "s1", ToolName: "read_file"})
	if allowed.Decision != DecisionAllow {
		t.Fatalf("Decision = %q, want allow for non-matching tool", allowed.Decision)
	}
}

func TestDispatcherRegexMatcherFiltersSessionID(t *testing.T) {
	script := hookScript(t, `{"decision":"block"}`)
	d, err := New(Config{Hooks: []HookDef{{
		Event: EventOnSessionStart, Command: script, Matcher: "^prod-",
	}}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	blocked := d.Dispatch(context.Background(), Input{Event: EventOnSessionStart, SessionID: "prod-123"})
	if blocked.Decision != DecisionBlock {
		t.Fatalf("Decision = %q, want block for matching session", blocked.Decision)
	}

	allowed := d.Dispatch(context.Background(), Input{Event: EventOnSessionStart, SessionID: "dev-456"})
	if allowed.Decision != DecisionAllow {
		t.Fatalf("Decision = %q, want allow for non-matching session", allowed.Decision)
	}
}

func TestDispatcherInjectDecision(t *testing.T) {
	script := hookScript(t, `{"decision":"inject","context":"extra context"}`)
	d, err := New(Config{Hooks: []HookDef{{Event: EventPreLLMCall, Command: script}}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	out := d.Dispatch(context.Background(), Input{Event: EventPreLLMCall, SessionID: "s1"})
	if out.Decision != DecisionInject {
		t.Fatalf("Decision = %q, want inject", out.Decision)
	}
	if out.Context != "extra context" {
		t.Fatalf("Context = %q, want 'extra context'", out.Context)
	}
}

func TestCrashingHookDefaultsToAllow(t *testing.T) {
	d, err := New(Config{Hooks: []HookDef{{
		Event: EventPostLLMCall, Command: "exit 1",
	}}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	out := d.Dispatch(context.Background(), Input{Event: EventPostLLMCall, SessionID: "s1"})
	if out.Decision != DecisionAllow {
		t.Fatalf("Decision = %q, want allow on crash", out.Decision)
	}
}

func TestTimeoutDefaultsToAllow(t *testing.T) {
	d, err := New(Config{Hooks: []HookDef{{
		Event:   EventOnMemoryWrite,
		Command: "sleep 10",
		Timeout: 1,
	}}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	start := time.Now()
	out := d.Dispatch(context.Background(), Input{Event: EventOnMemoryWrite, SessionID: "s1"})
	elapsed := time.Since(start)

	if out.Decision != DecisionAllow {
		t.Fatalf("Decision = %q, want allow on timeout", out.Decision)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("timeout not enforced: took %v", elapsed)
	}
}

func TestPerEventTimeoutOverridesDefault(t *testing.T) {
	d, err := New(Config{Hooks: []HookDef{{
		Event:   EventOnSessionEnd,
		Command: "sleep 10",
		Timeout: 1,
	}}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	start := time.Now()
	d.Dispatch(context.Background(), Input{Event: EventOnSessionEnd, SessionID: "s1"})
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("per-event timeout not enforced: took %v", elapsed)
	}
}

func TestInvalidRegexReturnsError(t *testing.T) {
	_, err := New(Config{Hooks: []HookDef{{
		Event: EventPreToolCall, Command: "echo", Matcher: "[invalid",
	}}})
	if err == nil {
		t.Fatal("want error for invalid regex, got nil")
	}
}

func TestFirstBlockStopsDispatch(t *testing.T) {
	blockScript := hookScript(t, `{"decision":"block","reason":"first"}`)
	allowScript := hookScript(t, `{"decision":"allow"}`)

	d, err := New(Config{Hooks: []HookDef{
		{Event: EventPreToolCall, Command: blockScript},
		{Event: EventPreToolCall, Command: allowScript},
	}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	out := d.Dispatch(context.Background(), Input{Event: EventPreToolCall, SessionID: "s1", ToolName: "exec"})
	if out.Decision != DecisionBlock {
		t.Fatalf("Decision = %q, want block (first hook should stop dispatch)", out.Decision)
	}
	if out.Reason != "first" {
		t.Fatalf("Reason = %q, want 'first'", out.Reason)
	}
}
