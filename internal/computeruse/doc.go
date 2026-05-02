// Package computeruse wires the open-codex-computer-use MCP server
// (https://github.com/iFurySt/open-codex-computer-use) into Conduit's MCP
// runtime so the agent can drive macOS via the Accessibility API and
// Screen Recording.
//
// This package owns the small surface that PRD §6.8 needs at the runtime
// level: the canonical server name, the default stdio launch command,
// platform gating (macOS-only by default), and a translator from a config
// flag into an mcp.ServerEntry.
//
// Sibling work — issues #38 (permissions onboarding), #39 (per-app
// approval), #40 (capability adapters), #41 (pre/post screenshots), and
// #42 (safety guardrails) — extends this package rather than reaching
// into mcp internals. Keep additions here narrow and clearly named.
package computeruse
