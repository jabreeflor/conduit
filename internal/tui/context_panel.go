package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// PanelView identifies which context panel sub-view is active.
type PanelView int

const (
	PanelWorkflow    PanelView = iota // numbered workflow steps with live status
	PanelMemory                       // persistent key-value memory entries
	PanelHooks                        // log of hook invocations and results
	PanelSessionTree                  // session events and tree of forks/replays
	panelViewCount
)

// WorkflowStep is one step in the active workflow execution.
type WorkflowStep struct {
	Index  int
	Name   string
	Status StepStatus
}

// StepStatus is the execution state of a single workflow step.
type StepStatus int

const (
	StepPending StepStatus = iota
	StepRunning
	StepDone
	StepFailed
)

// MemoryEntry is one persistent item shown in the memory browser.
type MemoryEntry struct {
	Key       string
	Value     string
	UpdatedAt time.Time
}

// HookEvent is one record in the hook log.
type HookEvent struct {
	At     time.Time
	Hook   string
	Result string
}

// SessionNode is one node in the session fork / replay tree.
type SessionNode struct {
	ID       string
	Label    string
	Depth    int
	Children []SessionNode
}

// ContextPanel is the toggleable side panel that switches between four views:
// workflow, memory, hooks, and session tree.
//
// It owns the render data for each view and is intentionally free of any
// BubbleTea dependency so it composes with the io.Writer stub today and slots
// cleanly into the future interactive TUI.
type ContextPanel struct {
	active       PanelView
	visible      bool
	workflow     []WorkflowStep
	memory       []MemoryEntry
	hooks        []HookEvent
	sessionNodes []SessionNode
	sessionLog   []contracts.SessionLogEntry
}

// NewContextPanel returns a panel with the workflow view active and hidden.
func NewContextPanel() *ContextPanel {
	return &ContextPanel{active: PanelWorkflow}
}

// Toggle shows or hides the panel (bound to mod+p in the TUI).
func (p *ContextPanel) Toggle() {
	p.visible = !p.visible
}

// Visible reports whether the panel is currently shown.
func (p *ContextPanel) Visible() bool {
	return p.visible
}

// SetView switches to view v; no-ops on out-of-range values.
func (p *ContextPanel) SetView(v PanelView) {
	if v >= 0 && v < panelViewCount {
		p.active = v
	}
}

// ActiveView returns the currently displayed view.
func (p *ContextPanel) ActiveView() PanelView {
	return p.active
}

// CycleView advances to the next view, wrapping around.
func (p *ContextPanel) CycleView() {
	p.active = (p.active + 1) % panelViewCount
}

// TabBar returns the one-line header showing all views with the active one
// marked, e.g. "workflow | [memory] | hooks | session".
func (p *ContextPanel) TabBar() string {
	names := make([]string, panelViewCount)
	for i := PanelView(0); i < panelViewCount; i++ {
		name := viewName(i)
		if i == p.active {
			name = "[" + name + "]"
		}
		names[i] = name
	}
	return strings.Join(names, " | ")
}

// SetWorkflow replaces the workflow step list.
func (p *ContextPanel) SetWorkflow(steps []WorkflowStep) { p.workflow = steps }

// SetMemory replaces the memory browser entries.
func (p *ContextPanel) SetMemory(entries []MemoryEntry) { p.memory = entries }

// AppendHookEvent adds one hook log entry.
func (p *ContextPanel) AppendHookEvent(ev HookEvent) { p.hooks = append(p.hooks, ev) }

// SetSessionNodes replaces the session fork/replay tree.
func (p *ContextPanel) SetSessionNodes(nodes []SessionNode) { p.sessionNodes = nodes }

// SetSessionLog replaces the engine session log entries shown in the session tree.
func (p *ContextPanel) SetSessionLog(entries []contracts.SessionLogEntry) {
	p.sessionLog = entries
}

// Render returns the full text content for the active view.
// Returns an empty string when the panel is hidden.
func (p *ContextPanel) Render() string {
	if !p.visible {
		return ""
	}
	header := p.TabBar() + "\n\n"
	switch p.active {
	case PanelWorkflow:
		return header + p.renderWorkflow()
	case PanelMemory:
		return header + p.renderMemory()
	case PanelHooks:
		return header + p.renderHooks()
	case PanelSessionTree:
		return header + p.renderSessionTree()
	}
	return header
}

func (p *ContextPanel) renderWorkflow() string {
	var sb strings.Builder
	sb.WriteString("── workflow steps ────────────\n\n")
	if len(p.workflow) == 0 {
		sb.WriteString(" no workflow steps\n")
		return sb.String()
	}
	for _, s := range p.workflow {
		icon := stepIcon(s.Status)
		sb.WriteString(fmt.Sprintf(" %s %d. %s\n", icon, s.Index, s.Name))
	}
	return sb.String()
}

func (p *ContextPanel) renderHooks() string {
	var sb strings.Builder
	sb.WriteString("── hook log ──────────────────\n\n")
	if len(p.hooks) == 0 {
		sb.WriteString(" no hook events\n")
		return sb.String()
	}
	for _, h := range p.hooks {
		sb.WriteString(fmt.Sprintf(" [%s] %s → %s\n", h.At.Format("15:04:05"), h.Hook, h.Result))
	}
	return sb.String()
}

func (p *ContextPanel) renderSessionTree() string {
	var sb strings.Builder
	sb.WriteString("── session tree ──────────────\n\n")
	for _, e := range p.sessionLog {
		sb.WriteString(fmt.Sprintf(" [%s] %s\n", e.At.Format("15:04:05"), e.Message))
	}
	for _, n := range p.sessionNodes {
		renderSessionNode(&sb, n)
	}
	if len(p.sessionLog) == 0 && len(p.sessionNodes) == 0 {
		sb.WriteString(" no session events\n")
	}
	return sb.String()
}

// renderMemory renders the memory browser view.
// TODO: implement memory browser rendering
func (p *ContextPanel) renderMemory() string {
	var sb strings.Builder
	sb.WriteString("── memory browser ────────────\n\n")
	if len(p.memory) == 0 {
		sb.WriteString(" no memory entries\n")
		return sb.String()
	}
	// TODO: implement how individual MemoryEntry values are formatted and displayed
	return sb.String()
}

func renderSessionNode(sb *strings.Builder, n SessionNode) {
	indent := strings.Repeat("  ", n.Depth)
	sb.WriteString(fmt.Sprintf("%s▸ %s  %s\n", indent, n.Label, n.ID))
	for _, child := range n.Children {
		renderSessionNode(sb, child)
	}
}

func stepIcon(s StepStatus) string {
	switch s {
	case StepDone:
		return "✓"
	case StepRunning:
		return "⟳"
	case StepFailed:
		return "✗"
	default:
		return "○"
	}
}

func viewName(v PanelView) string {
	switch v {
	case PanelWorkflow:
		return "workflow"
	case PanelMemory:
		return "memory"
	case PanelHooks:
		return "hooks"
	case PanelSessionTree:
		return "session"
	}
	return "unknown"
}
