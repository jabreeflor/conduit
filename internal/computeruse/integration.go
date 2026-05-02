package computeruse

import (
	"github.com/jabreeflor/conduit/internal/config"
	"github.com/jabreeflor/conduit/internal/mcp"
)

// FromRootConfig translates the root Conduit config's computer_use block
// into the package-local Config. The two structs are kept separate so
// ForceNonDarwin and other runtime knobs can grow here without bloating
// the root YAML schema.
func FromRootConfig(cfg config.Config) Config {
	c := cfg.ComputerUse
	return Config{
		Enabled:        c.Enabled,
		Command:        c.Command,
		Args:           append([]string(nil), c.Args...),
		Env:            append([]string(nil), c.Env...),
		Allowlist:      append([]string(nil), c.Allowlist...),
		ForceNonDarwin: c.ForceNonDarwin,
	}
}

// MergeInto returns mcpCfg with the open-computer-use server entry
// appended (or replacing an existing same-named entry) when the
// integration is active. When inactive, mcpCfg is returned unchanged.
//
// User-supplied entries with the same name in mcpCfg take precedence —
// this lets advanced users pin a specific binary path or transport
// without losing the rest of their MCP config.
func MergeInto(mcpCfg mcp.Config, c Config) mcp.Config {
	entry, ok := c.ServerEntry()
	if !ok {
		return mcpCfg
	}

	for _, existing := range mcpCfg.Servers {
		if existing.Name == ServerName {
			// User-defined entry wins.
			return mcpCfg
		}
	}

	mcpCfg.Servers = append(mcpCfg.Servers, entry)
	return mcpCfg
}
