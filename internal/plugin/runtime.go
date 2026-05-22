package plugin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// Plugin represents a loaded plugin instance with its manifest and metadata.
type Plugin struct {
	Name        string
	Version     string
	Description string
	Author      string
	Directory   string
	Manifest    *Manifest
	Hooks       []*HookDef
	Tools       []*ToolDef
	Permissions []string
	Environment map[string]string
}

// Runtime manages the lifecycle of loaded plugins and provides methods for invoking hooks and tools.
type Runtime struct {
	mu       sync.RWMutex
	plugins  map[string]*Plugin
	registry *Registry
}

// NewRuntime creates a new plugin runtime.
func NewRuntime() *Runtime {
	return &Runtime{
		plugins:  make(map[string]*Plugin),
		registry: NewRegistry(),
	}
}

// Load loads a plugin from a directory containing a manifest.json file.
func (rt *Runtime) Load(pluginDir string) (*Plugin, error) {
	manifest, err := LoadManifest(pluginDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load manifest from %s: %w", pluginDir, err)
	}

	if manifest.Name == "" {
		return nil, fmt.Errorf("plugin manifest must specify a name")
	}

	// Create Plugin struct from manifest
	plugin := &Plugin{
		Name:        manifest.Name,
		Version:     manifest.Version,
		Description: manifest.Description,
		Author:      manifest.Author,
		Directory:   pluginDir,
		Manifest:    manifest,
		Hooks:       make([]*HookDef, len(manifest.Hooks)),
		Tools:       make([]*ToolDef, len(manifest.Tools)),
		Permissions: manifest.Permissions,
		Environment: manifest.Environment,
	}

	// Copy hook definitions
	for i, hookDef := range manifest.Hooks {
		hook := &HookDef{
			Event:       hookDef.Event,
			Description: hookDef.Description,
			Handler:     hookDef.Handler,
		}
		plugin.Hooks[i] = hook
	}

	// Copy tool definitions
	for i, toolDef := range manifest.Tools {
		tool := &ToolDef{
			Name:        toolDef.Name,
			Description: toolDef.Description,
			Handler:     toolDef.Handler,
			InputSchema: toolDef.InputSchema,
			Alias:       toolDef.Alias,
			Virtual:     toolDef.Virtual,
		}
		plugin.Tools[i] = tool
	}

	// Register plugin
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if _, exists := rt.plugins[plugin.Name]; exists {
		return nil, fmt.Errorf("plugin %q already loaded", plugin.Name)
	}

	if err := rt.registry.Register(plugin); err != nil {
		return nil, err
	}

	rt.plugins[plugin.Name] = plugin
	return plugin, nil
}

// InvokeHook executes a hook handler from a plugin.
// Returns the output of the hook invocation.
func (rt *Runtime) InvokeHook(pluginName string, event string, input map[string]interface{}) (string, error) {
	rt.mu.RLock()
	plugin, ok := rt.plugins[pluginName]
	rt.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("plugin %q not found", pluginName)
	}

	// Find the hook with the specified event
	var hook *HookDef
	for _, h := range plugin.Hooks {
		if h.Event == event {
			hook = h
			break
		}
	}

	if hook == nil {
		return "", fmt.Errorf("hook for event %q not found in plugin %q", event, pluginName)
	}

	// Execute the hook handler
	return rt.executeHandler(plugin, hook.Handler, input)
}

// InvokeTool executes a tool handler from a plugin.
// Returns the output of the tool invocation.
func (rt *Runtime) InvokeTool(pluginName string, toolName string, input map[string]interface{}) (string, error) {
	rt.mu.RLock()
	plugin, ok := rt.plugins[pluginName]
	rt.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("plugin %q not found", pluginName)
	}

	// Find the tool with the specified name
	var tool *ToolDef
	for _, t := range plugin.Tools {
		if t.Name == toolName {
			tool = t
			break
		}
	}

	if tool == nil {
		return "", fmt.Errorf("tool %q not found in plugin %q", toolName, pluginName)
	}

	if tool.Virtual {
		return "", fmt.Errorf("tool %q is virtual and cannot be invoked", toolName)
	}

	// Execute the tool handler
	return rt.executeHandler(plugin, tool.Handler, input)
}

// ListTools returns all tools provided by all loaded plugins.
func (rt *Runtime) ListTools() []*ToolDef {
	return rt.registry.ListTools()
}

// ListPlugins returns all loaded plugins.
func (rt *Runtime) ListPlugins() []*Plugin {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	plugins := make([]*Plugin, 0, len(rt.plugins))
	for _, p := range rt.plugins {
		plugins = append(plugins, p)
	}
	return plugins
}

// executeHandler runs a handler command or script.
// This is a simple implementation that executes shell commands.
func (rt *Runtime) executeHandler(plugin *Plugin, handler string, input map[string]interface{}) (string, error) {
	// Construct the command path relative to plugin directory
	handlerPath := filepath.Join(plugin.Directory, handler)

	// Check if the handler exists
	if _, err := os.Stat(handlerPath); err != nil {
		return "", fmt.Errorf("handler %q not found: %w", handler, err)
	}

	// Make it executable
	os.Chmod(handlerPath, 0755)

	// Execute the handler as a subprocess
	cmd := exec.Command(handlerPath)
	cmd.Dir = plugin.Directory
	cmd.Env = append(os.Environ(), mapToEnv(plugin.Environment)...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("handler %q failed: %w", handler, err)
	}

	return string(output), nil
}

// mapToEnv converts a map to environment variable strings.
func mapToEnv(envMap map[string]string) []string {
	env := make([]string, 0, len(envMap))
	for k, v := range envMap {
		env = append(env, k+"="+v)
	}
	return env
}
