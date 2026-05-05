package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestNewRegistry tests the creation of a new registry.
func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()
	if registry == nil {
		t.Error("expected non-nil registry, got nil")
	}
	if len(registry.All()) != 0 {
		t.Error("expected empty registry, got populated registry")
	}
}

// TestRegistryRegister tests plugin registration.
func TestRegistryRegister(t *testing.T) {
	registry := NewRegistry()

	plugin := &Plugin{
		Name:    "test-plugin",
		Version: "1.0.0",
		Tools: []*ToolDef{
			{Name: "tool1", Alias: "t1"},
		},
	}

	err := registry.Register(plugin)
	if err != nil {
		t.Fatalf("unexpected error registering plugin: %v", err)
	}

	if len(registry.All()) != 1 {
		t.Errorf("expected 1 plugin, got %d", len(registry.All()))
	}
}

// TestRegistryRegisterDuplicate tests that duplicate plugin names are rejected.
func TestRegistryRegisterDuplicate(t *testing.T) {
	registry := NewRegistry()

	plugin := &Plugin{Name: "test-plugin"}
	registry.Register(plugin)

	err := registry.Register(plugin)
	if err == nil {
		t.Error("expected error registering duplicate plugin, got nil")
	}
}

// TestRegistryGet tests retrieving a plugin by name.
func TestRegistryGet(t *testing.T) {
	registry := NewRegistry()

	plugin := &Plugin{Name: "test-plugin", Version: "1.0.0"}
	registry.Register(plugin)

	retrieved, ok := registry.Get("test-plugin")
	if !ok {
		t.Error("expected to find plugin, got false")
	}
	if retrieved.Name != "test-plugin" {
		t.Errorf("expected plugin name 'test-plugin', got %q", retrieved.Name)
	}
}

// TestRegistryGetNotFound tests retrieving a non-existent plugin.
func TestRegistryGetNotFound(t *testing.T) {
	registry := NewRegistry()

	_, ok := registry.Get("nonexistent")
	if ok {
		t.Error("expected to not find plugin, got true")
	}
}

// TestRegistryResolveAlias tests tool alias resolution.
func TestRegistryResolveAlias(t *testing.T) {
	registry := NewRegistry()

	plugin := &Plugin{
		Name: "test-plugin",
		Tools: []*ToolDef{
			{Name: "my-tool", Alias: "mt"},
		},
	}
	registry.Register(plugin)

	resolved := registry.ResolveAlias("mt")
	if resolved != "test-plugin" {
		t.Errorf("expected to resolve alias to 'test-plugin', got %q", resolved)
	}
}

// TestRegistryListTools tests listing all tools from registered plugins.
func TestRegistryListTools(t *testing.T) {
	registry := NewRegistry()

	plugin := &Plugin{
		Name: "test-plugin",
		Tools: []*ToolDef{
			{Name: "tool1"},
			{Name: "tool2"},
		},
	}
	registry.Register(plugin)

	tools := registry.ListTools()
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
}

// TestNewRuntime tests creation of a new plugin runtime.
func TestNewRuntime(t *testing.T) {
	runtime := NewRuntime()
	if runtime == nil {
		t.Error("expected non-nil runtime, got nil")
	}
	if len(runtime.ListPlugins()) != 0 {
		t.Error("expected empty runtime, got plugins")
	}
}

// TestLoadManifest tests loading a manifest from a directory.
func TestLoadManifest(t *testing.T) {
	// Create a temporary directory with a manifest
	tempDir := t.TempDir()

	manifest := &Manifest{
		Name:        "test-plugin",
		Version:     "1.0.0",
		Description: "A test plugin",
		Author:      "test-author",
		Hooks: []HookDef{
			{Event: "pre_tool", Handler: "handler.sh"},
		},
		Tools: []ToolDef{
			{Name: "test-tool", Handler: "tool.sh"},
		},
	}

	err := SaveManifest(tempDir, manifest)
	if err != nil {
		t.Fatalf("failed to save manifest: %v", err)
	}

	loaded, err := LoadManifest(tempDir)
	if err != nil {
		t.Fatalf("failed to load manifest: %v", err)
	}

	if loaded.Name != "test-plugin" {
		t.Errorf("expected name 'test-plugin', got %q", loaded.Name)
	}
	if loaded.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", loaded.Version)
	}
}

