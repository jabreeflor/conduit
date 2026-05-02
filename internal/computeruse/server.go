package computeruse

import (
	"runtime"

	"github.com/jabreeflor/conduit/internal/mcp"
)

// ServerName is the canonical MCP server name Conduit uses for the
// open-codex-computer-use integration. The wider codebase (allowlists,
// approval gates, telemetry) keys off this string, so do not change it
// without a migration.
const ServerName = "open-computer-use"

// DefaultCommand is the binary published by the upstream npm package
// `open-computer-use`. Users install it once with `npm i -g
// open-computer-use` and grant Accessibility + Screen Recording on macOS
// the first time it runs.
const DefaultCommand = "open-computer-use"

// DefaultArgs invokes the server's stdio MCP transport, matching the
// snippet documented in the upstream README's "mcpServers" example.
var DefaultArgs = []string{"mcp"}

// Config controls whether and how Conduit launches the open-computer-use
// MCP server. Zero value is "off"; Enabled must be set explicitly.
//
// Sibling issues #38–#42 layer on top of this struct (per-app approval,
// safety rails) — extend rather than rename.
type Config struct {
	// Enabled gates the entire integration. Defaults to false. The
	// effective gate also requires runtime.GOOS == "darwin" unless the
	// caller passes ForceNonDarwin (intended for tests / Linux preview).
	Enabled bool `yaml:"enabled"`

	// Command overrides the binary path. Empty means DefaultCommand.
	Command string `yaml:"command,omitempty"`

	// Args overrides the launch args. Nil means DefaultArgs.
	Args []string `yaml:"args,omitempty"`

	// Env is forwarded to the subprocess. Use the "KEY=value" form mcp
	// already accepts.
	Env []string `yaml:"env,omitempty"`

	// Allowlist optionally restricts which tools the server may expose.
	// Empty means all upstream tools are allowed; refer to the
	// open-computer-use README for the current tool set.
	Allowlist []string `yaml:"allowlist,omitempty"`

	// ForceNonDarwin allows enabling the integration on non-macOS hosts.
	// Off by default because Accessibility + Screen Recording are
	// macOS-specific; upstream does support Linux/Windows but Conduit's
	// permission flow (#38) is macOS-first in v1.
	ForceNonDarwin bool `yaml:"force_non_darwin,omitempty"`
}

// IsActive reports whether ServerEntry should be registered for this
// host given the config and current GOOS.
func (c Config) IsActive() bool {
	if !c.Enabled {
		return false
	}
	if runtime.GOOS == "darwin" {
		return true
	}
	return c.ForceNonDarwin
}

// ServerEntry returns the mcp.ServerEntry that boots open-computer-use
// over stdio. The second return value is false when the integration is
// inactive on the current host (caller should skip registration).
//
// Defaults are filled in here so callers can pass a bare Config{Enabled: true}
// and get a working entry.
func (c Config) ServerEntry() (mcp.ServerEntry, bool) {
	if !c.IsActive() {
		return mcp.ServerEntry{}, false
	}

	cmd := c.Command
	if cmd == "" {
		cmd = DefaultCommand
	}

	args := c.Args
	if args == nil {
		args = DefaultArgs
	}

	enabled := true
	return mcp.ServerEntry{
		Name:      ServerName,
		Transport: mcp.TransportStdio,
		Command:   cmd,
		Args:      append([]string(nil), args...),
		Env:       append([]string(nil), c.Env...),
		Enabled:   &enabled,
		Allowlist: append([]string(nil), c.Allowlist...),
	}, true
}
