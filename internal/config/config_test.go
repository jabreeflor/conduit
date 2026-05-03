package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jabreeflor/conduit/internal/config"
)

func writeYAML(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestLoadFiles_bothMissing(t *testing.T) {
	cfg, err := config.LoadFiles("/no/such/user.yaml", "/no/such/project.yaml")
	if err != nil {
		t.Fatalf("expected no error for missing files, got %v", err)
	}
	if cfg.Models.Primary != "" {
		t.Errorf("expected empty config, got models.primary=%q", cfg.Models.Primary)
	}
}

func TestLoadFiles_userOnly(t *testing.T) {
	dir := t.TempDir()
	userPath := writeYAML(t, dir, "user.yaml", `
models:
  primary: claude-opus-4-6
  fallbacks:
    - gpt-4o
escalation:
  default_model: claude-haiku-4-5
  escalation_model: claude-opus-4-6
  confidence_threshold: 0.65
`)
	cfg, err := config.LoadFiles(userPath, "/no/such/project.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Models.Primary != "claude-opus-4-6" {
		t.Errorf("models.primary: got %q, want %q", cfg.Models.Primary, "claude-opus-4-6")
	}
	if len(cfg.Models.Fallbacks) != 1 || cfg.Models.Fallbacks[0] != "gpt-4o" {
		t.Errorf("models.fallbacks: got %v", cfg.Models.Fallbacks)
	}
	if cfg.Escalation.DefaultModel != "claude-haiku-4-5" {
		t.Errorf("escalation.default_model: got %q", cfg.Escalation.DefaultModel)
	}
}

func TestLoadFiles_projectOverridesUser(t *testing.T) {
	dir := t.TempDir()
	userPath := writeYAML(t, dir, "user.yaml", `
models:
  primary: claude-opus-4-6
  fallbacks:
    - gpt-4o
escalation:
  default_model: claude-haiku-4-5
  escalation_model: claude-opus-4-6
  confidence_threshold: 0.65
`)
	projectPath := writeYAML(t, dir, "project.yaml", `
models:
  primary: gpt-4o
escalation:
  confidence_threshold: 0.80
`)
	cfg, err := config.LoadFiles(userPath, projectPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Project wins on models.primary.
	if cfg.Models.Primary != "gpt-4o" {
		t.Errorf("models.primary: got %q, want %q", cfg.Models.Primary, "gpt-4o")
	}
	// User value preserved for keys not overridden by project.
	if len(cfg.Models.Fallbacks) != 1 || cfg.Models.Fallbacks[0] != "gpt-4o" {
		t.Errorf("models.fallbacks: got %v, want [gpt-4o]", cfg.Models.Fallbacks)
	}
	// Project overrides nested escalation field.
	if cfg.Escalation.ConfidenceThreshold != 0.80 {
		t.Errorf("escalation.confidence_threshold: got %v, want 0.80", cfg.Escalation.ConfidenceThreshold)
	}
	// User's escalation fields not touched by project are preserved.
	if cfg.Escalation.DefaultModel != "claude-haiku-4-5" {
		t.Errorf("escalation.default_model: got %q, want %q", cfg.Escalation.DefaultModel, "claude-haiku-4-5")
	}
}

func TestLoadFiles_hooksAndPolicies(t *testing.T) {
	dir := t.TempDir()
	userPath := writeYAML(t, dir, "user.yaml", `
hooks:
  - event: pre_tool_call
    command: ~/.conduit/hooks/audit.sh
    matcher: "curl.*"
    timeout: 5
policies:
  - tool: exec
    require_confirmation: true
    when: outside_project_dir
`)
	cfg, err := config.LoadFiles(userPath, "/no/such/project.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Hooks) != 1 {
		t.Fatalf("hooks: got %d, want 1", len(cfg.Hooks))
	}
	if cfg.Hooks[0].Event != "pre_tool_call" {
		t.Errorf("hooks[0].event: got %q", cfg.Hooks[0].Event)
	}
	if cfg.Hooks[0].Timeout != 5 {
		t.Errorf("hooks[0].timeout: got %d", cfg.Hooks[0].Timeout)
	}
	if len(cfg.Policies) != 1 {
		t.Fatalf("policies: got %d, want 1", len(cfg.Policies))
	}
	if !cfg.Policies[0].RequireConfirmation {
		t.Errorf("policies[0].require_confirmation: expected true")
	}
}

func TestLoadFiles_budgetsAndCredentials(t *testing.T) {
	dir := t.TempDir()
	userPath := writeYAML(t, dir, "user.yaml", `
budgets:
  overall:
    monthly_limit: 200.00
    currency: USD
  models:
    claude-opus-4-6:
      monthly_limit: 80.00
      warning_pct: 75
      hard_stop: true
credentials:
  anthropic:
    primary: $ANTHROPIC_KEY_1
    pool:
      - $ANTHROPIC_KEY_2
`)
	cfg, err := config.LoadFiles(userPath, "/no/such/project.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Budgets.Overall.MonthlyLimit != 200.00 {
		t.Errorf("budgets.overall.monthly_limit: got %v", cfg.Budgets.Overall.MonthlyLimit)
	}
	opusBudget, ok := cfg.Budgets.Models["claude-opus-4-6"]
	if !ok {
		t.Fatal("budgets.models[claude-opus-4-6]: missing")
	}
	if !opusBudget.HardStop {
		t.Errorf("budgets.models[claude-opus-4-6].hard_stop: expected true")
	}
	if opusBudget.WarningPct != 75 {
		t.Errorf("budgets.models[claude-opus-4-6].warning_pct: got %d", opusBudget.WarningPct)
	}
	creds, ok := cfg.Credentials["anthropic"]
	if !ok {
		t.Fatal("credentials[anthropic]: missing")
	}
	if creds.Primary != "$ANTHROPIC_KEY_1" {
		t.Errorf("credentials[anthropic].primary: got %q", creds.Primary)
	}
	if len(creds.Pool) != 1 || creds.Pool[0] != "$ANTHROPIC_KEY_2" {
		t.Errorf("credentials[anthropic].pool: got %v", creds.Pool)
	}
}

func TestLoadFiles_costConfig(t *testing.T) {
	dir := t.TempDir()
	userPath := writeYAML(t, dir, "user.yaml", `
costs:
  pricing_path: ~/.conduit/pricing.json
  electricity_rate_usd_per_kwh: 0.18
  currency: USD
`)
	cfg, err := config.LoadFiles(userPath, "/no/such/project.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Costs.PricingPath != "~/.conduit/pricing.json" {
		t.Errorf("costs.pricing_path: got %q", cfg.Costs.PricingPath)
	}
	if cfg.Costs.ElectricityRateUSDPerKWh != 0.18 {
		t.Errorf("costs.electricity_rate_usd_per_kwh: got %v", cfg.Costs.ElectricityRateUSDPerKWh)
	}
	if cfg.Costs.Currency != "USD" {
		t.Errorf("costs.currency: got %q", cfg.Costs.Currency)
	}
}

func TestLoadFiles_invalidYAML(t *testing.T) {
	dir := t.TempDir()
	badPath := writeYAML(t, dir, "bad.yaml", "models: [\nnot: valid yaml{{{{")
	_, err := config.LoadFiles(badPath, "/no/such/project.yaml")
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoadFiles_projectSliceReplacesUser(t *testing.T) {
	dir := t.TempDir()
	userPath := writeYAML(t, dir, "user.yaml", `
models:
  fallbacks:
    - gpt-4o
    - ollama/llama3
`)
	projectPath := writeYAML(t, dir, "project.yaml", `
models:
  fallbacks:
    - local-model
`)
	cfg, err := config.LoadFiles(userPath, projectPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Slices replace, not append.
	if len(cfg.Models.Fallbacks) != 1 || cfg.Models.Fallbacks[0] != "local-model" {
		t.Errorf("models.fallbacks: got %v, want [local-model]", cfg.Models.Fallbacks)
	}
}

func TestLoadFiles_sandboxConfig(t *testing.T) {
	dir := t.TempDir()
	userPath := writeYAML(t, dir, "user.yaml", `
sandbox:
  backend: apple_virtualization
  base_image: ubuntu-24.04
  network_policy: controlled_egress
  preinstalled_runtimes:
    - go
    - node
    - python
    - rust
  runtime_versions:
    python: "3.12"
    node: "20"
    go: "1.22"
  package_managers:
    - pip
    - npm
    - yarn
    - pnpm
    - cargo
  preinstalled_tools:
    - git
    - curl
    - jq
    - rg
    - fd
    - vim
    - nano
    - sqlite3
  allowlisted_registries:
    - pypi.org
    - registry.npmjs.org
    - proxy.golang.org
    - crates.io
  custom_base_images:
    - name: team-python
      image: ghcr.io/acme/conduit-python:2026-05
      digest: sha256:abc123
      description: Team Python image
      shared: true
`)
	cfg, err := config.LoadFiles(userPath, "/no/such/project.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Sandbox.Backend != "apple_virtualization" {
		t.Errorf("sandbox.backend: got %q", cfg.Sandbox.Backend)
	}
	if cfg.Sandbox.BaseImage != "ubuntu-24.04" {
		t.Errorf("sandbox.base_image: got %q", cfg.Sandbox.BaseImage)
	}
	if len(cfg.Sandbox.PreinstalledRuntimes) != 4 {
		t.Errorf("sandbox.preinstalled_runtimes: got %v", cfg.Sandbox.PreinstalledRuntimes)
	}
	if cfg.Sandbox.RuntimeVersions["python"] != "3.12" {
		t.Errorf("sandbox.runtime_versions.python: got %q", cfg.Sandbox.RuntimeVersions["python"])
	}
	if len(cfg.Sandbox.PackageManagers) != 5 {
		t.Errorf("sandbox.package_managers: got %v", cfg.Sandbox.PackageManagers)
	}
	if len(cfg.Sandbox.PreinstalledTools) != 8 {
		t.Errorf("sandbox.preinstalled_tools: got %v", cfg.Sandbox.PreinstalledTools)
	}
	if len(cfg.Sandbox.AllowlistedRegistries) != 4 {
		t.Errorf("sandbox.allowlisted_registries: got %v", cfg.Sandbox.AllowlistedRegistries)
	}
	if len(cfg.Sandbox.CustomBaseImages) != 1 || !cfg.Sandbox.CustomBaseImages[0].Shared {
		t.Errorf("sandbox.custom_base_images: got %+v", cfg.Sandbox.CustomBaseImages)
	}
}
