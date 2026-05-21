package help

// First-launch guided tour. The TUI / GUI host calls help.Default.Tour() on
// first run and walks the user through these steps, highlighting the named
// element each time.
func init() {
	Default.SetTour([]TourStep{
		{
			Element: "tui.input",
			Title:   "Welcome to Conduit",
			Body: `This is your prompt. Type a message and press Enter to send.
Conduit picks a provider for you based on cost and capability.`,
		},
		{
			Element: "tui.status_bar",
			Title:   "Status bar",
			Body: `The status bar shows the active provider, model, session,
and running cost. Hover any item for details.`,
		},
		{
			Element: "tui.status_bar.session",
			Title:   "Sessions",
			Body: `Every conversation is saved as JSONL on disk. Press Ctrl+S
to browse, fork, or replay past sessions.`,
		},
		{
			Element: "tui.status_bar.permission",
			Title:   "Coding agent",
			Body: `Run ` + "`conduit code`" + ` for a coding REPL with file and
shell tools. The permission tier here controls what's allowed.`,
		},
		{
			Element: "tui.input",
			Title:   "Help is one slash away",
			Body: `Type ` + "`/help`" + ` any time for an index, or
` + "`/help <topic>`" + ` for a specific area. ` + "`/help search <query>`" + `
runs a full-text search.`,
		},
	})
}
