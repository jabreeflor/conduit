// Package tui contains the terminal surface for Conduit.
package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/jabreeflor/conduit/internal/core"
)

// formatStatusBar returns the "[Model] [Tokens] [$X.XX]" string for the status line.
func formatStatusBar(model string, totalTokens int, totalCostUSD float64) string {
	return fmt.Sprintf("[%s] [%d tokens] [$%.4f]", model, totalTokens, totalCostUSD)
}

// Run starts the terminal surface. The real Bubble Tea application will land
// after the TUI stack decision; this boot path proves the core/surface contract.
func Run(out io.Writer) error {
	engine := core.New("dev")
	info := engine.Info()

	surfaces := make([]string, 0, len(info.Surfaces))
	for _, surface := range info.Surfaces {
		surfaces = append(surfaces, string(surface))
	}

	modelStatus := engine.ModelStatus()
	usageSummary := engine.UsageSummary()
	statusBar := formatStatusBar(modelStatus.SelectedModel, usageSummary.TotalTokens, usageSummary.TotalCostUSD)

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
