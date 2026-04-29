package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
)

func TestPipelineAppliesAgentOverrideBeforePolicy(t *testing.T) {
	pipeline := NewPipeline([]Tool{
		{Name: "exec", Description: "run a command"},
	}, PolicyConfig{
		Policies: []PolicyRule{{
			Tool:                "exec",
			RequireConfirmation: true,
		}},
	})

	decision := pipeline.Resolve(Call{ToolName: "exec", Agent: "main"}, AgentOverrides{
		BlockTools: []string{"exec"},
	})

	if decision.Allowed {
		t.Fatal("exec was allowed, want blocked by agent override")
	}
	if decision.RequiresConfirmation {
		t.Fatal("policy confirmation ran after blocking override")
	}
	if decision.Reason != "blocked by agent override" {
		t.Fatalf("Reason = %q, want agent override", decision.Reason)
	}
}

func TestPolicySupportsBlockAllowConfirmationAndAgentRestrictions(t *testing.T) {
	pipeline := NewPipeline([]Tool{
		{Name: "exec"},
		{Name: "read_file"},
	}, PolicyConfig{
		Policies: []PolicyRule{
			{Tool: "exec", RequireConfirmation: true},
			{Agent: "subagent-*", BlockTools: []string{"exec"}},
			{Tool: "read_file", Block: true},
			{Tool: "read_file", Allow: true},
		},
	})

	execDecision := pipeline.Resolve(Call{ToolName: "exec", Agent: "main"}, AgentOverrides{})
	if !execDecision.Allowed || !execDecision.RequiresConfirmation {
		t.Fatalf("exec decision = %#v, want allowed with confirmation", execDecision)
	}

	subagentDecision := pipeline.Resolve(Call{ToolName: "exec", Agent: "subagent-1"}, AgentOverrides{})
	if subagentDecision.Allowed {
		t.Fatalf("subagent exec decision = %#v, want blocked", subagentDecision)
	}

	readDecision := pipeline.Resolve(Call{ToolName: "read_file", Agent: "main"}, AgentOverrides{})
	if !readDecision.Allowed {
		t.Fatalf("read_file decision = %#v, want later allow to override block", readDecision)
	}
}

func TestPolicyAllowsOnlyConfiguredDomains(t *testing.T) {
	pipeline := NewPipeline([]Tool{{Name: "browser"}}, PolicyConfig{
		Policies: []PolicyRule{{
			Tool:           "browser",
			AllowedDomains: []string{"*.github.com", "anthropic.com"},
		}},
	})

	allowed := pipeline.Resolve(Call{
		ToolName: "browser",
		Input:    map[string]any{"url": "https://api.github.com/repos/jabreeflor/conduit"},
	}, AgentOverrides{})
	if !allowed.Allowed {
		t.Fatalf("github decision = %#v, want allowed", allowed)
	}

	blocked := pipeline.Resolve(Call{
		ToolName: "browser",
		Input:    map[string]any{"url": "https://example.com"},
	}, AgentOverrides{})
	if blocked.Allowed {
		t.Fatalf("example decision = %#v, want blocked", blocked)
	}
}

func TestNormalizeHandlesProviderSchemaQuirks(t *testing.T) {
	pipeline := NewPipeline(nil, PolicyConfig{})
	tool := Tool{
		Name:        "write_file",
		Description: "write a file",
		Schema: map[string]any{
			"type":   "object",
			"strict": true,
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "strict": true},
			},
		},
	}

	anthropic := pipeline.Normalize(tool, ProviderAnthropic)
	if _, ok := anthropic.Schema["strict"]; ok {
		t.Fatalf("anthropic schema kept strict: %#v", anthropic.Schema)
	}
	properties := anthropic.Schema["properties"].(map[string]any)
	pathSchema := properties["path"].(map[string]any)
	if _, ok := pathSchema["strict"]; ok {
		t.Fatalf("anthropic nested schema kept strict: %#v", pathSchema)
	}

	openai := pipeline.Normalize(Tool{Name: "empty"}, ProviderOpenAI)
	if openai.Schema["type"] != "object" {
		t.Fatalf("openai schema type = %#v, want object", openai.Schema["type"])
	}
}

func TestExecuteWrapsRunnerWithAbortSignal(t *testing.T) {
	pipeline := NewPipeline([]Tool{{
		Name: "slow",
		Run: func(ctx context.Context, _ json.RawMessage) (Result, error) {
			<-ctx.Done()
			return Result{}, ctx.Err()
		},
	}}, PolicyConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := pipeline.Execute(ctx, Call{ToolName: "slow"}, AgentOverrides{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute error = %v, want context canceled", err)
	}
}

func TestWrapRunnerReturnsWhenContextCancelsDuringExecution(t *testing.T) {
	pipeline := NewPipeline(nil, PolicyConfig{})
	runner := pipeline.WrapRunner(func(context.Context, json.RawMessage) (Result, error) {
		time.Sleep(time.Minute)
		return Result{Text: "late"}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runner(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wrapped runner error = %v, want context canceled", err)
	}
}

func TestLoadPolicyFile(t *testing.T) {
	path := t.TempDir() + "/policies.yaml"
	if err := os.WriteFile(path, []byte(`
policies:
  - tool: exec
    require_confirmation: true
  - tool: browser
    allowed_domains:
      - "*.github.com"
  - agent: subagent-*
    block_tools:
      - exec
`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadPolicyFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Policies) != 3 {
		t.Fatalf("len(Policies) = %d, want 3", len(cfg.Policies))
	}
	if !cfg.Policies[0].RequireConfirmation {
		t.Fatal("exec policy did not load require_confirmation")
	}
	if cfg.Policies[1].AllowedDomains[0] != "*.github.com" {
		t.Fatalf("AllowedDomains = %#v, want github wildcard", cfg.Policies[1].AllowedDomains)
	}
}
