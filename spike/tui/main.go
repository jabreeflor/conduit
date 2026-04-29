package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── styles ────────────────────────────────────────────────────────────────────

var (
	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#1a1a2e")).
			Foreground(lipgloss.Color("#a0a0b0")).
			Padding(0, 1)

	modelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7c6af7")).
			Bold(true)

	costStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4ecca3"))

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#2a2a3e"))

	activePanelStyle = panelStyle.
				BorderForeground(lipgloss.Color("#7c6af7"))

	userMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffffff")).
			Bold(true)

	agentMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#a0a0b0"))

	toolRunningStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#f4c542"))

	toolDoneStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4ecca3"))

	toolFailStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#e05c5c"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#555566"))
)

// ── message types ─────────────────────────────────────────────────────────────

type tokenMsg string       // a streamed token arrives
type toolDoneMsg int       // tool at index i finishes
type tickMsg time.Time     // drives the streaming simulation

// ── tool call ─────────────────────────────────────────────────────────────────

type toolStatus int

const (
	toolRunning toolStatus = iota
	toolDone
	toolFailed
)

type toolCall struct {
	name     string
	input    string
	status   toolStatus
	expanded bool
}

func (t toolCall) render() string {
	icon := toolRunningStyle.Render("⟳")
	switch t.status {
	case toolDone:
		icon = toolDoneStyle.Render("✓")
	case toolFailed:
		icon = toolFailStyle.Render("✗")
	}
	toggle := dimStyle.Render("▶")
	if t.expanded {
		toggle = dimStyle.Render("▼")
	}
	header := fmt.Sprintf("%s %s %s %s", toggle, icon, agentMsgStyle.Render("tool:"), lipgloss.NewStyle().Foreground(lipgloss.Color("#9d79d6")).Render(t.name))
	if !t.expanded {
		return header
	}
	return header + "\n" + dimStyle.Render("  input: "+t.input)
}

// ── chat message ──────────────────────────────────────────────────────────────

type role int

const (
	roleUser role = iota
	roleAgent
	roleTool
)

type message struct {
	role     role
	text     string
	toolIdx  int // only for roleTool
}

// ── key bindings ─────────────────────────────────────────────────────────────

type keyMap struct {
	TogglePanel key.Binding
	Quit        key.Binding
	Submit      key.Binding
	ExpandTool  key.Binding
}

var keys = keyMap{
	TogglePanel: key.NewBinding(
		key.WithKeys("ctrl+p"),
		key.WithHelp("ctrl+p", "toggle context panel"),
	),
	Quit: key.NewBinding(
		key.WithKeys("esc", "ctrl+c"),
		key.WithHelp("esc", "quit"),
	),
	Submit: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "send"),
	),
	ExpandTool: key.NewBinding(
		key.WithKeys("x"),
		key.WithHelp("x", "expand/collapse last tool call"),
	),
}

// ── model ─────────────────────────────────────────────────────────────────────

type model struct {
	width        int
	height       int
	conversation viewport.Model
	contextPanel viewport.Model
	input        textarea.Model
	messages     []message
	toolCalls    []toolCall
	showContext  bool
	sessionCost  float64
	activeModel  string
	streaming    bool
	streamBuffer string
	tickCount    int
}

func initialModel() model {
	ta := textarea.New()
	ta.Placeholder = "Message conduit..."
	ta.Focus()
	ta.SetWidth(80)
	ta.SetHeight(2)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetKeys("shift+enter")

	m := model{
		input:       ta,
		showContext: true,
		activeModel: "claude-opus-4-6",
		sessionCost: 0.04,
		messages: []message{
			{role: roleAgent, text: "Hello! I'm Conduit. How can I help you today?"},
		},
		toolCalls: []toolCall{
			{name: "read_file", input: `{"path": "~/conduit/docs/PRD.md"}`, status: toolDone},
			{name: "web_search", input: `{"query": "bubbletea tui layout"}`, status: toolRunning},
		},
	}
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, tick())
}

