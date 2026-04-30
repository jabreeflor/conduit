package hooks_test

import (
	"context"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/hooks"
)

func TestFire_NoHooks(t *testing.T) {
	m := hooks.New(hooks.Config{}, "sess-1")
	out, err := m.Fire(context.Background(), hooks.EventPreToolCall, hooks.HookInput{ToolName: "read_file"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Decision != hooks.DecisionAllow {
		t.Fatalf("expected allow, got %q", out.Decision)
	}
}

func TestFire_AllowDecision(t *testing.T) {
	cfg := hooks.Config{Hooks: []hooks.HookConfig{{
		Event:   "pre_tool_call",
		Command: `echo '{"decision":"allow"}'`,
		Timeout: 2,
	}}}
	m := hooks.New(cfg, "sess-2")
	out, err := m.Fire(context.Background(), hooks.EventPreToolCall, hooks.HookInput{ToolName: "read_file"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Decision != hooks.DecisionAllow {
		t.Fatalf("expected allow, got %q", out.Decision)
	}
}

func TestFire_BlockDecision(t *testing.T) {
	cfg := hooks.Config{Hooks: []hooks.HookConfig{{
		Event:   "pre_tool_call",
		Command: `echo '{"decision":"block","reason":"not allowed"}'`,
		Timeout: 2,
	}}}
	m := hooks.New(cfg, "sess-3")
	out, err := m.Fire(context.Background(), hooks.EventPreToolCall, hooks.HookInput{ToolName: "write_file"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Decision != hooks.DecisionBlock {
		t.Fatalf("expected block, got %q", out.Decision)
	}
	if out.Reason != "not allowed" {
		t.Fatalf("expected reason %q, got %q", "not allowed", out.Reason)
	}
}

func TestFire_InjectDecision(t *testing.T) {
	cfg := hooks.Config{Hooks: []hooks.HookConfig{{
		Event:   "pre_tool_call",
		Command: `echo '{"decision":"inject","context":"extra context"}'`,
		Timeout: 2,
	}}}
	m := hooks.New(cfg, "sess-4")
	out, err := m.Fire(context.Background(), hooks.EventPreToolCall, hooks.HookInput{ToolName: "search"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Decision != hooks.DecisionInject {
		t.Fatalf("expected inject, got %q", out.Decision)
	}
	if out.Context != "extra context" {
		t.Fatalf("expected context %q, got %q", "extra context", out.Context)
	}
}

func TestFire_BlockBeatsInject(t *testing.T) {
	cfg := hooks.Config{Hooks: []hooks.HookConfig{
		{Event: "pre_tool_call", Command: `echo '{"decision":"inject","context":"ctx"}'`, Timeout: 2},
		{Event: "pre_tool_call", Command: `echo '{"decision":"block","reason":"denied"}'`, Timeout: 2},
	}}
	m := hooks.New(cfg, "sess-5")
	out, err := m.Fire(context.Background(), hooks.EventPreToolCall, hooks.HookInput{ToolName: "write_file"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Decision != hooks.DecisionBlock {
		t.Fatalf("expected block to win, got %q", out.Decision)
	}
}

func TestFire_CrashIsFailSafe(t *testing.T) {
	cfg := hooks.Config{Hooks: []hooks.HookConfig{{
		Event:   "pre_tool_call",
		Command: `exit 1`,
		Timeout: 2,
	}}}
	m := hooks.New(cfg, "sess-6")
	out, err := m.Fire(context.Background(), hooks.EventPreToolCall, hooks.HookInput{ToolName: "read_file"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Decision != hooks.DecisionAllow {
		t.Fatalf("crash should default to allow, got %q", out.Decision)
	}
}

func TestFire_TimeoutIsFailSafe(t *testing.T) {
	cfg := hooks.Config{Hooks: []hooks.HookConfig{{
		Event:   "pre_tool_call",
		Command: `sleep 10 && echo '{"decision":"block"}'`,
		Timeout: 1,
	}}}
	m := hooks.New(cfg, "sess-7")
	start := time.Now()
	out, err := m.Fire(context.Background(), hooks.EventPreToolCall, hooks.HookInput{ToolName: "read_file"})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Decision != hooks.DecisionAllow {
		t.Fatalf("timeout should default to allow, got %q", out.Decision)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("timeout not enforced: took %v", elapsed)
	}
}

func TestFire_MatcherFiltersHook(t *testing.T) {
	cfg := hooks.Config{Hooks: []hooks.HookConfig{{
		Event:   "pre_tool_call",
		Command: `echo '{"decision":"block"}'`,
		Matcher: `^write_`,
		Timeout: 2,
	}}}
	m := hooks.New(cfg, "sess-8")

	// read_file should not match the ^write_ matcher → allow
	out, _ := m.Fire(context.Background(), hooks.EventPreToolCall, hooks.HookInput{ToolName: "read_file"})
	if out.Decision != hooks.DecisionAllow {
		t.Fatalf("non-matching tool should be allowed, got %q", out.Decision)
	}

	// write_file matches → block
	out, _ = m.Fire(context.Background(), hooks.EventPreToolCall, hooks.HookInput{ToolName: "write_file"})
	if out.Decision != hooks.DecisionBlock {
		t.Fatalf("matching tool should be blocked, got %q", out.Decision)
	}
}

func TestFire_WrongEventSkipped(t *testing.T) {
	cfg := hooks.Config{Hooks: []hooks.HookConfig{{
		Event:   "post_tool_call",
		Command: `echo '{"decision":"block"}'`,
		Timeout: 2,
	}}}
	m := hooks.New(cfg, "sess-9")
	out, _ := m.Fire(context.Background(), hooks.EventPreToolCall, hooks.HookInput{ToolName: "any"})
	if out.Decision != hooks.DecisionAllow {
		t.Fatalf("wrong-event hook should be skipped, got %q", out.Decision)
	}
}

func TestAuditTrail(t *testing.T) {
	cfg := hooks.Config{Hooks: []hooks.HookConfig{{
		Event:   "pre_tool_call",
		Command: `echo '{"decision":"allow"}'`,
		Timeout: 2,
	}}}
	m := hooks.New(cfg, "sess-10")
	m.Fire(context.Background(), hooks.EventPreToolCall, hooks.HookInput{ToolName: "read_file"}) //nolint:errcheck
	trail := m.AuditTrail()
	if len(trail) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(trail))
	}
	if trail[0].Event != "pre_tool_call" {
		t.Fatalf("unexpected event %q", trail[0].Event)
	}
	if trail[0].Decision != "allow" {
		t.Fatalf("unexpected decision %q", trail[0].Decision)
	}
}

func TestLoadConfigFile_MissingFile(t *testing.T) {
	cfg, err := hooks.LoadConfigFile("/does/not/exist/hooks.yaml")
	if err != nil {
		t.Fatalf("missing file should not error, got: %v", err)
	}
	if len(cfg.Hooks) != 0 {
		t.Fatalf("expected empty config, got %d hooks", len(cfg.Hooks))
	}
}
