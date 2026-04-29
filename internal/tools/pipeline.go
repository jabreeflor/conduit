// Package tools owns Conduit's tool policy pipeline.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProviderFormat identifies a model provider's tool schema dialect.
type ProviderFormat string

const (
	ProviderAnthropic ProviderFormat = "anthropic"
	ProviderOpenAI    ProviderFormat = "openai"
)

// Tool describes one callable capability before provider-specific schema
// normalization.
type Tool struct {
	Name        string
	Description string
	Schema      map[string]any
	Run         Runner
}

// Runner executes a tool with a cancellable context.
type Runner func(context.Context, json.RawMessage) (Result, error)

// Result is the normalized output from a tool execution.
type Result struct {
	Text string
	Data map[string]any
}

// Call is the policy input for a pending tool invocation.
type Call struct {
	ToolName string
	Agent    string
	Input    map[string]any
}

// Decision explains how the policy layer handled a tool call.
type Decision struct {
	Tool                 Tool
	Allowed              bool
	RequiresConfirmation bool
	Reason               string
}

// NormalizedTool is the provider-facing tool descriptor.
type NormalizedTool struct {
	Name        string
	Description string
	Schema      map[string]any
}

// AgentOverrides are agent-specific changes parsed by the identity layer from
// SOUL.md or profile metadata.
type AgentOverrides struct {
	BlockTools   []string
	AllowTools   []string
	ReplaceTools map[string]Tool
}

// Registry stores the base tool set.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates a base tool registry.
func NewRegistry(base []Tool) Registry {
	registry := Registry{tools: make(map[string]Tool, len(base))}
	for _, tool := range base {
		if tool.Name == "" {
			continue
		}
		registry.tools[tool.Name] = tool
	}
	return registry
}

// List returns registered tool names in stable order.
func (r Registry) List() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Pipeline applies base tools, agent overrides, policy rules, schema
// normalization, and abort wrapping in that order.
type Pipeline struct {
	registry Registry
	policy   PolicyConfig
}

// NewPipeline creates a policy pipeline from base tools and policy config.
func NewPipeline(base []Tool, policy PolicyConfig) *Pipeline {
	return &Pipeline{
		registry: NewRegistry(base),
		policy:   policy,
	}
}

// Resolve applies base tool lookup, agent overrides, and policy filtering.
func (p *Pipeline) Resolve(call Call, overrides AgentOverrides) Decision {
	tool, ok := p.registry.tools[call.ToolName]
	if !ok {
		return Decision{Allowed: false, Reason: "tool not registered"}
	}

	if replacement, ok := overrides.ReplaceTools[call.ToolName]; ok {
		tool = replacement
	}
	if matchesAny(tool.Name, overrides.BlockTools) && !matchesAny(tool.Name, overrides.AllowTools) {
		return Decision{Tool: tool, Allowed: false, Reason: "blocked by agent override"}
	}

	result := p.policy.evaluate(call)
	if result.blocked {
		return Decision{Tool: tool, Allowed: false, Reason: result.reason}
	}

	return Decision{
		Tool:                 tool,
		Allowed:              true,
		RequiresConfirmation: result.requiresConfirmation,
		Reason:               result.reason,
	}
}

// Normalize converts a resolved tool into the requested provider dialect.
func (p *Pipeline) Normalize(tool Tool, provider ProviderFormat) NormalizedTool {
	schema := cloneSchema(tool.Schema)
	switch provider {
	case ProviderAnthropic:
		schema = stripOpenAIStrict(schema)
	case ProviderOpenAI:
		if _, ok := schema["type"]; !ok {
			schema["type"] = "object"
		}
	}
	return NormalizedTool{
		Name:        tool.Name,
		Description: tool.Description,
		Schema:      schema,
	}
}

// WrapRunner returns a runner that observes the context before and during
// execution. This is Conduit's Go equivalent of AbortSignal wrapping.
func (p *Pipeline) WrapRunner(runner Runner) Runner {
	return func(ctx context.Context, input json.RawMessage) (Result, error) {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}

		type outcome struct {
			result Result
			err    error
		}
		done := make(chan outcome, 1)
		go func() {
			result, err := runner(ctx, input)
			done <- outcome{result: result, err: err}
		}()

		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case out := <-done:
			return out.result, out.err
		}
	}
}

// Execute resolves, normalizes, and runs a tool with the abort wrapper applied.
func (p *Pipeline) Execute(ctx context.Context, call Call, overrides AgentOverrides) (Result, Decision, error) {
	decision := p.Resolve(call, overrides)
	if !decision.Allowed {
		return Result{}, decision, errors.New(decision.Reason)
	}
	if decision.RequiresConfirmation {
		return Result{}, decision, errors.New("tool requires confirmation")
	}
	if decision.Tool.Run == nil {
		return Result{}, decision, errors.New("tool runner unavailable")
	}

	input, err := json.Marshal(call.Input)
	if err != nil {
		return Result{}, decision, err
	}
	result, err := p.WrapRunner(decision.Tool.Run)(ctx, input)
	return result, decision, err
}

