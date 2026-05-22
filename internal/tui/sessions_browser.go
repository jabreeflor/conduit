package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jabreeflor/conduit/internal/sessions"
)

// SessionsBrowser is a navigable visualisation of the session JSONL forest.
// It is intentionally lightweight: the visible tree is flattened into a
// row list ahead of time so navigation is O(1) and the renderer is trivial.
//
// Keybindings:
//
//	up / k        : move selection up
//	down / j      : move selection down
//	enter         : Load currently selected session (or session a turn lives in)
//	f             : Fork from selected turn
//	r             : Replay from selected turn
//	q / esc       : close the browser without doing anything
//
// The browser does not persist anywhere on its own; it returns one of three
// "actions" via Selected() so the embedding TUI / test driver can wire the
// outcome (load a session, fork, replay) into its own state.
type SessionsBrowser struct {
	tree    *sessions.Tree
	rows    []sessionsRow
	cursor  int
	width   int
	height  int
	action  SessionsBrowserAction
	closed  bool
	help    string
	errText string
}

// SessionsBrowserAction is the choice made by the user when they close
// the browser. ActionNone means the browser was dismissed without input.
type SessionsBrowserAction int

const (
	// ActionNone signals a clean dismissal — nothing should happen.
	ActionNone SessionsBrowserAction = iota
	// ActionLoad asks the host to load the chosen session id.
	ActionLoad
	// ActionFork asks the host to fork from the chosen turn id.
	ActionFork
	// ActionReplay asks the host to replay from the chosen turn id.
	ActionReplay
)

// SessionsBrowserSelection is the user's chosen action plus the ids the
// host needs to act on it.
type SessionsBrowserSelection struct {
	Action    SessionsBrowserAction
	SessionID string
	TurnID    string
}

type sessionsRow struct {
	depth     int
	kind      sessions.NodeKind
	sessionID string
	turnID    string
	label     string
	preview   string
}

// NewSessionsBrowser constructs the browser model from a fully built tree.
func NewSessionsBrowser(tree *sessions.Tree) SessionsBrowser {
	b := SessionsBrowser{tree: tree, help: "↑/↓ navigate · enter load · f fork · r replay · q quit"}
	b.rebuildRows()
	return b
}

// SetSize updates the panel dimensions; called on tea.WindowSizeMsg.
func (b *SessionsBrowser) SetSize(w, h int) {
	b.width = w
	b.height = h
}

// Init satisfies tea.Model.
func (b SessionsBrowser) Init() tea.Cmd { return nil }

var sessionsBrowserKeys = struct {
	Up, Down, Load, Fork, Replay, Quit key.Binding
}{
	Up:     key.NewBinding(key.WithKeys("up", "k")),
	Down:   key.NewBinding(key.WithKeys("down", "j")),
	Load:   key.NewBinding(key.WithKeys("enter")),
	Fork:   key.NewBinding(key.WithKeys("f")),
	Replay: key.NewBinding(key.WithKeys("r")),
	Quit:   key.NewBinding(key.WithKeys("q", "esc")),
}

// Update handles the small set of key events this browser cares about.
// It never produces tea.Cmds — outcomes are read by the host via
// Selected() / IsClosed() once the user dismisses the browser.
func (b SessionsBrowser) Update(msg tea.Msg) (SessionsBrowser, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		b.width = msg.Width
		b.height = msg.Height
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, sessionsBrowserKeys.Up):
			if b.cursor > 0 {
				b.cursor--
			}
		case key.Matches(msg, sessionsBrowserKeys.Down):
			if b.cursor+1 < len(b.rows) {
				b.cursor++
			}
		case key.Matches(msg, sessionsBrowserKeys.Load):
			b.completeWith(ActionLoad)
		case key.Matches(msg, sessionsBrowserKeys.Fork):
			b.completeWith(ActionFork)
		case key.Matches(msg, sessionsBrowserKeys.Replay):
			b.completeWith(ActionReplay)
		case key.Matches(msg, sessionsBrowserKeys.Quit):
			b.action = ActionNone
			b.closed = true
		}
	}
	return b, nil
}

// IsClosed reports whether the user has finished interacting with the browser.
func (b SessionsBrowser) IsClosed() bool { return b.closed }

// Selected returns the user's chosen action and ids. Only meaningful once
// IsClosed() returns true.
func (b SessionsBrowser) Selected() SessionsBrowserSelection {
	row, ok := b.currentRow()
	sel := SessionsBrowserSelection{Action: b.action}
	if !ok {
		return sel
	}
	sel.SessionID = row.sessionID
	sel.TurnID = row.turnID
	return sel
}

// View renders the browser as a self-contained string.
func (b SessionsBrowser) View() string {
	var sb strings.Builder
	sb.WriteString(styleAgent.Render("Session Tree Browser") + "\n")
	sb.WriteString(styleDim.Render(b.help) + "\n\n")
	if b.errText != "" {
		sb.WriteString(styleToolFail.Render(b.errText) + "\n\n")
	}
	if len(b.rows) == 0 {
		sb.WriteString(styleDim.Render(" no sessions yet — start a conversation to populate the tree"))
		return sb.String()
	}
	for i, row := range b.rows {
		marker := "  "
		if i == b.cursor {
			marker = "▶ "
		}
		indent := strings.Repeat("  ", row.depth)
		line := fmt.Sprintf("%s%s%s", marker, indent, row.label)
		if row.preview != "" {
			line += "  " + styleDim.Render(row.preview)
		}
		if i == b.cursor {
			line = styleUser.Render(line)
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

func (b *SessionsBrowser) completeWith(a SessionsBrowserAction) {
	row, ok := b.currentRow()
	if !ok {
		b.errText = "no row selected"
		return
	}
	switch a {
	case ActionFork, ActionReplay:
		if row.turnID == "" {
			b.errText = "fork/replay requires selecting a turn (not a session header)"
			return
		}
	}
	b.action = a
	b.closed = true
}

func (b *SessionsBrowser) currentRow() (sessionsRow, bool) {
	if b.cursor < 0 || b.cursor >= len(b.rows) {
		return sessionsRow{}, false
	}
	return b.rows[b.cursor], true
}

func (b *SessionsBrowser) rebuildRows() {
	b.rows = b.rows[:0]
	if b.tree == nil {
		return
	}
	b.tree.Walk(func(n *sessions.Node, depth int) {
		row := sessionsRow{depth: depth, kind: n.Kind}
		switch n.Kind {
		case sessions.NodeKindSession:
			row.sessionID = n.Session.ID
			label := fmt.Sprintf("● %s", n.Session.ID)
			if n.Session.ForkParentID != "" {
				label += " (forked)"
			}
			row.label = label
			if n.Session.Title != "" {
				row.preview = n.Session.Title
			}
		case sessions.NodeKindTurn:
			row.sessionID = n.Turn.SessionID
			row.turnID = n.Turn.ID
			role := n.Turn.Role
			if role == "" {
				role = "?"
			}
			row.label = fmt.Sprintf("· [%s] %s", role, shortID(n.Turn.ID))
			row.preview = truncatePreview(n.Turn.Content, 60)
		}
		b.rows = append(b.rows, row)
	})
}

func truncatePreview(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:6] + "…" + id[len(id)-4:]
}
