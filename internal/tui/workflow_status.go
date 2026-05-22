// Package tui — workflow status view.
//
// This file renders a numbered list of workflow steps with their status,
// per-step cost, and per-step duration. It is intentionally decoupled from
// the (in-progress) `internal/workflow` package: the view consumes a small
// local interface (StepView / WorkflowStatus) so it can compile and be tested
// before the foundational workflow types land. After those types ship, an
// adapter in the workflow package can satisfy these interfaces.
//
// Issue: #36 — PRD §19 Phase 3.
package tui

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// StepStatus values are declared in context_panel.go: StepPending,
// StepRunning, StepDone, StepFailed. We reuse them here so the workflow
// surface is consistent across the context panel and the dedicated status
// view, and so this file does not duplicate those declarations.

// StepView is the minimal contract the status renderer needs from a step.
// Defined locally so this package does not depend on the unfinished
// internal/workflow package (issue #33).
type StepView interface {
	Name() string
	Status() StepStatus
	// CostUSD returns the cost in USD spent on this step. Zero means "no cost
	// yet" and is omitted from the rendered line.
	CostUSD() float64
	// Duration returns the elapsed time for this step. Zero means "not yet
	// timed" and is omitted from the rendered line.
	Duration() time.Duration
}

// WorkflowStatus is the minimal contract for a workflow's view state.
type WorkflowStatus interface {
	Steps() []StepView
	// CurrentStep is the 0-based index of the active step, or -1 if no step
	// is currently running (e.g. the workflow has not started or is done).
	CurrentStep() int
}

// ANSI escape codes. Kept local so we don't pull in a TUI library — matches
// the hand-rolled ANSI style of the rest of internal/tui.
const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiRed   = "\x1b[31m"
	ansiGreen = "\x1b[32m"
	ansiCyan  = "\x1b[36m"
	ansiDim   = "\x1b[2m"
)

// statusGlyph returns the icon + ANSI styling envelope for a given status.
// The current/highlighted step is rendered bold cyan when running.
func statusGlyph(s StepStatus) string {
	switch s {
	case StepRunning:
		return ansiBold + ansiCyan + "▶" + ansiReset // ▶
	case StepDone:
		return ansiGreen + "●" + ansiReset // ●
	case StepFailed:
		return ansiRed + "✕" + ansiReset // ✕
	case StepPending:
		fallthrough
	default:
		return ansiDim + "○" + ansiReset // ○
	}
}

// formatDuration renders a duration as a compact human-readable string.
//
//	1.5ns   -> "0ms"
//	450ms   -> "450ms"
//	1.25s   -> "1.2s"
//	75s     -> "1m15s"
//	3725s   -> "1h2m"
//
// Anything zero or negative renders as the empty string so the caller can
// elide the field entirely.
func formatDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	if d < time.Second {
		ms := d / time.Millisecond
		return fmt.Sprintf("%dms", ms)
	}
	if d < time.Minute {
		// e.g. 1.2s
		secs := float64(d) / float64(time.Second)
		return fmt.Sprintf("%.1fs", secs)
	}
	if d < time.Hour {
		m := int(d / time.Minute)
		s := int((d % time.Minute) / time.Second)
		return fmt.Sprintf("%dm%ds", m, s)
	}
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	return fmt.Sprintf("%dh%dm", h, m)
}

// formatCost renders a cost as "$0.0123". Matches the style used by
// formatStatusBar in app.go (4 decimal places). Zero costs return "" so the
// caller can omit the field.
func formatCost(usd float64) string {
	if usd <= 0 {
		return ""
	}
	return fmt.Sprintf("$%.4f", usd)
}

// WorkflowStatusView renders a WorkflowStatus to a writer.
//
// Output format (one line per step):
//
//  1. step name [icon] $0.0123 1.2s
//
// The current step is highlighted bold-cyan via the icon and name.
// Empty cost and empty duration are omitted (no trailing whitespace).
type WorkflowStatusView struct {
	// NoColor disables ANSI escape codes. Useful for plain logs / CI / tests.
	NoColor bool
}

// Render writes the status block for w to out.
func (v WorkflowStatusView) Render(out io.Writer, w WorkflowStatus) error {
	if w == nil {
		return nil
	}
	steps := w.Steps()
	current := w.CurrentStep()

	var b strings.Builder
	for i, s := range steps {
		isCurrent := i == current && s.Status() == StepRunning
		icon := statusGlyph(s.Status())
		name := s.Name()
		if isCurrent {
			name = ansiBold + ansiCyan + name + ansiReset
		}

		line := fmt.Sprintf("%d. %s %s", i+1, icon, name)

		if c := formatCost(s.CostUSD()); c != "" {
			line += " " + c
		}
		if d := formatDuration(s.Duration()); d != "" {
			line += " " + d
		}

		if v.NoColor {
			line = stripANSI(line)
		}

		b.WriteString(line)
		b.WriteByte('\n')
	}

	_, err := io.WriteString(out, b.String())
	return err
}

// stripANSI removes CSI escape sequences (ESC [ ... letter) from s.
// Conservative: handles the small set of escapes statusGlyph emits.
func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			// Skip until we find a letter (final byte of CSI sequence).
			j := i + 2
			for j < len(s) {
				c := s[j]
				if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
					j++
					break
				}
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
