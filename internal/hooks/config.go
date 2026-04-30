// Package hooks provides the hook execution subsystem for Conduit.
// Hooks run as shell subprocesses with a JSON wire protocol: event data on
// stdin, a decision (allow / block / inject) on stdout.
package hooks

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// EventType identifies one of the lifecycle points where hooks can fire.
type EventType string

const (
	EventPreToolCall    EventType = "pre_tool_call"
	EventPostToolCall   EventType = "post_tool_call"
	EventOnSessionStart EventType = "on_session_start"
	EventOnSessionEnd   EventType = "on_session_end"
	EventPreLLMCall     EventType = "pre_llm_call"
	EventPostLLMCall    EventType = "post_llm_call"
	EventOnMemoryWrite  EventType = "on_memory_write"
)

// HookConfig is one entry from ~/.conduit/hooks.yaml.
type HookConfig struct {
	Event   string `yaml:"event"`
	Command string `yaml:"command"`
	Matcher string `yaml:"matcher"` // optional regex filter on tool_name
	Timeout int    `yaml:"timeout"` // seconds; 0 → DefaultTimeout
}

// Config is the top-level structure of ~/.conduit/hooks.yaml.
type Config struct {
	Hooks []HookConfig `yaml:"hooks"`
}

const defaultConfigPath = ".conduit/hooks.yaml"

// LoadConfig reads hooks from ~/.conduit/hooks.yaml. A missing file is not an error.
func LoadConfig() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, err
	}
	return LoadConfigFile(filepath.Join(home, defaultConfigPath))
}

// LoadConfigFile reads hook config from an explicit path.
func LoadConfigFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