// TestLoadManifestNotFound tests loading a manifest from a non-existent directory.
func TestLoadManifestNotFound(t *testing.T) {
	_, err := LoadManifest("/nonexistent/directory")
	if err == nil {
		t.Error("expected error loading non-existent manifest, got nil")
	}
}

// TestSaveManifest tests saving a manifest to a directory.
func TestSaveManifest(t *testing.T) {
	tempDir := t.TempDir()

	manifest := &Manifest{
		Name:    "save-test",
		Version: "2.0.0",
	}

	err := SaveManifest(tempDir, manifest)
	if err != nil {
		t.Fatalf("failed to save manifest: %v", err)
	}

	// Verify the file was created
	manifestPath := filepath.Join(tempDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to read saved manifest: %v", err)
	}

	var loaded Manifest
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal manifest: %v", err)
	}

	if loaded.Name != "save-test" {
		t.Errorf("expected name 'save-test', got %q", loaded.Name)
	}
}

// TestRuntimeLoad tests loading a plugin into the runtime.
func TestRuntimeLoad(t *testing.T) {
	tempDir := t.TempDir()

	manifest := &Manifest{
		Name:    "test-plugin",
		Version: "1.0.0",
		Tools: []ToolDef{
			{Name: "test-tool"},
		},
	}
	SaveManifest(tempDir, manifest)

	runtime := NewRuntime()
	plugin, err := runtime.Load(tempDir)
	if err != nil {
		t.Fatalf("failed to load plugin: %v", err)
	}

	if plugin.Name != "test-plugin" {
		t.Errorf("expected plugin name 'test-plugin', got %q", plugin.Name)
	}
	if len(runtime.ListPlugins()) != 1 {
		t.Errorf("expected 1 plugin, got %d", len(runtime.ListPlugins()))
	}
}

// TestRuntimeLoadDuplicate tests that duplicate plugin loads are rejected.
func TestRuntimeLoadDuplicate(t *testing.T) {
	tempDir := t.TempDir()

	manifest := &Manifest{
		Name:    "dup-plugin",
		Version: "1.0.0",
	}
	SaveManifest(tempDir, manifest)

	runtime := NewRuntime()
	_, err := runtime.Load(tempDir)
	if err != nil {
		t.Fatalf("first load failed: %v", err)
	}

	_, err = runtime.Load(tempDir)
	if err == nil {
		t.Error("expected error loading duplicate plugin, got nil")
	}
}

// TestRuntimeLoadNoName tests that loading a manifest without a name fails.
func TestRuntimeLoadNoName(t *testing.T) {
	tempDir := t.TempDir()

	manifest := &Manifest{
		Version: "1.0.0",
		// Name is empty
	}
	SaveManifest(tempDir, manifest)

	runtime := NewRuntime()
	_, err := runtime.Load(tempDir)
	if err == nil {
		t.Error("expected error loading manifest without name, got nil")
	}
}

// TestRuntimeListPlugins tests listing all loaded plugins.
func TestRuntimeListPlugins(t *testing.T) {
	tempDir := t.TempDir()

	manifest := &Manifest{
		Name:    "plugin1",
		Version: "1.0.0",
	}
	SaveManifest(tempDir, manifest)

	runtime := NewRuntime()
	runtime.Load(tempDir)

	plugins := runtime.ListPlugins()
	if len(plugins) != 1 {
		t.Errorf("expected 1 plugin, got %d", len(plugins))
	}
}

// TestRuntimeListTools tests listing all tools from loaded plugins.
func TestRuntimeListTools(t *testing.T) {
	tempDir := t.TempDir()

	manifest := &Manifest{
		Name: "test-plugin",
		Tools: []ToolDef{
			{Name: "tool1"},
			{Name: "tool2"},
		},
	}
	SaveManifest(tempDir, manifest)

	runtime := NewRuntime()
	runtime.Load(tempDir)

	tools := runtime.ListTools()
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
}

// TestRuntimeInvokeHookNotFound tests invoking a hook from a non-existent plugin.
func TestRuntimeInvokeHookNotFound(t *testing.T) {
	runtime := NewRuntime()

	_, err := runtime.InvokeHook("nonexistent", "event", nil)
	if err == nil {
		t.Error("expected error invoking hook from non-existent plugin, got nil")
	}
}

