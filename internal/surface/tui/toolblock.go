package tui

import (
	"strings"

	"github.com/jabreeflor/conduit/internal/contracts"
)

const (
	iconRunning   = "⟳"
	iconDone      = "✓"
	iconFailed    = "✗"
	iconCollapsed = "▶"
	iconExpanded  = "▼"
)

// RenderToolBlock returns the display string for one tool call.
// Collapsed: "▶ ⟳ tool: name"
// Expanded:  header + indented input (and output if present)
func RenderToolBlock(tc contracts.ToolCall) string {
	statusIcon := iconRunning
	switch tc.Status {
	case contracts.ToolStatusDone:
		statusIcon = iconDone
	case contracts.ToolStatusFailed:
		statusIcon = iconFailed
	}

	toggleIcon := iconCollapsed
	if tc.Expanded {
		toggleIcon = iconExpanded
	}

	header := toggleIcon + " " + statusIcon + " tool: " + tc.Name

	if !tc.Expanded {
		return header
	}

	var sb strings.Builder
	sb.WriteString(header)
	if tc.Input != "" {
		sb.WriteString("\n  input:  " + tc.Input)
	}
	if tc.Output != "" {
		sb.WriteString("\n  output: " + tc.Output)
	}
	return sb.String()
}
