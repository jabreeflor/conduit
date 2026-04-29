// Package tui contains the terminal surface for Conduit.
package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/jabreeflor/conduit/internal/core"
)

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

	_, err := fmt.Fprintf(
		out,
		"%s core online (%s)\nstatus: model %s; escalates to %s\n",
		info.Name,
		strings.Join(surfaces, ", "),
		modelStatus.SelectedModel,
		modelStatus.EscalationModel,
	)
	return err
}
