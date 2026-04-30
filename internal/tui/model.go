package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── message types ─────────────────────────────────────────────────────────────

type tokenMsg string   // streamed token arrives
type toolDoneMsg int   // tool at index i finishes
type tickMsg time.Time // drives streaming simulation

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
	icon := styleToolRunning.Render("⟳")
	switch t.status {
	case toolDone:
		icon = styleToolDone.Render("✓")
	case toolFailed:
		icon = styleToolFail.Render("✗")
	}
	toggle := styleDim.Render("▶")
	if t.expanded {
		toggle = styleDim.Render("▼")
	}
	name := lipgloss.NewStyle().Foreground(lipgloss.Color("#9d79d6")).Render(t.name)
	header := fmt.Sprintf("%s %s %s %s", toggle, icon, styleAgent.Render("tool:"), name)
	if !t.expanded {
		return header
	}
	return header + "\n" + styleDim.Render("  input: "+t.input)
}

// ── chat message ──────────────────────────────────────────────────────────────

type role int

const (
	roleUser role = iota
	roleAgent
)

type message struct {
	role role
	text string
}

// ── key bindings ─────────────────────────────────────────────────────────────

type keyMap struct {
	TogglePanel key.Binding
	Quit        key.Binding
	Submit      key.Binding
	ExpandTool  key.Binding
	SetupLocal  key.Binding
	ExternalAPI key.Binding
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
	SetupLocal: key.NewBinding(
		key.WithKeys("l"),
		key.WithHelp("l", "set up local ai"),
	),
	ExternalAPI: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "external api"),
	),
}

// ── model ─────────────────────────────────────────────────────────────────────

// Model is the Bubble Tea application state for the three-panel layout.
type Model struct {
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
	setup        contracts.FirstRunSetupSnapshot
	setupLocalAI func() (contracts.FirstRunSetupSnapshot, error)
	streaming    bool
	streamBuffer string
	tickCount    int
}

func newModel(activeModel string, setup contracts.FirstRunSetupSnapshot, setupLocalAI func() (contracts.FirstRunSetupSnapshot, error)) Model {
	ta := textarea.New()
	ta.Placeholder = "Message conduit..."
	ta.Focus()
	ta.SetWidth(80)
	ta.SetHeight(2)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetKeys("shift+enter")

	return Model{
		input:        ta,
		showContext:  true,
		activeModel:  activeModel,
		setup:        setup,
		setupLocalAI: setupLocalAI,
		messages: []message{
			{role: roleAgent, text: "Welcome to Conduit."},
		},
	}
}

func (m Model) Init() tea.Cmd {
	return textarea.Blink
}

func tick() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// ── update ────────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
				m = m.refreshContent()
			}
		case key.Matches(msg, keys.SetupLocal):
			if m.setup.Phase == contracts.FirstRunSetupPhaseWelcome && m.setup.Recommendation.ID != "" {
				if m.setupLocalAI != nil {
					setup, err := m.setupLocalAI()
					if err != nil {
						m.setup = setup
						m.messages = append(m.messages, message{role: roleAgent, text: "Local AI setup needs attention: " + err.Error()})
						m = m.refreshContent()
						break
					}
					m.setup = setup
				} else {
					m.setup.Phase = contracts.FirstRunSetupPhaseReady
					m.setup.Ready = true
					for i := range m.setup.Steps {
						m.setup.Steps[i].Status = contracts.FirstRunSetupStepDone
					}
				}
				m.messages = append(m.messages, message{role: roleAgent, text: "Local AI is ready. You can start a session with the recommended model now."})
				m = m.refreshContent()
			}
		case key.Matches(msg, keys.ExternalAPI):
			if m.setup.Phase == contracts.FirstRunSetupPhaseWelcome {
				m.setup.Phase = contracts.FirstRunSetupPhaseExternal
				m.messages = append(m.messages, message{role: roleAgent, text: "External API setup selected. Add a provider key to continue without local model downloads."})
				m = m.refreshContent()
			}
		case key.Matches(msg, keys.Submit):
			if text := strings.TrimSpace(m.input.Value()); text != "" {
				m.messages = append(m.messages, message{role: roleUser, text: text})
				m.input.Reset()
				m.streaming = true
				m.streamBuffer = ""
				m = m.refreshContent()
				m.conversation.GotoBottom()
			}
		}

	case tickMsg:
		if m.streaming {
			tokens := []string{"Sure", "!", " I", " can", " help", " with", " that", "."}
			if m.tickCount < len(tokens) {
				m.streamBuffer += tokens[m.tickCount]
				m.tickCount++
				m = m.refreshContent()
				m.conversation.GotoBottom()
				cmds = append(cmds, tick())
			} else {
				m.messages = append(m.messages, message{role: roleAgent, text: m.streamBuffer})
				m.streaming = false
				m.streamBuffer = ""
				m.tickCount = 0
				m = m.refreshContent()
			}
		}

	case toolDoneMsg:
		idx := int(msg)
		if idx < len(m.toolCalls) {
			m.toolCalls[idx].status = toolDone
			m = m.refreshContent()
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
