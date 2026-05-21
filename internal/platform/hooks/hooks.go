// Package hooks implements Conduit's event-driven extensibility system.
// Hooks run as shell subprocesses with a JSON wire protocol.
package hooks

import "gopkg.in/yaml.v3"

// Event identifies one of the 7 supported hook points.
type Event string

const (
	EventOnSessionStart Event = "on_session_start"
	EventOnSessionEnd   Event = "on_session_end"
	EventPreLLMCall     Event = "pre_llm_call"
	EventPostLLMCall    Event = "post_llm_call"
	EventPreToolCall    Event = "pre_tool_call"
	EventPostToolCall   Event = "post_tool_call"
	EventOnMemoryWrite  Event = "on_memory_write"
)

// Decision is the directive returned by a hook process.
type Decision string

const (
	DecisionAllow  Decision = "allow"
	DecisionBlock  Decision = "block"
	DecisionInject Decision = "inject"
)

// Input is sent to the hook process via stdin as JSON.
type Input struct {
	Event     Event          `json:"event"`
	ToolName  string         `json:"tool_name,omitempty"`
	ToolInput map[string]any `json:"tool_input,omitempty"`
	SessionID string         `json:"session_id"`
	CWD       string         `json:"cwd"`
}

// Output is parsed from the hook process's stdout.
// A hook that crashes or times out is treated as Output{Decision: DecisionAllow}.
type Output struct {
	Decision Decision `json:"decision"`
	Reason   string   `json:"reason,omitempty"`
	Context  string   `json:"context,omitempty"`
}

// HookDef is one registered hook from ~/.conduit/config.yaml.
type HookDef struct {
	Event   Event  `yaml:"event"`
	Command string `yaml:"command"`
	// Matcher is an optional regex applied to the tool name for pre/post_tool_call,
	// or to session_id for other events. Empty means always match.
	Matcher string `yaml:"matcher,omitempty"`
	// Timeout is the per-hook timeout in seconds. 0 uses the default (5s).
	Timeout int `yaml:"timeout,omitempty"`
}

// Config holds all registered hooks parsed from the config file.
type Config struct {
	Hooks []HookDef `yaml:"hooks"`
}

// ParseConfig decodes YAML hook configuration.
func ParseConfig(data []byte) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
