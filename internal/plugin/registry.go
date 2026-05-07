package plugin

import (
	"fmt"
	"sync"
)

// Registry manages a collection of loaded plugins with thread-safe access.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]*Plugin
	aliases map[string]string // Maps alias to plugin name
}

// NewRegistry creates an empty plugin registry.
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]*Plugin),
		aliases: make(map[string]string),
	}
}

// Register adds a plugin to the registry.
func (r *Registry) Register(plugin *Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[plugin.Name]; exists {
		return fmt.Errorf("plugin %q already registered", plugin.Name)
	}

	r.plugins[plugin.Name] = plugin

	// Register tool aliases
	for _, tool := range plugin.Tools {
		if tool.Alias != "" {
			if existing, aliasExists := r.aliases[tool.Alias]; aliasExists {
				return fmt.Errorf("alias %q for tool %q conflicts with existing alias in plugin %q", 
					tool.Alias, tool.Name, existing)
			}
			r.aliases[tool.Alias] = plugin.Name
		}
	}

	return nil
}

// Get retrieves a plugin by name.
func (r *Registry) Get(name string) (*Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	plugin, ok := r.plugins[name]
	return plugin, ok
}

// All returns all registered plugins.
func (r *Registry) All() []*Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	plugins := make([]*Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		plugins = append(plugins, p)
	}
	return plugins
}

// ResolveAlias returns the plugin name for a tool alias, or empty string if not found.
func (r *Registry) ResolveAlias(alias string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.aliases[alias]
}

// ListTools returns all tools from all registered plugins, including aliases.
func (r *Registry) ListTools() []*ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var tools []*ToolDef
	for _, plugin := range r.plugins {
		for _, tool := range plugin.Tools {
			tools = append(tools, tool)
		}
	}
	return tools
}
