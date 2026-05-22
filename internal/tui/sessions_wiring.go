package tui

import (
	"fmt"

	"github.com/jabreeflor/conduit/internal/sessions"
)

// AttachSessions injects a sessions.Dispatcher into the model so /sessions
// slash commands and the ctrl+s browser have something to talk to. Surfaces
// call this once at boot — the dispatcher is reused across browser opens
// so list/load are cheap.
func (m Model) AttachSessions(dispatcher *sessions.Dispatcher) Model {
	m.sessions = dispatcher
	return m
}

// openSessionsBrowser builds a fresh tree from the dispatcher's store and
// hands it to a new SessionsBrowser instance. We rebuild on every open so
// new sessions show up without restarting the TUI.
func (m Model) openSessionsBrowser() Model {
	if m.sessions == nil || m.sessions.Store == nil {
		m.messages = append(m.messages, message{
			role: roleAgent,
			text: "Session browser unavailable — no session store attached.",
		})
		return m.refreshContent()
	}
	tree, err := sessions.BuildTree(m.sessions.Store)
	if err != nil {
		m.messages = append(m.messages, message{
			role: roleAgent,
			text: "Could not load session tree: " + err.Error(),
		})
		return m.refreshContent()
	}
	browser := NewSessionsBrowser(tree)
	browser.SetSize(m.width, m.height)
	m.sessionsBrowser = &browser
	return m
}

// handleSessionsSlash parses the /sessions command line and renders the
// dispatcher's response into the conversation log. A bare "/sessions" or
// "/sessions list" with no arguments opens the browser instead so users
// can navigate visually.
func (m Model) handleSessionsSlash(line string) Model {
	if m.sessions == nil {
		m.messages = append(m.messages, message{
			role: roleAgent,
			text: "Sessions are not configured for this surface.",
		})
		return m.refreshContent()
	}
	res, err := m.sessions.DispatchLine(line)
	if err != nil {
		m.messages = append(m.messages, message{
			role: roleAgent,
			text: fmt.Sprintf("/sessions error: %v", err),
		})
		return m.refreshContent()
	}
	if res.Output != "" {
		m.messages = append(m.messages, message{role: roleAgent, text: res.Output})
	}
	if res.LoadSessionID != "" {
		m.activeSessionID = res.LoadSessionID
	}
	return m.refreshContent()
}

// handleSessionsBrowserClose runs when the browser hands focus back. It
// inspects the chosen action and dispatches it through the same pipeline
// the slash commands use, so output stays consistent.
func (m Model) handleSessionsBrowserClose(sel SessionsBrowserSelection) Model {
	m.sessionsBrowser = nil
	if m.sessions == nil {
		return m.refreshContent()
	}
	switch sel.Action {
	case ActionLoad:
		if sel.SessionID == "" {
			return m.refreshContent()
		}
		res, err := m.sessions.Dispatch([]string{"load", sel.SessionID})
		return m.appendDispatchResult(res, err)
	case ActionFork:
		if sel.TurnID == "" {
			return m.refreshContent()
		}
		res, err := m.sessions.Dispatch([]string{"fork", sel.TurnID})
		return m.appendDispatchResult(res, err)
	case ActionReplay:
		if sel.TurnID == "" {
			return m.refreshContent()
		}
		// No --model override here; the host TUI doesn't yet have a model
		// picker UI for the browser. Users can run /sessions replay with a
		// flag from the prompt instead.
		res, err := m.sessions.Dispatch([]string{"replay", sel.TurnID})
		return m.appendDispatchResult(res, err)
	}
	return m.refreshContent()
}

func (m Model) appendDispatchResult(res sessions.Result, err error) Model {
	if err != nil {
		m.messages = append(m.messages, message{
			role: roleAgent,
			text: fmt.Sprintf("/sessions error: %v", err),
		})
		return m.refreshContent()
	}
	if res.Output != "" {
		m.messages = append(m.messages, message{role: roleAgent, text: res.Output})
	}
	if res.LoadSessionID != "" {
		m.activeSessionID = res.LoadSessionID
	}
	return m.refreshContent()
}
