package router

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/contextassembler"
)

func TestRouterAssemblesContextBeforeProviderCall(t *testing.T) {
	cfg := Config{Models: ModelConfig{
		Primary: "openai",
		Providers: []ProviderConfig{
			{Name: "openai", Model: "gpt-4o"},
		},
	}}
	openai := &fakeProvider{name: "openai", response: Response{Text: "ok"}}
	sink := &fakeOptimizationSink{}

	r, err := New(
		cfg,
		[]Provider{openai},
		WithContextAssembler(contextassembler.New()),
		WithOptimizationSink(sink),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = r.Infer(context.Background(), Request{
		TaskType: TaskGeneral,
		Prompt:   "hello",
		Inputs:   []Input{{Type: InputText, Text: "supporting context"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(openai.requests) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(openai.requests))
	}
	if got := openai.requests[0].Prompt; got == "hello" || !strings.Contains(got, "<recent_conversation") {
		t.Fatalf("provider prompt was not assembled:\n%s", got)
	}
	if len(sink.summaries) != 1 {
		t.Fatalf("optimization summaries = %d, want 1", len(sink.summaries))
	}
}

func TestRouterRoutesCodeToPreferredProvider(t *testing.T) {
	cfg := Config{Models: ModelConfig{
		Primary:   "openai",
		Fallbacks: []string{"ollama"},
		RoutingRules: []RoutingRule{{
			TaskType: TaskCode,
			Prefer:   "claude",
		}},
		Providers: []ProviderConfig{
			{Name: "openai", Model: "gpt-4o"},
			{Name: "claude", Model: "claude-opus-4-6"},
			{Name: "ollama", Model: "ollama/llama3"},
		},
	}}
	claude := &fakeProvider{name: "claude", response: Response{Text: "ok"}}
	openai := &fakeProvider{name: "openai", response: Response{Text: "wrong"}}

	r, err := New(cfg, []Provider{claude, openai})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := r.Infer(context.Background(), Request{TaskType: TaskCode, Prompt: "write code"})
	if err != nil {
		t.Fatal(err)
	}

	if resp.Provider != "claude" {
		t.Fatalf("Provider = %q, want claude", resp.Provider)
	}
	if len(claude.requests) != 1 {
		t.Fatalf("claude calls = %d, want 1", len(claude.requests))
	}
	if len(openai.requests) != 0 {
		t.Fatalf("openai calls = %d, want 0", len(openai.requests))
	}
}

func TestRouterFailsOverFromCheckpointAndRecordsUsage(t *testing.T) {
	cfg := Config{Models: ModelConfig{
		Primary:   "claude",
		Fallbacks: []string{"openai"},
		Providers: []ProviderConfig{
			{
				Name:               "claude",
				Model:              "claude-opus-4-6",
				Timeout:            time.Second,
				InputCostPer1KUSD:  3,
				OutputCostPer1KUSD: 15,
			},
			{
				Name:               "openai",
				Model:              "gpt-4o",
				Timeout:            time.Second,
				InputCostPer1KUSD:  2.50,
				OutputCostPer1KUSD: 10,
			},
		},
	}}
	claude := &fakeProvider{name: "claude", err: errors.New("rate limited")}
	openai := &fakeProvider{
		name: "openai",
		response: Response{
			Text:  "resumed",
			Usage: Usage{InputTokens: 1000, OutputTokens: 500},
		},
	}
	usage := &MemoryUsageSink{}
	failovers := &MemoryFailoverSink{}
	checkpoints := fakeCheckpoints{checkpoint: Checkpoint{SessionID: "s1", ID: "cp-7"}}

	r, err := New(
		cfg,
		[]Provider{claude, openai},
		WithCheckpointStore(checkpoints),
		WithUsageSink(usage),
		WithFailoverSink(failovers),
	)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := r.Infer(context.Background(), Request{
		SessionID: "s1",
		TaskType:  TaskGeneral,
		Feature:   "chat",
		Plugin:    "core",
		Prompt:    "hello",
	})
	if err != nil {
		t.Fatal(err)
	}

	if resp.Provider != "openai" {
		t.Fatalf("Provider = %q, want openai", resp.Provider)
	}
	if got := openai.requests[0].CheckpointID; got != "cp-7" {
		t.Fatalf("fallback CheckpointID = %q, want cp-7", got)
	}
	if got := resp.Usage.CostUSD; got != 7.5 {
		t.Fatalf("CostUSD = %v, want 7.5", got)
	}
	if len(usage.Records) != 2 {
		t.Fatalf("usage records = %#v, want failed claude and successful openai records", usage.Records)
	}
	if usage.Records[0].Provider != "claude" || usage.Records[0].Status != "error" {
		t.Fatalf("first usage record = %#v, want claude error", usage.Records[0])
	}
	if usage.Records[0].ErrorType != "rate_limited" {
		t.Fatalf("ErrorType = %q, want rate_limited", usage.Records[0].ErrorType)
	}
	if usage.Records[1].Provider != "openai" || usage.Records[1].Status != "success" {
		t.Fatalf("second usage record = %#v, want openai success", usage.Records[1])
	}
	if usage.Records[1].Feature != "chat" || usage.Records[1].Plugin != "core" {
		t.Fatalf("feature/plugin = %q/%q, want chat/core", usage.Records[1].Feature, usage.Records[1].Plugin)
	}
	if len(failovers.Events) != 1 {
		t.Fatalf("failover events = %d, want 1", len(failovers.Events))
	}
	if failovers.Events[0].FromProvider != "claude" || failovers.Events[0].ToProvider != "openai" {
		t.Fatalf("failover event = %#v, want claude -> openai", failovers.Events[0])
	}
}

func TestRouterPromotesVisionCapableProviderForImageInput(t *testing.T) {
	cfg := Config{Models: ModelConfig{
		Primary:   "text-only",
		Fallbacks: []string{"vision"},
		RoutingRules: []RoutingRule{{
			InputType:         InputImage,
			RequireCapability: CapabilityVision,
			Prefer:            "vision",
		}},
		Providers: []ProviderConfig{
			{Name: "text-only", Model: "cheap-text", Capabilities: []Capability{CapabilityText}},
			{Name: "vision", Model: "claude-opus-4-6", Capabilities: []Capability{CapabilityText, CapabilityVision}},
		},
	}}
	textOnly := &fakeProvider{name: "text-only", response: Response{Text: "wrong"}}
	vision := &fakeProvider{name: "vision", response: Response{Text: "saw it"}}

	r, err := New(cfg, []Provider{textOnly, vision})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := r.Infer(context.Background(), Request{
		TaskType: TaskImage,
		Inputs:   []Input{{Type: InputImage, Ref: "/tmp/screenshot.png"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if resp.Provider != "vision" {
		t.Fatalf("Provider = %q, want vision", resp.Provider)
	}
	if len(textOnly.requests) != 0 {
		t.Fatalf("text-only calls = %d, want 0", len(textOnly.requests))
	}
}

func TestRouterAcceptsModelNamesInPriorityList(t *testing.T) {
	cfg := Config{Models: ModelConfig{
		Primary:   "claude-opus-4-6",
		Fallbacks: []string{"gpt-4o"},
		Providers: []ProviderConfig{
			{Name: "anthropic", Model: "claude-opus-4-6"},
			{Name: "openai", Model: "gpt-4o"},
		},
	}}
	anthropic := &fakeProvider{name: "anthropic", response: Response{Text: "ok"}}

	r, err := New(cfg, []Provider{anthropic})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := r.Infer(context.Background(), Request{TaskType: TaskGeneral})
	if err != nil {
		t.Fatal(err)
	}

	if resp.Provider != "anthropic" {
		t.Fatalf("Provider = %q, want anthropic", resp.Provider)
	}
	if resp.Model != "claude-opus-4-6" {
		t.Fatalf("Model = %q, want claude-opus-4-6", resp.Model)
	}
}

func TestLoadConfigFile(t *testing.T) {
	path := t.TempDir() + "/config.yaml"
	data := []byte(`
models:
  primary: claude
  fallbacks:
    - openai
  computer_use: codex
  routing_rules:
    - task_type: code
      prefer: claude
    - input_type: image
      require_capability: vision
      prefer: claude
  providers:
    - name: claude
      model: claude-opus-4-6
      timeout: 5s
      capabilities: [text, vision]
      input_cost_per_1k_usd: 3
      output_cost_per_1k_usd: 15
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Models.Primary != "claude" {
		t.Fatalf("Primary = %q, want claude", cfg.Models.Primary)
	}
	if got := cfg.Models.Providers[0].Timeout; got != 5*time.Second {
		t.Fatalf("Timeout = %s, want 5s", got)
	}
	if cfg.Models.RoutingRules[1].RequireCapability != CapabilityVision {
		t.Fatalf("RequireCapability = %q, want vision", cfg.Models.RoutingRules[1].RequireCapability)
	}
}

type fakeProvider struct {
	name     string
	response Response
	err      error
	requests []Request
}

func (p *fakeProvider) Name() string { return p.name }

func (p *fakeProvider) Infer(_ context.Context, req Request) (Response, error) {
	p.requests = append(p.requests, req)
	if p.err != nil {
		return Response{}, p.err
	}
	return p.response, nil
}

type fakeOptimizationSink struct {
	summaries []contextassembler.Summary
}

func (s *fakeOptimizationSink) RecordContextOptimization(_ context.Context, summary contextassembler.Summary) error {
	s.summaries = append(s.summaries, summary)
	return nil
}

type fakeCheckpoints struct {
	checkpoint Checkpoint
	err        error
}

func (c fakeCheckpoints) LastCheckpoint(context.Context, string) (Checkpoint, error) {
	if c.err != nil {
		return Checkpoint{}, c.err
	}
	return c.checkpoint, nil
}
