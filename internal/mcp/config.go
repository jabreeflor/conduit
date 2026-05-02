package mcp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// TransportKind identifies which transport an MCP server entry uses.
type TransportKind string

const (
	TransportStdio         TransportKind = "stdio"
	TransportStreamingHTTP TransportKind = "http"
)

// ServerEntry is one external MCP server registered in config.
type ServerEntry struct {
	// Name is the human-readable label for this server (used in @ToolName
	// resolution and the allowlist).
	Name string `yaml:"name"`

	// Transport selects the connection mechanism.
	Transport TransportKind `yaml:"transport"`

	// Command and Args launch a subprocess server (stdio transport only).
	Command string   `yaml:"command,omitempty"`
	Args    []string `yaml:"args,omitempty"`
	Env     []string `yaml:"env,omitempty"`

	// URL is the base URL for streaming-HTTP servers.
	URL string `yaml:"url,omitempty"`

	// Enabled can be set to false to disable a server without removing it.
	Enabled *bool `yaml:"enabled,omitempty"`

	// Allowlist restricts which tool names from this server are exposed.
	// Empty means all tools are allowed.
	Allowlist []string `yaml:"allowlist,omitempty"`
}

// IsEnabled returns true unless the entry explicitly sets enabled: false.
func (e ServerEntry) IsEnabled() bool {
	return e.Enabled == nil || *e.Enabled
}

// Config is the user-global or project-scoped MCP configuration.
// The two layers are merged at runtime: project config overrides global.
type Config struct {
	// Servers lists all registered MCP server entries.
	Servers []ServerEntry `yaml:"servers"`

	// ExposeAsServer, when true, starts a conduit-as-MCP-server listener so
	// external clients can call Conduit's built-in tools via MCP.
	ExposeAsServer bool `yaml:"expose_as_server,omitempty"`

	// ServerAddr is the listen address when ExposeAsServer is true.
	// Defaults to "127.0.0.1:0" (OS-assigned port) if empty.
	ServerAddr string `yaml:"server_addr,omitempty"`
}

// globalConfigPath returns ~/.conduit/mcp.yaml.
func globalConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".conduit", "mcp.yaml"), nil
}

// projectConfigPath returns .conduit/mcp.yaml relative to cwd.
func projectConfigPath() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, ".conduit", "mcp.yaml"), nil
}

// LoadConfig reads and merges user-global + project-scoped MCP configs,
// then layers any process-registered built-in entries on top (lowest
// precedence — user config always wins on a name collision).
// Missing files are silently skipped; parse errors are returned.
func LoadConfig() (Config, error) {
	global, err := globalConfigPath()
	if err != nil {
		return Config{}, err
	}
	project, err := projectConfigPath()
	if err != nil {
		return Config{}, err
	}
	cfg, err := loadAndMerge(global, project)
	if err != nil {
		return Config{}, err
	}
	return applyBuiltins(cfg), nil
}

// builtinServers is a process-wide registry of MCP server entries
// contributed by Conduit subsystems (currently: computer-use, PRD §6.8).
// Entries here are merged into LoadConfig's output as a base layer —
// user-supplied entries with the same name always win.
//
// Registration is idempotent on Name. Intended caller: cmd/conduit at
// startup. Tests can use ResetBuiltins to clean up.
var builtinServers []ServerEntry

// RegisterBuiltinServer adds (or replaces by Name) an entry that
// LoadConfig should include unless the user has already configured a
// server with the same name.
func RegisterBuiltinServer(entry ServerEntry) {
	for i, existing := range builtinServers {
		if existing.Name == entry.Name {
			builtinServers[i] = entry
			return
		}
	}
	builtinServers = append(builtinServers, entry)
}

// ResetBuiltins clears the built-in server registry. For tests.
func ResetBuiltins() {
	builtinServers = nil
}

// applyBuiltins merges builtinServers into cfg. User entries with the
// same Name win.
func applyBuiltins(cfg Config) Config {
	if len(builtinServers) == 0 {
		return cfg
	}
	existing := make(map[string]bool, len(cfg.Servers))
	for _, s := range cfg.Servers {
		existing[s.Name] = true
	}
	for _, b := range builtinServers {
		if existing[b.Name] {
			continue
		}
		cfg.Servers = append(cfg.Servers, b)
	}
	return cfg
}

func loadAndMerge(globalPath, projectPath string) (Config, error) {
	base, err := loadFile(globalPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("mcp: global config: %w", err)
	}

	overlay, err := loadFile(projectPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("mcp: project config: %w", err)
	}

	return merge(base, overlay), nil
}

func loadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// merge returns a config where project entries take precedence over global
// entries that share the same name, and project-level flags override global
// ones when set.
func merge(global, project Config) Config {
	out := Config{
		ExposeAsServer: global.ExposeAsServer,
		ServerAddr:     global.ServerAddr,
	}
	if project.ExposeAsServer {
		out.ExposeAsServer = true
	}
	if project.ServerAddr != "" {
		out.ServerAddr = project.ServerAddr
	}

	byName := make(map[string]ServerEntry, len(global.Servers))
	for _, s := range global.Servers {
		byName[s.Name] = s
	}
	for _, s := range project.Servers {
		byName[s.Name] = s // project wins
	}

	out.Servers = make([]ServerEntry, 0, len(byName))
	// Preserve global order first, then append project-only entries.
	seen := make(map[string]bool)
	for _, s := range global.Servers {
		merged := byName[s.Name]
		out.Servers = append(out.Servers, merged)
		seen[s.Name] = true
	}
	for _, s := range project.Servers {
		if !seen[s.Name] {
			out.Servers = append(out.Servers, s)
		}
	}
	return out
}