func tick() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// ── update ────────────────────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = m.recalculateLayout()

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.TogglePanel):
			m.showContext = !m.showContext
			m = m.recalculateLayout()
		case key.Matches(msg, keys.ExpandTool):
			if len(m.toolCalls) > 0 {
				last := len(m.toolCalls) - 1
				m.toolCalls[last].expanded = !m.toolCalls[last].expanded
				m = m.refreshConversation()
			}
		case key.Matches(msg, keys.Submit):
			if text := strings.TrimSpace(m.input.Value()); text != "" {
				m.messages = append(m.messages, message{role: roleUser, text: text})
				m.input.Reset()
				m.streaming = true
				m.streamBuffer = ""
				m = m.refreshConversation()
				m.conversation.GotoBottom()
			}
		}

	case tickMsg:
		if m.streaming {
			tokens := []string{"Sure", "!", " I", " can", " help", " with", " that", ".", " Let", " me", " look", " into", " it", " now", "..."}
			if m.tickCount < len(tokens) {
				m.streamBuffer += tokens[m.tickCount]
				m.tickCount++
				m = m.refreshConversation()
				m.conversation.GotoBottom()
				cmds = append(cmds, tick())
			} else {
				m.messages = append(m.messages, message{role: roleAgent, text: m.streamBuffer})
				m.streaming = false
				m.streamBuffer = ""
				m.tickCount = 0
				m = m.refreshConversation()
			}
		}

	case toolDoneMsg:
		idx := int(msg)
		if idx < len(m.toolCalls) {
			m.toolCalls[idx].status = toolDone
			m = m.refreshConversation()
		}
	}

	var inputCmd tea.Cmd
	m.input, inputCmd = m.input.Update(msg)
	cmds = append(cmds, inputCmd)

	var convCmd, ctxCmd tea.Cmd
	m.conversation, convCmd = m.conversation.Update(msg)
	m.contextPanel, ctxCmd = m.contextPanel.Update(msg)
	cmds = append(cmds, convCmd, ctxCmd)

	return m, tea.Batch(cmds...)
}

// ── layout helpers ────────────────────────────────────────────────────────────

func (m model) recalculateLayout() model {
	statusH := 1
	inputH := 3
	panelH := m.height - statusH - inputH - 4 // 4 for borders

	conversationW := m.width - 2
	if m.showContext {
		conversationW = (m.width * 60 / 100) - 2
	}
	contextW := (m.width * 40 / 100) - 2

	m.conversation = viewport.New(conversationW, panelH)
	m.conversation.SetContent(m.conversationContent())

	m.contextPanel = viewport.New(contextW, panelH)
	m.contextPanel.SetContent(m.contextContent())

	m.input.SetWidth(m.width - 4)
	return m
}

func (m model) refreshConversation() model {
	m.conversation.SetContent(m.conversationContent())
	m.contextPanel.SetContent(m.contextContent())
	return m
}

func (m model) conversationContent() string {
	var sb strings.Builder
	for _, msg := range m.messages {
		switch msg.role {
		case roleUser:
			sb.WriteString(userMsgStyle.Render("you  ") + msg.text + "\n\n")
		case roleAgent:
			sb.WriteString(agentMsgStyle.Render("conduit  ") + msg.text + "\n\n")
		}
	}
	for _, tc := range m.toolCalls {
		sb.WriteString(tc.render() + "\n\n")
	}
	if m.streaming && m.streamBuffer != "" {
		sb.WriteString(agentMsgStyle.Render("conduit  ") + m.streamBuffer + "▌\n")
	}
	return sb.String()
}

func (m model) contextContent() string {
	var sb strings.Builder
	sb.WriteString(dimStyle.Render("── workflow ──────────────────") + "\n\n")
	steps := []struct {
		n    int
		name string
		done bool
	}{
		{1, "read context files", true},
		{2, "search bubbletea docs", false},
		{3, "write spike code", false},
	}
	for _, s := range steps {
		icon := toolDoneStyle.Render("✓")
		nameStyle := dimStyle
		if !s.done {
			icon = toolRunningStyle.Render("⟳")
			nameStyle = agentMsgStyle
		}
		sb.WriteString(fmt.Sprintf(" %s %s. %s\n", icon, dimStyle.Render(fmt.Sprintf("%d", s.n)), nameStyle.Render(s.name)))
	}
	sb.WriteString("\n" + dimStyle.Render("── session ───────────────────") + "\n\n")
	sb.WriteString(agentMsgStyle.Render(" session-abc1234\n"))
	sb.WriteString(dimStyle.Render(" 3 turns · 2 tool calls\n"))
	return sb.String()
}

// ── view ──────────────────────────────────────────────────────────────────────

func (m model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	// status bar
	cost := costStyle.Render(fmt.Sprintf("$%.4f", m.sessionCost))
	mod := modelStyle.Render(m.activeModel)
	session := dimStyle.Render("session-abc1234")
	gap := strings.Repeat(" ", max(0, m.width-lipgloss.Width(mod)-lipgloss.Width(cost)-lipgloss.Width(session)-6))
	statusBar := statusBarStyle.Width(m.width).Render(
		fmt.Sprintf(" %s  %s%s%s ", mod, cost, gap, session),
	)

	// panels
	convPanel := activePanelStyle.Render(m.conversation.View())

	var mainRow string
	if m.showContext {
		ctxPanel := panelStyle.Render(m.contextPanel.View())
		mainRow = lipgloss.JoinHorizontal(lipgloss.Top, convPanel, ctxPanel)
	} else {
		mainRow = convPanel
	}

	// input
	help := dimStyle.Render(" enter:send  ctrl+p:panel  x:expand  esc:quit")
	inputBox := panelStyle.Render(m.input.View())
	inputRow := lipgloss.JoinVertical(lipgloss.Left, inputBox, help)

	return lipgloss.JoinVertical(lipgloss.Left, statusBar, mainRow, inputRow)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	p := tea.NewProgram(
		initialModel(),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
