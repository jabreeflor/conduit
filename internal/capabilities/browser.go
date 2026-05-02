package capabilities

// BrowserCapability is the "Browser" tier of PRD §6.8 — web automation via
// the Chrome MCP server. The adapter does not embed Chrome MCP; instead it
// proxies to whichever MCP server the user has registered (default name:
// "chrome") via MCPClientFactory. If that server is missing, ListTools and
// Dispatch return UnavailableError so the rest of the harness keeps working.
type BrowserCapability struct{ *mcpProxy }

// NewBrowserCapability builds the Browser adapter. factory may be nil — the
// adapter will then report itself as unavailable, which is the correct
// behavior when Chrome MCP has not been configured.
func NewBrowserCapability(cfg Config, approval Approval, factory MCPClientFactory) *BrowserCapability {
	return &BrowserCapability{mcpProxy: newMCPProxy(KindBrowser, cfg.resolveServerName(KindBrowser), approval, factory)}
}
