package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Manifest describes the structure and metadata of a plugin.
type Manifest struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description,omitempty"`
	Author      string            `json:"author,omitempty"`
	Hooks       []HookDef         `json:"hooks,omitempty"`
	Tools       []ToolDef         `json:"tools,omitempty"`
	Permissions []string          `json:"permissions,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
}

// HookDef defines a lifecycle hook provided by the plugin.
type HookDef struct {
	Event       string `json:"event"`        // Hook event (e.g., "before_tool", "after_tool")
	Description string `json:"description"` // Human-readable description
	Handler     string `json:"handler"`     // Function or command to invoke
}

// ToolDef defines a tool provided by the plugin that can be invoked by the agent.
type ToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Handler     string                 `json:"handler"`        // Function or command to invoke
	InputSchema map[string]interface{} `json:"input_schema"` // JSON schema for input parameters
	Alias       string                 `json:"alias,omitempty"` // Alternative name for the tool
	Virtual     bool                   `json:"virtual,omitempty"` // Whether this is a virtual tool (no actual handler)
}

// LoadManifest reads and parses a plugin manifest from a manifest.json file.
func LoadManifest(pluginDir string) (*Manifest, error) {
	manifestPath := filepath.Join(pluginDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

// SaveManifest writes a plugin manifest to manifest.json.
func SaveManifest(pluginDir string, manifest *Manifest) error {
	manifestPath := filepath.Join(pluginDir, "manifest.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath, data, 0644)
}
