// Package computeruse holds the runtime hooks that gate Conduit's macOS
// computer-use capability (PRD §6.8).
//
// This package wires the open-codex-computer-use MCP server
// (https://github.com/iFurySt/open-codex-computer-use) into Conduit's MCP
// runtime so the agent can drive macOS via the Accessibility API and
// Screen Recording. It owns the canonical server name, the default stdio
// launch command, platform gating (macOS-only by default), and a
// translator from a config flag into an mcp.ServerEntry.
//
// It also owns the per-app approval surface (#39): grants are persisted
// to ~/.conduit/approved-apps.json so revocations and grants survive
// across processes. The on-disk format is documented on ApprovalRecord
// and is intentionally human-readable: users may audit or hand-edit it
// just like the credentials/usage files.
//
// Sibling computer-use modules (capability adapters in #40, pre/post
// screenshots in #41, safety guardrails in #42) integrate via the
// hooks exposed here. Keep additions narrow and clearly named.
package computeruse
