package hooks

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// Manager fires configured hooks at agent lifecycle events.
// All methods are safe for concurrent use.
type Manager struct {
	mu        sync.Mutex
	hooks     []HookConfig
	sessionID string
	now       func() time.Time
	audit     []contracts.HookAuditEntry
}

// New creates a Manager from the supplied config.
func New(cfg Config, sessionID string) *Manager {
	return &Manager{
		hooks:     cfg.Hooks,
		sessionID: sessionID,
		now:       func() time.Time { return time.Now().UTC() },
	}
}

// Fire runs every hook registered for event. Returns the most restrictive
// decision across all matching hooks (block > inject > allow).
func (m *Manager) Fire(ctx context.Context, event EventType, input HookInput) (HookOutput, error) {
	input.Event = string(event)
	input.SessionID = m.sessionID
	if input.CWD == "" {
		input.CWD, _ = os.Getwd()
	}

	overall := HookOutput{Decision: DecisionAllow}

	for _, h := range m.hooks {
		if EventType(h.Event) != event {
			continue
		}
		if h.Command == "" {
			continue
		}
		if h.Matcher != "" && input.ToolName != "" {
			matched, err := regexp.MatchString(h.Matcher, input.ToolName)
			if err != nil || !matched {
				continue
			}
		}

		timeout := timeoutFor(h)
		hookCtx, cancel := context.WithTimeout(ctx, timeout)
		start := m.now()
		out, runErr := run(hookCtx, h.Command, input)
		elapsed := m.now().Sub(start)
		timedOut := hookCtx.Err() == context.DeadlineExceeded
		cancel()

		if out.Decision == "" {
			out.Decision = DecisionAllow
		}

		entry := contracts.HookAuditEntry{
			At:       start,
			Event:    string(event),
			Command:  h.Command,
			Decision: string(out.Decision),
			Reason:   out.Reason,
			Context:  out.Context,
			Elapsed:  elapsed,
			TimedOut: timedOut,
		}
		if runErr != nil {
			entry.Error = runErr.Error()
		}

		m.mu.Lock()
		m.audit = append(m.audit, entry)
		m.mu.Unlock()

		// block beats inject beats allow
		switch out.Decision {
		case DecisionBlock:
			overall = out
		case DecisionInject:
			if overall.Decision != DecisionBlock {
				overall = out
			}
		}
	}

	return overall, nil
}

// AuditTrail returns a copy of every hook execution record.
func (m *Manager) AuditTrail() []contracts.HookAuditEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]contracts.HookAuditEntry(nil), m.audit...)
}

// FormatDecision formats a hook decision for the session log.
func FormatDecision(event string, out HookOutput) string {
	if out.Reason != "" {
		return fmt.Sprintf("hook %s %s: %s", event, out.Decision, out.Reason)
	}
	return fmt.Sprintf("hook %s %s", event, out.Decision)
}
