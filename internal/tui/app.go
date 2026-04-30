// Package tui contains the terminal surface for Conduit.
package tui

import (
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jabreeflor/conduit/internal/contracts"
	"github.com/jabreeflor/conduit/internal/core"
)

// formatStatusBar returns the status line from a UsageSummary.
// It always shows model, session cost, and session ID; it appends workflow
// only when one is active.
func formatStatusBar(s contracts.UsageSummary) string {
	parts := []string{
		fmt.Sprintf("[%s]", s.Model),
		fmt.Sprintf("[$%.4f]", s.TotalCostUSD),
		fmt.Sprintf("[session:%s]", s.SessionID),
	}
	if s.ActiveWorkflow != "" {
		parts = append(parts, fmt.Sprintf("[workflow:%s]", s.ActiveWorkflow))
	}
	return strings.Join(parts, " ")
}

// Run is the non-interactive boot path that proves the core/surface contract.
// Used by tests and piped output. For the interactive TUI, call RunInteractive.
func Run(out io.Writer) error {
	engine := core.New("dev")
	info := engine.Info()

	surfaces := make([]string, 0, len(info.Surfaces))
	for _, surface := range info.Surfaces {
		surfaces = append(surfaces, string(surface))
	}

	modelStatus := engine.ModelStatus()
	usageSummary := engine.UsageSummary()
	usageSummary.Model = modelStatus.SelectedModel
	statusBar := formatStatusBar(usageSummary)

	_, err := fmt.Fprintf(
		out,
		"%s core online (%s)\nstatus: model %s; escalates to %s\n%s\n",
		info.Name,
		strings.Join(surfaces, ", "),
		modelStatus.SelectedModel,
		modelStatus.EscalationModel,
		statusBar,
	)
	return err
}

// RunInteractive launches the full Bubble Tea three-panel TUI. It takes over
// the terminal (alt screen) until the user quits with esc or ctrl+c.
func RunInteractive() error {
	engine := core.New("dev")
	modelStatus := engine.ModelStatus()

	m := newModel(modelStatus.SelectedModel)
	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	_, err := p.Run()
	return err
}
