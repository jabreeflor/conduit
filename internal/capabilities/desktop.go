package capabilities

// DesktopCapability is the "Desktop" tier of PRD §6.8 — full macOS GUI
// control (click, type, scroll, screenshot) via the open-codex-computer-use
// MCP server (issue #37). The adapter does not import or launch the
// open-codex-computer-use server itself; #37 owns the MCP wiring. This
// adapter is a thin proxy that delegates to whatever MCP server name #37
// registered (default: "open-codex-computer-use").
//
// When that server is not present, the adapter cleanly reports
// UnavailableError("Desktop capability unavailable: ...") rather than
// crashing the harness. This decoupling is what lets #37 and #40 land in
// either order.
type DesktopCapability struct{ *mcpProxy }

// NewDesktopCapability builds the Desktop adapter. factory may be nil — the
// adapter will then report itself as unavailable, matching the behavior the
// PRD requires when computer use has not been opt-ed into.
func NewDesktopCapability(cfg Config, approval Approval, factory MCPClientFactory) *DesktopCapability {
	return &DesktopCapability{mcpProxy: newMCPProxy(KindDesktop, cfg.resolveServerName(KindDesktop), approval, factory)}
}
