package capabilities

import (
	"context"
)

// Config selects which capability adapters are active. All three default to
// false so callers must opt in explicitly. The PRD-suggested safe minimum is
// shell-only; see DefaultConfig.
type Config struct {
	// Shell enables ShellCapability (terminal commands, code execution, file
	// ops via local exec). Gated by per-app approval (#39).
	Shell bool `yaml:"shell"`

	// Browser enables BrowserCapability — a thin proxy to Chrome MCP.
	Browser bool `yaml:"browser"`

	// Desktop enables DesktopCapability — a thin proxy to the
	// open-codex-computer-use MCP server (#37).
	Desktop bool `yaml:"desktop"`

	// MCPServerNames overrides the default MCP server names this package
	// looks up via MCPClientFactory. The defaults match the PRD wiring:
	//   browser -> "chrome"
	//   desktop -> "open-codex-computer-use"
	MCPServerNames map[Kind]string `yaml:"mcp_server_names,omitempty"`

	// ShellAllowedCommands restricts which executables ShellCapability will
	// invoke. An empty slice means "all commands allowed" (subject to the
	// approval gate). Specifying e.g. ["git", "ls", "cat"] is the safest
	// default for non-interactive callers.
	ShellAllowedCommands []string `yaml:"shell_allowed_commands,omitempty"`

	// ShellAppName is the application identifier passed to the approval gate
	// when ShellCapability runs a command. Defaults to "shell".
	ShellAppName string `yaml:"shell_app_name,omitempty"`
}

// DefaultConfig returns Conduit's safe minimum: shell on, browser/desktop off.
// The active approval gate (passed via NewManager) is what actually keeps
// shell safe — DefaultConfig only flips the bit.
func DefaultConfig() Config {
	return Config{
		Shell:   true,
		Browser: false,
		Desktop: false,
	}
}

// resolveServerName returns the MCP server name to look up for a given kind,
// falling back to PRD defaults when the user has not overridden it.
func (c Config) resolveServerName(kind Kind) string {
	if c.MCPServerNames != nil {
		if v, ok := c.MCPServerNames[kind]; ok && v != "" {
			return v
		}
	}
	switch kind {
	case KindBrowser:
		return "chrome"
	case KindDesktop:
		return "open-codex-computer-use"
	default:
		return ""
	}
}

// MCPToolClient is the minimal surface this package needs from an MCP client.
// It matches the shape of internal/mcp.Client without forcing a hard import,
// so callers can pass either the production client or a fake in tests.
type MCPToolClient interface {
	ListTools(ctx context.Context) ([]MCPToolDef, error)
	CallTool(ctx context.Context, name string, args map[string]any) ([]MCPContent, error)
	Close() error
}

// MCPToolDef mirrors mcp.ToolDef. Duplicated here so the capabilities package
// has zero dependency on internal/mcp.
type MCPToolDef struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// MCPContent mirrors mcp.Content. Only the text + data fields are needed for
// capability dispatch.
type MCPContent struct {
	Type string
	Text string
	Data string
	MIME string
}

// MCPClientFactory opens a connection to a named MCP server. Returns
// (nil, nil) when the server is not configured — adapters treat this as
// "capability unavailable" rather than a fatal error.
//
// Callers wire in a real factory backed by mcp.LoadConfig + mcp.Connect; this
// package never imports internal/mcp directly so #37/#39 can ship in any order.
type MCPClientFactory func(ctx context.Context, serverName string) (MCPToolClient, error)
