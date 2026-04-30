package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// ── layout ────────────────────────────────────────────────────────────────────

const (
	statusBarHeight = 1
	inputHeight     = 3
	borderPad       = 4 // top+bottom borders on conversation and context panels
	// conversationRatio is the fraction of terminal width the conversation panel takes
	// when the context panel is visible. The context panel gets (1 - conversationRatio).
	conversationRatio = 0.60
	// minContextWidth is the narrowest the context panel will render before hiding.
	minContextWidth = 20
)

// recalculateLayout recomputes viewport and input dimensions after a resize or
// after the context panel is toggled.
func (m Model) recalculateLayout() Model {
	panelH := m.height - statusBarHeight - inputHeight - borderPad

	convW, ctxW := panelWidths(m.width, m.showContext)

	m.conversation = viewport.New(convW, panelH)
	m.conversation.SetContent(m.conversationContent())

	m.contextPanel = viewport.New(ctxW, panelH)
	m.contextPanel.SetContent(m.contextContent())

	m.input.SetWidth(m.width - borderPad)
	return m
}

// panelWidths returns the inner widths (excluding borders) of the conversation
// and context panels. When showContext is false or the terminal is too narrow,
// ctxW is zero and the conversation panel takes the full width.
func panelWidths(totalW int, showContext bool) (convW, ctxW int) {
	if !showContext {
		return totalW - 2, 0
	}
	proposed := int(float64(totalW) * (1 - conversationRatio))
	if proposed < minContextWidth {
		// Terminal is too narrow to split — hide context automatically.
		return totalW - 2, 0
	}
	convW = int(float64(totalW)*conversationRatio) - 2
	ctxW = totalW - convW - 4 // account for both panels' borders
	return convW, ctxW
}

// refreshContent re-renders viewport content without resizing.
func (m Model) refreshContent() Model {
	m.conversation.SetContent(m.conversationContent())
	m.contextPanel.SetContent(m.contextContent())
	return m
}

// ── content renderers ─────────────────────────────────────────────────────────

func (m Model) conversationContent() string {
	var sb strings.Builder
	if m.setup.Phase != "" {
		sb.WriteString(m.setupWelcomeContent() + "\n\n")
	}
	for _, msg := range m.messages {
		switch msg.role {
		case roleUser:
			sb.WriteString(styleUser.Render("you  ") + msg.text + "\n\n")
		case roleAgent:
			sb.WriteString(styleAgent.Render("conduit  ") + msg.text + "\n\n")
		}
	}
	for _, tc := range m.toolCalls {
		sb.WriteString(tc.render() + "\n\n")
	}
	if m.streaming && m.streamBuffer != "" {
		sb.WriteString(styleAgent.Render("conduit  ") + m.streamBuffer + "▌\n")
	}
	return sb.String()
}

func (m Model) contextContent() string {
	var sb strings.Builder
	if len(m.setup.Steps) > 0 {
		sb.WriteString(styleDim.Render("── first run ─────────────────") + "\n\n")
		for i, step := range m.setup.Steps {
			icon := setupStepIcon(step.Status)
			nameStyle := styleAgent
			if step.Status == "pending" {
				nameStyle = styleDim
			}
			sb.WriteString(fmt.Sprintf(" %s %s. %s\n", icon, styleDim.Render(fmt.Sprintf("%d", i+1)), nameStyle.Render(step.Name)))
			if step.Detail != "" {
				sb.WriteString(styleDim.Render("    "+step.Detail) + "\n")
			}
		}
	} else {
		sb.WriteString(styleDim.Render("── workflow ──────────────────") + "\n\n")
		steps := []struct {
			n    int
			name string
			done bool
		}{
			{1, "load context files", true},
			{2, "plan approach", false},
			{3, "execute steps", false},
		}
		for _, s := range steps {
			icon := styleToolDone.Render("✓")
			nameStyle := styleDim
			if !s.done {
				icon = styleToolRunning.Render("⟳")
				nameStyle = styleAgent
			}
			sb.WriteString(fmt.Sprintf(" %s %s. %s\n", icon, styleDim.Render(fmt.Sprintf("%d", s.n)), nameStyle.Render(s.name)))
		}
	}
	sb.WriteString("\n" + styleDim.Render("── session ───────────────────") + "\n\n")
	sb.WriteString(styleAgent.Render(fmt.Sprintf(" %s\n", m.activeModel)))
	sb.WriteString(styleDim.Render(fmt.Sprintf(" %d turns · %d tool calls\n", len(m.messages), len(m.toolCalls))))
	return sb.String()
}

// ── view ──────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	statusBar := m.renderStatusBar()
	mainRow := m.renderMainRow()
	inputRow := m.renderInputRow()

	return lipgloss.JoinVertical(lipgloss.Left, statusBar, mainRow, inputRow)
}

func (m Model) renderStatusBar() string {
	mod := styleActiveModel.Render(m.activeModel)
	cost := styleCost.Render(fmt.Sprintf("$%.4f", m.sessionCost))
	gap := strings.Repeat(" ", max(0, m.width-lipgloss.Width(mod)-lipgloss.Width(cost)-6))
	return styleStatusBar.Width(m.width).Render(
		fmt.Sprintf(" %s  %s%s", mod, cost, gap),
	)
}

func (m Model) renderMainRow() string {
	convPanel := styleActivePanel.Render(m.conversation.View())
	if !m.showContext || m.contextPanel.Width == 0 {
		return convPanel
	}
	ctxPanel := stylePanel.Render(m.contextPanel.View())
	return lipgloss.JoinHorizontal(lipgloss.Top, convPanel, ctxPanel)
}

func (m Model) renderInputRow() string {
	help := " enter:send  ctrl+p:panel  x:expand  esc:quit"
	if m.setup.Phase == "welcome" {
		help = " l:local setup  a:external api  enter:send  ctrl+p:panel  esc:quit"
	}
	help = styleDim.Render(help)
	inputBox := stylePanel.Render(m.input.View())
	return lipgloss.JoinVertical(lipgloss.Left, inputBox, help)
}

func (m Model) setupWelcomeContent() string {
	rec := m.setup.Recommendation
	var sb strings.Builder
	sb.WriteString(styleAgent.Render("Welcome to Conduit") + "\n")
	sb.WriteString(fmt.Sprintf("Machine: %.0fGB RAM, %.0fGB free disk\n", m.setup.MachineProfile.Memory.TotalGB, m.setup.MachineProfile.Disk.AvailableGB))
	if rec.LocalRecommended {
		sb.WriteString(fmt.Sprintf("Set up local AI: %s via %s (%.1fGB download, ~%.0f tok/s)\n", rec.Model, rec.Runtime, rec.DownloadSizeGB, rec.EstimatedTokensPerS))
	} else {
		sb.WriteString("Local AI is not recommended for this machine.\n")
	}
	sb.WriteString("External API: " + formatExternalAPIOptions(m.setup.ExternalAPI))
	return sb.String()
}

func setupStepIcon(status interface{}) string {
	switch fmt.Sprint(status) {
	case "done":
		return styleToolDone.Render("✓")
	case "running":
		return styleToolRunning.Render("⟳")
	default:
		return styleDim.Render("○")
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