// PolicyConfig mirrors ~/.conduit/policies.yaml.
type PolicyConfig struct {
	Policies []PolicyRule `yaml:"policies"`
}

// PolicyRule controls one tool, agent glob, or agent/tool combination.
type PolicyRule struct {
	Tool                string   `yaml:"tool"`
	Agent               string   `yaml:"agent"`
	Block               bool     `yaml:"block"`
	Allow               bool     `yaml:"allow"`
	RequireConfirmation bool     `yaml:"require_confirmation"`
	AllowedDomains      []string `yaml:"allowed_domains"`
	BlockTools          []string `yaml:"block_tools"`
	When                string   `yaml:"when"`
}

// LoadPolicy reads the default policy file from ~/.conduit/policies.yaml.
func LoadPolicy() (PolicyConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return PolicyConfig{}, err
	}
	return LoadPolicyFile(filepath.Join(home, ".conduit", "policies.yaml"))
}

// LoadPolicyFile reads a policy YAML file from path.
func LoadPolicyFile(path string) (PolicyConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PolicyConfig{}, err
	}
	var cfg PolicyConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return PolicyConfig{}, err
	}
	return cfg, nil
}

type policyResult struct {
	blocked              bool
	requiresConfirmation bool
	reason               string
}

func (c PolicyConfig) evaluate(call Call) policyResult {
	result := policyResult{}
	for _, rule := range c.Policies {
		if !rule.matches(call) {
			continue
		}
		if len(rule.BlockTools) > 0 && matchesAny(call.ToolName, rule.BlockTools) {
			result.blocked = true
			result.reason = "blocked by agent policy"
		}
		if rule.Block {
			result.blocked = true
			result.reason = "blocked by policy"
		}
		if len(rule.AllowedDomains) > 0 && !domainAllowed(call.Input, rule.AllowedDomains) {
			result.blocked = true
			result.reason = "domain blocked by policy"
		}
		if rule.Allow {
			result.blocked = false
			if result.reason == "" {
				result.reason = "allowed by policy"
			}
		}
		if rule.RequireConfirmation {
			result.requiresConfirmation = true
			if result.reason == "" {
				result.reason = "confirmation required by policy"
			}
		}
	}
	return result
}

func (r PolicyRule) matches(call Call) bool {
	if r.Tool != "" && !globMatch(r.Tool, call.ToolName) {
		return false
	}
	if r.Agent != "" && !globMatch(r.Agent, call.Agent) {
		return false
	}
	return r.Tool != "" || r.Agent != ""
}

func domainAllowed(input map[string]any, allowed []string) bool {
	raw := firstString(input, "url", "href", "domain")
	if raw == "" {
		return false
	}
	host := raw
	if parsed, err := url.Parse(raw); err == nil && parsed.Hostname() != "" {
		host = parsed.Hostname()
	}
	host = strings.ToLower(host)
	for _, pattern := range allowed {
		if domainMatch(pattern, host) {
			return true
		}
	}
	return false
}

func firstString(input map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := input[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func matchesAny(value string, patterns []string) bool {
	for _, pattern := range patterns {
		if globMatch(pattern, value) {
			return true
		}
	}
	return false
}

func globMatch(pattern, value string) bool {
	if pattern == "" {
		return false
	}
	if ok, err := filepath.Match(pattern, value); err == nil && ok {
		return true
	}
	return pattern == value
}

func domainMatch(pattern, host string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(host, suffix) || host == strings.TrimPrefix(suffix, ".")
	}
	return globMatch(pattern, host)
}

func cloneSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return map[string]any{"type": "object"}
	}
	data, err := json.Marshal(schema)
	if err != nil {
		panic(fmt.Sprintf("clone tool schema: %v", err))
	}
	var cloned map[string]any
	if err := json.Unmarshal(data, &cloned); err != nil {
		panic(fmt.Sprintf("clone tool schema: %v", err))
	}
	return cloned
}

func stripOpenAIStrict(schema map[string]any) map[string]any {
	delete(schema, "strict")
	for _, value := range schema {
		if nested, ok := value.(map[string]any); ok {
			stripOpenAIStrict(nested)
		}
		if nestedList, ok := value.([]any); ok {
			for _, item := range nestedList {
				if nested, ok := item.(map[string]any); ok {
					stripOpenAIStrict(nested)
				}
			}
		}
	}
	return schema
}
