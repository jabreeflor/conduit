package tui

import "github.com/charmbracelet/lipgloss"

// Conduit Design System color tokens — sourced from docs/mockups/tui-mockups.html.
const (
	colorBG        = lipgloss.Color("#0b0d10")
	colorPanel     = lipgloss.Color("#11151a")
	colorBorder    = lipgloss.Color("#2a313a")
	colorText      = lipgloss.Color("#d6dde6")
	colorMuted     = lipgloss.Color("#8a94a3")
	colorDim       = lipgloss.Color("#5b6573")
	colorAccent    = lipgloss.Color("#7cc4ff") // tool running / active panel border
	colorGood      = lipgloss.Color("#6dd58c") // tool done
	colorBad       = lipgloss.Color("#ff7a7a") // tool failed
	colorWarn      = lipgloss.Color("#f5c87a")
	colorUser      = lipgloss.Color("#ffffff")
	colorAgent     = lipgloss.Color("#b4bccb")
	colorMagenta   = lipgloss.Color("#c084fc") // active model name
	colorCyan      = lipgloss.Color("#5ee2cf") // session cost
	colorSelection = lipgloss.Color("#1b3a52")
)

var (
	styleStatusBar = lipgloss.NewStyle().
			Background(colorPanel).
			Foreground(colorMuted).
			Padding(0, 1)

	styleActiveModel = lipgloss.NewStyle().
				Foreground(colorMagenta).
				Bold(true)

	styleCost = lipgloss.NewStyle().
			Foreground(colorCyan)

	styleDim = lipgloss.NewStyle().
			Foreground(colorDim)

	// stylePanel is the default (inactive) border style.
	stylePanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder)

	// styleActivePanel highlights the focused panel.
	styleActivePanel = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorAccent)

	styleUser = lipgloss.NewStyle().
			Foreground(colorUser).
			Bold(true)

	styleAgent = lipgloss.NewStyle().
			Foreground(colorAgent)

	styleToolRunning = lipgloss.NewStyle().
				Foreground(colorAccent)

	styleToolDone = lipgloss.NewStyle().
			Foreground(colorGood)

	styleToolFail = lipgloss.NewStyle().
			Foreground(colorBad)
)
