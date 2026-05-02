package guardrails

import (
	"context"
	"fmt"
	"strings"
)

// Policy maps each Category to a default Verdict. Callers can override
// individual entries via Engine options. Defaults follow the PRD §6.8 rule
// of thumb: financial actions are deny-by-default on ambiguity, every
// other destructive category requires explicit confirmation.
type Policy struct {
	// Default is the verdict for actions that match no category.
	Default Verdict
	// Per-category verdicts. Missing categories fall through to Default.
	Categories map[Category]Verdict
	// FinancialDefaultDeny preserves the safety floor: when true, an action
	// with CategoryFinancial that would otherwise be RequireConfirmation
	// upgrades to Deny. The MCP dispatcher must surface a clear error to
	// the user — better to interrupt than to send a wrong wire.
	FinancialDefaultDeny bool
}

// DefaultPolicy returns the PRD §6.8 default policy.
func DefaultPolicy() Policy {
	return Policy{
		Default: VerdictAllow,
		Categories: map[Category]Verdict{
			CategoryCommunication: VerdictRequireConfirmation,
			CategoryFinancial:     VerdictRequireConfirmation,
			CategoryFilesystem:    VerdictRequireConfirmation,
			CategorySystem:        VerdictRequireConfirmation,
		},
		FinancialDefaultDeny: true,
	}
}

// verdictFor returns the verdict for the highest-priority category in cats.
// When no category matches, it returns p.Default. The Financial-default-deny
// upgrade is applied last so it always wins.
func (p Policy) verdictFor(cats []Category) (Verdict, Category) {
	if len(cats) == 0 {
		return p.Default, CategoryUnknown
	}
	dominant := cats[0]
	v, ok := p.Categories[dominant]
	if !ok {
		v = p.Default
	}
	if dominant == CategoryFinancial && p.FinancialDefaultDeny &&
		v != VerdictAllow {
		v = VerdictDeny
	}
	return v, dominant
}

// reasonFor renders a short, user-readable reason string.
func reasonFor(verdict Verdict, dominant Category, a Action) string {
	parts := []string{string(verdict)}
	if dominant != CategoryUnknown {
		parts = append(parts, "category="+string(dominant))
	}
	if a.Verb != "" {
		parts = append(parts, "verb="+a.Verb)
	}
	if a.BundleID != "" {
		parts = append(parts, "bundle="+a.BundleID)
	} else if a.AppName != "" {
		parts = append(parts, "app="+a.AppName)
	}
	if a.URL != "" {
		if h := hostFromURL(a.URL); h != "" {
			parts = append(parts, "host="+h)
		}
	}
	if a.Target != "" {
		parts = append(parts, fmt.Sprintf("target=%q", a.Target))
	}
	return strings.Join(parts, " ")
}

// Confirmer is the host-side hook that the TUI / CLI / GUI implements.
// It MUST block until the user makes a decision OR the supplied context is
// cancelled. On context cancellation it MUST return (false, ctx.Err()) so
// the engine can fail closed.
//
// Implementations are expected to render `reason` verbatim so the user
// sees exactly what is being asked.
type Confirmer interface {
	Confirm(ctx context.Context, action Action, reason string) (bool, error)
}

// ConfirmerFunc adapts a plain function to the Confirmer interface.
type ConfirmerFunc func(ctx context.Context, action Action, reason string) (bool, error)

// Confirm implements Confirmer.
func (f ConfirmerFunc) Confirm(ctx context.Context, action Action, reason string) (bool, error) {
	return f(ctx, action, reason)
}

// denyConfirmer is the safe-by-default confirmer used when the host did not
// supply one. It always denies, ensuring the engine fails closed.
type denyConfirmer struct{}

func (denyConfirmer) Confirm(_ context.Context, _ Action, _ string) (bool, error) {
	return false, nil
}
