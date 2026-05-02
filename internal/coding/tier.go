// Package coding owns Conduit's `conduit code` REPL — the dedicated coding
// agent entry point: tier-gated tool set, context budgeting, and session
// persistence.
package coding

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jabreeflor/conduit/internal/contracts"
	"github.com/jabreeflor/conduit/internal/tools"
	"github.com/jabreeflor/conduit/internal/tools/websearch"
)

// TieredTool wraps a tools.Tool with the coding tier that controls whether
// the user has consented to register it. Tier is layered on top of the
// pipeline policy so we can build a permitted slice once at REPL start
// rather than re-evaluating per-call.
type TieredTool struct {
	Tool tools.Tool
	Tier contracts.CodingTier
}

// RegisterCodingTools filters base by tier against the user's permissions
// and returns a tools.Tool slice ready to feed tools.NewPipeline. Always
// tools are unconditionally included; write/shell tiers require explicit
// opt-in flags so a default `conduit code` invocation cannot mutate the
// host filesystem or run arbitrary commands.
func RegisterCodingTools(base []TieredTool, perms contracts.CodingPermissions) []tools.Tool {
	out := make([]tools.Tool, 0, len(base))
	for _, t := range base {
		switch t.Tier {
		case contracts.CodingTierAlways:
			out = append(out, t.Tool)
		case contracts.CodingTierRequiresWrite:
			if perms.AllowWrite {
				out = append(out, t.Tool)
			}
		case contracts.CodingTierRequiresShell:
			if perms.AllowShell {
				out = append(out, t.Tool)
			}
		}
	}
	return out
}

// DefaultCodingTools returns the PRD §6.24.4 coding tool set.
// web_search is fully implemented (PRD §6.25.7); remaining tools are stubs
// whose real runners arrive in follow-up PRs.
func DefaultCodingTools() []TieredTool {
	alwaysStubs := []string{
		"list_dir",
		"read_file",
		"glob_search",
		"grep_search",
		"web_fetch",
		"tool_search",
		"sleep",
	}
	requiresWrite := []string{"write_file", "edit_file", "notebook_edit"}
	requiresShell := []string{"bash"}

	out := make([]TieredTool, 0, len(alwaysStubs)+1+len(requiresWrite)+len(requiresShell))
	for _, name := range alwaysStubs {
		out = append(out, newStubTool(name, contracts.CodingTierAlways))
	}
	out = append(out, TieredTool{
		Tool: websearch.New(websearch.DefaultConfig()),
		Tier: contracts.CodingTierAlways,
	})
	for _, name := range requiresWrite {
		out = append(out, newStubTool(name, contracts.CodingTierRequiresWrite))
	}
	for _, name := range requiresShell {
		out = append(out, newStubTool(name, contracts.CodingTierRequiresShell))
	}
	return out
}

// DefaultCodingToolsWithConfig returns DefaultCodingTools with the web_search
// tool configured from cfg instead of the package defaults.
func DefaultCodingToolsWithConfig(wsCfg websearch.Config) []TieredTool {
	tools := DefaultCodingTools()
	for i, t := range tools {
		if t.Tool.Name == "web_search" {
			tools[i].Tool = websearch.New(wsCfg)
			break
		}
	}
	return tools
}

func newStubTool(name string, tier contracts.CodingTier) TieredTool {
	return TieredTool{
		Tool: tools.Tool{
			Name:        name,
			Description: fmt.Sprintf("coding-agent stub for %s (not yet implemented)", name),
			// Placeholder schema: per-tool schemas land alongside their
			// runners. The pipeline.Normalize call still needs a non-nil
			// object so providers receive a valid descriptor.
			Schema: map[string]any{"type": "object"},
			Run: func(_ context.Context, _ json.RawMessage) (tools.Result, error) {
				return tools.Result{Text: fmt.Sprintf("stub: %s not yet implemented", name)}, nil
			},
		},
		Tier: tier,
	}
}
