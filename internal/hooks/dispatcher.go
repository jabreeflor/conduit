package hooks

import (
	"context"
	"regexp"
	"time"
)

type hookEntry struct {
	def     HookDef
	re      *regexp.Regexp // nil if Matcher is empty
	timeout time.Duration
}

// Dispatcher fires all matching hooks for a given event.
// A nil Dispatcher is a no-op that returns allow.
type Dispatcher struct {
	entries []hookEntry
}

// New compiles regex matchers and returns a Dispatcher ready to fire hooks.
func New(cfg Config) (*Dispatcher, error) {
	entries := make([]hookEntry, 0, len(cfg.Hooks))
	for _, def := range cfg.Hooks {
		entry := hookEntry{def: def, timeout: defaultTimeout}
		if def.Timeout > 0 {
			entry.timeout = time.Duration(def.Timeout) * time.Second
		}
		if def.Matcher != "" {
			re, err := regexp.Compile(def.Matcher)
			if err != nil {
				return nil, err
			}
			entry.re = re
		}
		entries = append(entries, entry)
	}
	return &Dispatcher{entries: entries}, nil
}

// Dispatch runs all hooks registered for input.Event in registration order.
// The matcher is applied to ToolName for pre/post_tool_call events, and to
// SessionID for all other events. The first block or inject decision is returned
// immediately; otherwise allow is returned after all hooks complete.
func (d *Dispatcher) Dispatch(ctx context.Context, input Input) Output {
	if d == nil {
		return Output{Decision: DecisionAllow}
	}
	for _, entry := range d.entries {
		if entry.def.Event != input.Event {
			continue
		}
		if !entry.matches(input) {
			continue
		}
		out := run(ctx, entry.def.Command, entry.timeout, input)
		if out.Decision == DecisionBlock || out.Decision == DecisionInject {
			return out
		}
	}
	return Output{Decision: DecisionAllow}
}

func (e *hookEntry) matches(input Input) bool {
	if e.re == nil {
		return true
	}
	subject := input.SessionID
	if input.Event == EventPreToolCall || input.Event == EventPostToolCall {
		subject = input.ToolName
	}
	return e.re.MatchString(subject)
}
