// Package router provides Conduit's unified inference API and model
// provider failover logic.
package router

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const defaultProviderTimeout = 60 * time.Second

// TaskType describes the broad kind of work a model request should perform.
type TaskType string

const (
	TaskGeneral TaskType = "general"
	TaskCode    TaskType = "code"
	TaskBrowser TaskType = "browser"
	TaskImage   TaskType = "image"
)

// InputType identifies non-text request payloads that can require model
// capabilities such as vision.
type InputType string

const (
	InputText  InputType = "text"
	InputImage InputType = "image"
	InputPDF   InputType = "pdf"
)

// Capability describes a model/provider feature the router can require.
type Capability string

const (
	CapabilityText        Capability = "text"
	CapabilityVision      Capability = "vision"
	CapabilityComputerUse Capability = "computer_use"
)

// Config mirrors the model router section in ~/.conduit/config.yaml.
type Config struct {
	Models ModelConfig `yaml:"models"`
}

// ModelConfig is the top-level YAML block for model routing.
type ModelConfig struct {
	Primary      string           `yaml:"primary"`
	Fallbacks    []string         `yaml:"fallbacks"`
	ComputerUse  string           `yaml:"computer_use"`
	RoutingRules []RoutingRule    `yaml:"routing_rules"`
	Providers    []ProviderConfig `yaml:"providers"`
}

// RoutingRule can prefer a provider/model for a task or require a capability
// for a given input type.
type RoutingRule struct {
	TaskType          TaskType   `yaml:"task_type"`
	InputType         InputType  `yaml:"input_type"`
	RequireCapability Capability `yaml:"require_capability"`
	Prefer            string     `yaml:"prefer"`
}

// ProviderConfig defines static provider metadata used before an API client is
// attached.
type ProviderConfig struct {
	Name               string         `yaml:"name"`
	Model              string         `yaml:"model"`
	BaseURL            string         `yaml:"base_url,omitempty"`
	APIKey             string         `yaml:"api_key,omitempty"`
	Capabilities       []Capability   `yaml:"capabilities"`
	Timeout            time.Duration  `yaml:"timeout"`
	InputCostPer1KUSD  float64        `yaml:"input_cost_per_1k_usd"`
	OutputCostPer1KUSD float64        `yaml:"output_cost_per_1k_usd"`
	Extra              map[string]any `yaml:",inline"`
}

// LoadConfig reads the default Conduit config from ~/.conduit/config.yaml.
func LoadConfig() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, err
	}
	return LoadConfigFile(filepath.Join(home, ".conduit", "config.yaml"))
}

// LoadConfigFile reads a router config from path.
func LoadConfigFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Models.Primary == "" {
		return Config{}, errors.New("router config requires models.primary")
	}
	return cfg, nil
}

func providerConfigs(cfg Config) map[string]ProviderConfig {
	providers := make(map[string]ProviderConfig, len(cfg.Models.Providers)+len(cfg.Models.Fallbacks)+1)
	for _, provider := range cfg.Models.Providers {
		normalized := provider.withDefaults()
		providers[normalized.Name] = normalized
		if normalized.Model != "" {
			providers[normalized.Model] = normalized
		}
	}

	for _, name := range append([]string{cfg.Models.Primary}, cfg.Models.Fallbacks...) {
		if _, ok := providers[name]; !ok && name != "" {
			providers[name] = ProviderConfig{
				Name:         name,
				Model:        name,
				Capabilities: []Capability{CapabilityText},
				Timeout:      defaultProviderTimeout,
			}
		}
	}
	if cfg.Models.ComputerUse != "" {
		if _, ok := providers[cfg.Models.ComputerUse]; !ok {
			providers[cfg.Models.ComputerUse] = ProviderConfig{
				Name:         cfg.Models.ComputerUse,
				Model:        cfg.Models.ComputerUse,
				Capabilities: []Capability{CapabilityText, CapabilityComputerUse},
				Timeout:      defaultProviderTimeout,
			}
		}
	}
	return providers
}

func (p ProviderConfig) withDefaults() ProviderConfig {
	if p.Name == "" {
		p.Name = p.Model
	}
	if p.Model == "" {
		p.Model = p.Name
	}
	if len(p.Capabilities) == 0 {
		p.Capabilities = []Capability{CapabilityText}
	}
	if p.Timeout <= 0 {
		p.Timeout = defaultProviderTimeout
	}
	return p
}
