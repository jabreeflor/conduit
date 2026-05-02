// Package config loads the single root YAML configuration for Conduit.
//
// Load order: ~/.conduit/config.yaml (user-global) is read first, then
// .conduit/config.yaml (project-scoped) is deep-merged on top. Project values
// take precedence over user-global values at every nested key.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the single root YAML document for Conduit.
type Config struct {
	Models      ModelsConfig                   `yaml:"models"`
	Escalation  EscalationConfig               `yaml:"escalation"`
	Hooks       []HookConfig                   `yaml:"hooks"`
	Policies    []PolicyConfig                 `yaml:"policies"`
	Sandbox     SandboxConfig                  `yaml:"sandbox"`
	Budgets     BudgetsConfig                  `yaml:"budgets"`
	Costs       CostConfig                     `yaml:"costs"`
	Credentials map[string]ProviderCredentials `yaml:"credentials"`
	ComputerUse ComputerUseConfig              `yaml:"computer_use"`
}

// ComputerUseConfig controls the open-codex-computer-use MCP integration
// (PRD §6.8). The full schema lives in internal/computeruse so the runtime
// gating logic stays close to its consumers; this struct mirrors the
// subset that lands in the root YAML config.
type ComputerUseConfig struct {
	// Enabled gates the integration. Defaults to false (off).
	Enabled bool `yaml:"enabled"`

	// Command overrides the binary path. Empty means the upstream default
	// (`open-computer-use`, installed via `npm i -g open-computer-use`).
	Command string `yaml:"command,omitempty"`

	// Args overrides the launch args. Nil means the upstream default
	// (["mcp"] — stdio transport).
	Args []string `yaml:"args,omitempty"`

	// Env is forwarded to the subprocess as "KEY=value" pairs.
	Env []string `yaml:"env,omitempty"`

	// Allowlist restricts which tools the upstream server may expose.
	// Empty means all upstream tools are allowed.
	Allowlist []string `yaml:"allowlist,omitempty"`

	// ForceNonDarwin allows enabling the integration on non-macOS hosts
	// (advanced; v1 permission flow is macOS-first — see PRD §6.8).
	ForceNonDarwin bool `yaml:"force_non_darwin,omitempty"`
}

// ModelsConfig is the model-routing section.
type ModelsConfig struct {
	Primary      string           `yaml:"primary"`
	Fallbacks    []string         `yaml:"fallbacks"`
	ComputerUse  string           `yaml:"computer_use"`
	RoutingRules []RoutingRule    `yaml:"routing_rules"`
	Providers    []ProviderConfig `yaml:"providers"`
}

// RoutingRule prefers a model for a task type or requires a capability for an
// input type.
type RoutingRule struct {
	TaskType          string `yaml:"task_type"`
	InputType         string `yaml:"input_type"`
	RequireCapability string `yaml:"require_capability"`
	Prefer            string `yaml:"prefer"`
}

// ProviderConfig defines per-provider model metadata and per-token cost.
type ProviderConfig struct {
	Name               string   `yaml:"name"`
	Model              string   `yaml:"model"`
	Capabilities       []string `yaml:"capabilities"`
	TimeoutSeconds     int      `yaml:"timeout_seconds"`
	InputCostPer1KUSD  float64  `yaml:"input_cost_per_1k_usd"`
	OutputCostPer1KUSD float64  `yaml:"output_cost_per_1k_usd"`
}

// EscalationConfig controls cheap-to-capable model routing.
type EscalationConfig struct {
	DefaultModel        string             `yaml:"default_model"`
	EscalationModel     string             `yaml:"escalation_model"`
	ConfidenceThreshold float64            `yaml:"confidence_threshold"`
	Triggers            EscalationTriggers `yaml:"triggers"`
}

// EscalationTriggers selects which conditions promote to the escalation model.
type EscalationTriggers struct {
	LowConfidence bool     `yaml:"low_confidence"`
	FirstRun      bool     `yaml:"first_run"`
	TaskTags      []string `yaml:"task_tags"`
}

// HookConfig registers one shell-subprocess hook for an agent loop event.
type HookConfig struct {
	Event   string `yaml:"event"`
	Command string `yaml:"command"`
	// Matcher is an optional regex applied to the tool name (pre_tool_call only).
	Matcher string `yaml:"matcher"`
	// Timeout is the maximum seconds the hook process may run before being killed.
	Timeout int `yaml:"timeout"`
}

// PolicyConfig defines one allow/block/confirm rule for a tool or agent.
type PolicyConfig struct {
	Tool                string   `yaml:"tool"`
	Agent               string   `yaml:"agent"`
	Block               bool     `yaml:"block"`
	RequireConfirmation bool     `yaml:"require_confirmation"`
	When                string   `yaml:"when"`
	AllowedDomains      []string `yaml:"allowed_domains"`
	BlockTools          []string `yaml:"block_tools"`
}