// TestRuntimeInvokeToolNotFound tests invoking a tool from a non-existent plugin.
func TestRuntimeInvokeToolNotFound(t *testing.T) {
	runtime := NewRuntime()

	_, err := runtime.InvokeTool("nonexistent", "tool", nil)
	if err == nil {
		t.Error("expected error invoking tool from non-existent plugin, got nil")
	}
}

// TestRuntimeInvokeVirtualTool tests that virtual tools cannot be invoked.
func TestRuntimeInvokeVirtualTool(t *testing.T) {
	tempDir := t.TempDir()

	manifest := &Manifest{
		Name: "test-plugin",
		Tools: []ToolDef{
			{Name: "virtual-tool", Virtual: true},
		},
	}
	SaveManifest(tempDir, manifest)

	runtime := NewRuntime()
	runtime.Load(tempDir)

	_, err := runtime.InvokeTool("test-plugin", "virtual-tool", nil)
	if err == nil {
		t.Error("expected error invoking virtual tool, got nil")
	}
}

// TestManifestMarshalUnmarshal tests JSON marshaling/unmarshaling of Manifest.
func TestManifestMarshalUnmarshal(t *testing.T) {
	original := &Manifest{
		Name:        "marshal-test",
		Version:     "1.0.0",
		Description: "Test manifest",
		Author:      "test",
		Hooks: []HookDef{
			{Event: "pre_tool", Handler: "handler.sh"},
		},
		Tools: []ToolDef{
			{Name: "tool1", Description: "A tool", Alias: "t1"},
		},
		Permissions: []string{"read", "write"},
		Environment: map[string]string{"VAR": "value"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}

	var unmarshaled Manifest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal manifest: %v", err)
	}

	if unmarshaled.Name != original.Name {
		t.Errorf("name mismatch: expected %q, got %q", original.Name, unmarshaled.Name)
	}
	if unmarshaled.Version != original.Version {
		t.Errorf("version mismatch: expected %q, got %q", original.Version, unmarshaled.Version)
	}
	if len(unmarshaled.Tools) != len(original.Tools) {
		t.Errorf("tools count mismatch: expected %d, got %d", len(original.Tools), len(unmarshaled.Tools))
	}
}

// TestPluginStructure tests that the Plugin struct is properly constructed.
func TestPluginStructure(t *testing.T) {
	plugin := &Plugin{
		Name:        "test",
		Version:     "1.0.0",
		Description: "Test plugin",
		Author:      "Author",
		Directory:   "/tmp/test",
		Hooks:       []*HookDef{},
		Tools:       []*ToolDef{},
		Permissions: []string{"read"},
		Environment: map[string]string{"VAR": "value"},
	}

	if plugin.Name != "test" {
		t.Errorf("expected name 'test', got %q", plugin.Name)
	}
	if plugin.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", plugin.Version)
	}
	if len(plugin.Permissions) != 1 {
		t.Errorf("expected 1 permission, got %d", len(plugin.Permissions))
	}
}

// TestConcurrentRegistryAccess tests concurrent access to the registry.
func TestConcurrentRegistryAccess(t *testing.T) {
	registry := NewRegistry()

	// Register plugins concurrently
	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func(id int) {
			plugin := &Plugin{Name: "plugin-" + string(rune(id))}
			registry.Register(plugin)
			done <- true
		}(i)
	}

	for i := 0; i < 5; i++ {
		<-done
	}

	if len(registry.All()) != 5 {
		t.Errorf("expected 5 plugins, got %d", len(registry.All()))
	}
}

// TestConcurrentRuntimeAccess tests concurrent access to the runtime.
func TestConcurrentRuntimeAccess(t *testing.T) {
	runtime := NewRuntime()

	// Load plugins concurrently
	done := make(chan bool, 3)
	for i := 0; i < 3; i++ {
		go func(id int) {
			tempDir := t.TempDir()
			manifest := &Manifest{
				Name:    "concurrent-plugin-" + string(rune(id)),
				Version: "1.0.0",
			}
			SaveManifest(tempDir, manifest)
			runtime.Load(tempDir)
			done <- true
		}(i)
	}

	for i := 0; i < 3; i++ {
		<-done
	}

	if len(runtime.ListPlugins()) != 3 {
		t.Errorf("expected 3 plugins, got %d", len(runtime.ListPlugins()))
	}
}
