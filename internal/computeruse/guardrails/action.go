// Package guardrails enforces safety rules on macOS computer-use actions
// before they reach the OS. PRD §6.8 mandates a checkpoint before any
// destructive action and explicit confirmation before sending email,
// posting publicly, or executing a financial transaction.
//
// The MCP step dispatcher (issue #37) MUST call (*Engine).Evaluate on every
// proposed action before executing it. The capability adapters (issue #40)
// supply the per-action metadata — bundle ID, URL, action verb, target
// element — that the classifier consumes.
package guardrails

import "time"

// Category labels the kind of risk an action carries. Multiple categories
// can apply to a single action (e.g. clicking a "Confirm purchase" button in
// a browser is both Communication-shaped and Financial — Financial wins).
type Category string

const (
	// CategoryUnknown is the absence of any matched rule.
	CategoryUnknown Category = ""
	// CategoryCommunication covers send-email, post-publicly, send-DM.
	CategoryCommunication Category = "communication"
	// CategoryFinancial covers banking, payments, checkout, confirm-purchase.
	CategoryFinancial Category = "financial"
	// CategoryFilesystem covers rm/delete/empty-trash/format/uninstall.
	CategoryFilesystem Category = "filesystem"
	// CategorySystem covers shutdown/restart/sudo/login-window logout.
	CategorySystem Category = "system"
)

// Verdict is the policy decision for one Action.
type Verdict string

const (
	// VerdictAllow means the dispatcher may execute the action without prompting.
	VerdictAllow Verdict = "allow"
	// VerdictRequireConfirmation means the dispatcher must obtain user consent
	// (via Engine.RequestConfirmation) before executing.
	VerdictRequireConfirmation Verdict = "require_confirmation"
	// VerdictDeny means the dispatcher must NOT execute the action. Used for
	// hard-blocked targets (financial by default on ambiguity) and for
	// confirmation timeouts.
	VerdictDeny Verdict = "deny"
)

// Action is the structured description of a single computer-use step that
// the capability adapter (#40) hands to the dispatcher (#37).
//
// Fields are intentionally permissive — a v1 classifier matches on whichever
// signals are present. None are required; an empty Action evaluates to
// CategoryUnknown / VerdictAllow.
type Action struct {
	// Verb is the high-level intent: "click", "type", "send", "delete",
	// "shutdown", "open", "screenshot", etc.
	Verb string
	// BundleID is the macOS application bundle identifier, e.g.
	// "com.apple.mail" or "com.tinyspeck.slackmacgap".
	BundleID string
	// AppName is a human-readable fallback when BundleID is unavailable.
	AppName string
	// URL is set for browser actions; the classifier matches the host.
	URL string
	// Target is the accessibility-tree label of the element being acted on,
	// e.g. "Send", "Confirm Purchase", "Empty Trash".
	Target string
	// Path is the filesystem path for file actions.
	Path string
	// Text is the typed/sent payload (used to spot keywords like "rm -rf").
	Text string
	// Description is a freeform sentence the model emitted for the step,
	// used as a final keyword fallback.
	Description string
}

// Decision is the result of evaluating one Action.
type Decision struct {
	Verdict    Verdict
	Categories []Category
	// MatchedRule names the policy entry that produced the verdict; empty
	// when nothing matched (default-allow path).
	MatchedRule string
	Reason      string
	// Confirmed reports whether a require_confirmation decision was actually
	// approved by the user. Audit consumers use this to distinguish a
	// "would-have-prompted" decision from an "approved" one.
	Confirmed bool
}

// AuditEntry is the JSONL record written to the session log for every
// guardrail decision. The shape matches the per-session JSONL convention
// established by the cost tracker (PR #15) — flat fields, RFC3339 timestamp.
type AuditEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	SessionID   string    `json:"session_id"`
	Verdict     Verdict   `json:"verdict"`
	Confirmed   bool      `json:"confirmed,omitempty"`
	Categories  []string  `json:"categories,omitempty"`
	MatchedRule string    `json:"matched_rule,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	Verb        string    `json:"verb,omitempty"`
	BundleID    string    `json:"bundle_id,omitempty"`
	AppName     string    `json:"app_name,omitempty"`
	URLHost     string    `json:"url_host,omitempty"`
	Target      string    `json:"target,omitempty"`
	Path        string    `json:"path,omitempty"`
	// Note: Text is intentionally NOT logged — it can contain secrets.
	Description string `json:"description,omitempty"`
}