// SandboxConfig controls the agent code-execution isolation layer.
type SandboxConfig struct {
	Backend              string   `yaml:"backend"`
	BaseImage            string   `yaml:"base_image"`
	NetworkPolicy        string   `yaml:"network_policy"`
	PreinstalledRuntimes []string `yaml:"preinstalled_runtimes"`
}

// BudgetsConfig sets spending limits by provider, feature, and plugin.
type BudgetsConfig struct {
	Overall  BudgetLimit            `yaml:"overall"`
	Models   map[string]ModelBudget `yaml:"models"`
	Features map[string]BudgetLimit `yaml:"features"`
	Plugins  map[string]BudgetLimit `yaml:"plugins"`
}

// BudgetLimit is a monthly spend ceiling with an optional currency code.
type BudgetLimit struct {
	MonthlyLimit float64 `yaml:"monthly_limit"`
	Currency     string  `yaml:"currency"`
}

// ModelBudget adds per-model warning thresholds and hard-stop enforcement.
type ModelBudget struct {
	MonthlyLimit float64 `yaml:"monthly_limit"`
	WarningPct   int     `yaml:"warning_pct"`
	HardStop     bool    `yaml:"hard_stop"`
}

// CostConfig controls local pricing and energy assumptions.
type CostConfig struct {
	PricingPath              string  `yaml:"pricing_path"`
	ElectricityRateUSDPerKWh float64 `yaml:"electricity_rate_usd_per_kwh"`
	Currency                 string  `yaml:"currency"`
}

// ProviderCredentials holds env-var references for a single API provider.
// Values are environment variable names (prefixed with $) or literal strings.
// Never store raw secrets in config files — always reference env vars.
type ProviderCredentials struct {
	Primary string   `yaml:"primary"`
	Pool    []string `yaml:"pool"`
}

// ErrInvalidYAML is returned when a config file contains malformed YAML.
var ErrInvalidYAML = errors.New("invalid YAML")

// Load reads the user-global config (~/.conduit/config.yaml) then
// deep-merges the project-scoped config (.conduit/config.yaml) on top.
// Either or both files may be absent; missing files are silently skipped.
func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("resolving home directory: %w", err)
	}
	return LoadFiles(
		filepath.Join(home, ".conduit", "config.yaml"),
		filepath.Join(".conduit", "config.yaml"),
	)
}

// LoadFiles loads from explicit user and project config file paths.
// Files that do not exist are silently skipped. Project config values take
// precedence over user config values.
func LoadFiles(userPath, projectPath string) (Config, error) {
	merged := make(map[string]any)

	for _, path := range []string{userPath, projectPath} {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return Config{}, fmt.Errorf("reading %s: %w", path, err)
		}

		var layer map[string]any
		if err := yaml.Unmarshal(data, &layer); err != nil {
			return Config{}, fmt.Errorf("%w: %s: %w", ErrInvalidYAML, path, err)
		}
		deepMerge(merged, layer)
	}

	// Round-trip through YAML to decode the merged map into the typed Config.
	mergedBytes, err := yaml.Marshal(merged)
	if err != nil {
		return Config{}, fmt.Errorf("re-encoding merged config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(mergedBytes, &cfg); err != nil {
		return Config{}, fmt.Errorf("decoding merged config: %w", err)
	}
	return cfg, nil
}

// deepMerge merges src into dst in-place.
//
// TODO: implement the merge strategy here — this is where the explicit
// precedence rule lives. Two approaches worth considering:
//
//  1. Recursive map merge: for each key, if both dst and src hold a
//     map[string]any, recurse. For all other types (including slices),
//     src replaces dst. This gives fine-grained per-key overrides but
//     means a project config that sets only escalation.default_model also
//     keeps the user's escalation.escalation_model.
//
//  2. Top-level section replace: if src contains a key at the top level
//     (models, escalation, hooks, …), replace the entire dst section with
//     the src value — no recursion. Simpler, but a project config that
//     overrides one models field must repeat all others.
//
// Implement your preferred strategy below (5-10 lines of recursive Go).
// Consider: should slices append or replace? Should nested maps merge or
// replace? How should a project config that only sets one field inside a
// section behave?
func deepMerge(dst, src map[string]any) {
	for k, srcVal := range src {
		dstVal, exists := dst[k]
		if !exists {
			dst[k] = srcVal
			continue
		}
		srcMap, srcIsMap := srcVal.(map[string]any)
		dstMap, dstIsMap := dstVal.(map[string]any)
		if srcIsMap && dstIsMap {
			deepMerge(dstMap, srcMap)
		} else {
			dst[k] = srcVal
		}
	}
}
