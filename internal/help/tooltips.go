package help

// Built-in tooltips for TUI / GUI elements. Element IDs follow a
// `<surface>.<area>.<thing>` convention so we can grep for "tui.*" or
// "gui.*" easily. Surface code does:
//
//	tip, ok := help.Default.Tooltip("tui.status_bar.cost")
//	if ok { render(tip.Text) }
func init() {
	for _, t := range builtinTooltips {
		Default.AddTooltip(t)
	}
}

var builtinTooltips = []Tooltip{
	// Status bar
	{Element: "tui.status_bar.provider", Text: "Active provider for this turn. Click to switch.", Topic: "providers"},
	{Element: "tui.status_bar.model", Text: "Model the router picked. Cascade may upgrade it on low confidence.", Topic: "router"},
	{Element: "tui.status_bar.cost", Text: "Running cost for this session, from the local usage ledger.", Topic: "usage"},
	{Element: "tui.status_bar.tokens", Text: "Token usage so far this turn. Includes cached input.", Topic: "usage"},
	{Element: "tui.status_bar.session", Text: "Current session ID. Press Ctrl+S to fork or load another.", Topic: "sessions"},
	{Element: "tui.status_bar.permission", Text: "Coding-agent permission tier. Tools above this tier require approval.", Topic: "coding"},

	// Settings — providers
	{Element: "gui.settings.providers.priority", Text: "Lower number = tried first. Ties broken by latency.", Topic: "router"},
	{Element: "gui.settings.providers.budget", Text: "Hard cap per day. The router skips this provider once hit.", Topic: "providers"},

	// Settings — coding
	{Element: "gui.settings.coding.permission_tier", Text: "Default permission tier for `conduit code`. Higher tiers expand the toolset.", Topic: "coding"},
	{Element: "gui.settings.coding.auto_continue", Text: "When a model output is truncated, automatically request continuation.", Topic: "coding"},

	// Settings — memory
	{Element: "gui.settings.memory.provider", Text: "Storage backend for memory entries. SQLite has FTS5; LanceDB adds vector search.", Topic: "memory"},

	// Friction points (these surface a "Learn more" affordance)
	{Element: "tui.friction.no_provider", Text: "No provider is configured. Add one with `conduit provider add` or via Settings.", Topic: "providers"},
	{Element: "tui.friction.permission_denied", Text: "This tool needs a higher permission tier than your current setting.", Topic: "coding"},
	{Element: "tui.friction.budget_exceeded", Text: "All available providers are over their daily budget cap.", Topic: "providers"},
}
