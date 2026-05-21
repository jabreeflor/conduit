// Package approval implements Conduit's reviewer agent — the safety layer
// that evaluates side-effecting actions before they are executed.
// PRD §6.25.8 (approval network).
//
// The reviewer is conservative by default. The defining principle: "if in
// doubt, escalate to the user." A reviewer that errors, hits an unexpected
// payload, or sees ambiguous categorisation must NOT silently allow — it
// returns DecisionConfirm so the user is asked. This file deliberately
// avoids any "auto-approve on failure" path.
package approval

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Category labels the kind of side effect a candidate action represents.
// These align with contracts.PermissionCategory values but are restated here
// so the reviewer can ship without depending on the broader permission
// graph; surfaces are responsible for translating one to the other.
type Category string

const (
	CategoryFilesystemWrite Category = "filesystem_write"
	CategoryShell           Category = "shell"
	CategoryNetwork         Category = "network"
	CategoryDestructive     Category = "destructive"
	CategoryCredentials     Category = "credentials"
	CategoryExternalComms   Category = "external_comms"
	CategoryComputerUse     Category = "computer_use"
)

// Decision is the reviewer's verdict on a candidate action.
type Decision string

const (
	// DecisionAllow means the action may proceed without prompting the
	// user. Reserved for actions whose category and content the policy
	// explicitly auto-approves.
	DecisionAllow Decision = "allow"
	// DecisionConfirm means the user must explicitly approve in the
	// surface. This is the conservative default for anything ambiguous.
	DecisionConfirm Decision = "confirm"
	// DecisionDeny means the action is blocked outright. Used when the
	// reviewer has high-confidence signal that the action is malicious or
	// disallowed by policy.
	DecisionDeny Decision = "deny"
)

// Action describes a single side-effecting operation the agent wants to run.
// Tool is the underlying tool/runner name; Category is the reviewer's
// classification (callers may pre-compute or leave empty to let the
// reviewer infer it). Payload carries arguments verbatim so detectors can
// inspect content (e.g. shell command body, target URL).
type Action struct {
	Tool     string
	Category Category
	Payload  map[string]any
	// Reason is the agent-supplied rationale, surfaced to the user when
	// asking for confirmation.
	Reason string
	// SessionID is used for audit logging only; reviewers must not consult
	// it to influence the decision (avoids "this user is trusted" logic
	// creeping into the safety layer).
	SessionID string
}

// Verdict is the reviewer's structured response.
type Verdict struct {
	Decision   Decision
	Category   Category
	Detections []Detection
	// Prompt is the typed approval prompt the surface should display when
	// Decision is DecisionConfirm. Empty when the verdict is allow/deny.
	Prompt ApprovalPrompt
}

// Detection records one trigger fired by the reviewer's heuristics. It is
// retained on the verdict so the surface can show "we asked because: …".
type Detection struct {
	Rule   string
	Detail string
	Severe bool
}

// ApprovalPrompt is the typed prompt presented to the user. Different
// categories carry different fields so surfaces can render them
// appropriately.
type ApprovalPrompt struct {
	Title       string
	Body        string
	Category    Category
	RiskLevel   string // "low" | "medium" | "high"
	Highlights  []string
	ActionTool  string
	ActionInput string
}

// AutomaticReviewPolicy controls when the reviewer auto-approves vs. always
// asks. The defaults are conservative; admins or users can broaden them via
// config (see PolicyFromMap).
type AutomaticReviewPolicy struct {
	// AutoAllow lists categories the reviewer may auto-approve when no
	// detections fire. Categories missing from this list always require
	// confirmation.
	AutoAllow []Category
	// AutoDeny lists categories that are always denied outright (e.g.
	// "credentials" in a hardened deployment).
	AutoDeny []Category
	// AllowedHosts is an optional allowlist for network actions. When
	// non-empty, network actions to other hosts always require
	// confirmation, even if Network is in AutoAllow.
	AllowedHosts []string
	// MaxShellLength caps the auto-approvable shell command length. Longer
	// commands escalate to confirmation regardless of detections.
	MaxShellLength int
}

// DefaultPolicy is the recommended starting point: filesystem-write to the
// workspace and read-only network are auto-approved; everything else
// requires confirmation.
func DefaultPolicy() AutomaticReviewPolicy {
	return AutomaticReviewPolicy{
		AutoAllow:      []Category{CategoryFilesystemWrite},
		MaxShellLength: 200,
	}
}

// Reviewer evaluates Actions against a configurable policy and a fixed set
// of detection rules. It is safe for concurrent use.
type Reviewer struct {
	mu        sync.RWMutex
	policy    AutomaticReviewPolicy
	detectors []Detector
	auditFn   func(Action, Verdict)
}

// Detector is one rule applied to every Action. Implementations should be
// cheap (no I/O) so the reviewer adds negligible per-action latency.
type Detector interface {
	Inspect(Action) []Detection
}

// NewReviewer constructs a reviewer with the supplied policy and detector
// list. Pass DefaultDetectors() to install the built-in safety checks.
func NewReviewer(policy AutomaticReviewPolicy, detectors []Detector) *Reviewer {
	return &Reviewer{policy: policy, detectors: detectors}
}

// SetAuditFn registers an optional audit callback invoked AFTER every
// Review call. Failures inside the callback do not influence the verdict.
func (r *Reviewer) SetAuditFn(fn func(Action, Verdict)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.auditFn = fn
}

// Review is the main entry point. The supplied context is used for
// deadlines only — no I/O is performed. Returns a Verdict; the caller MUST
// honour Decision (the function never executes the action itself).
func (r *Reviewer) Review(ctx context.Context, a Action) (Verdict, error) {
	if err := ctx.Err(); err != nil {
		return Verdict{Decision: DecisionConfirm, Category: a.Category}, err
	}
	if a.Tool == "" {
		// An action with no tool name is malformed; fail closed.
		return Verdict{
			Decision: DecisionConfirm, Category: a.Category,
			Prompt: ApprovalPrompt{
				Title: "Unidentified action", Body: "Action carried no tool name.",
				Category: a.Category, RiskLevel: "high",
			},
		}, errors.New("approval: action.Tool is empty")
	}

	r.mu.RLock()
	policy := r.policy
	detectors := append([]Detector(nil), r.detectors...)
	auditFn := r.auditFn
	r.mu.RUnlock()

	if a.Category == "" {
		a.Category = inferCategory(a.Tool)
	}

	var detections []Detection
	for _, d := range detectors {
		detections = append(detections, d.Inspect(a)...)
	}

	verdict := Verdict{Category: a.Category, Detections: detections}

	// Auto-deny categories take precedence over everything.
	for _, c := range policy.AutoDeny {
		if c == a.Category {
			verdict.Decision = DecisionDeny
			verdict.Prompt = ApprovalPrompt{
				Title:    "Action denied by policy",
				Body:     fmt.Sprintf("Category %q is on the auto-deny list.", a.Category),
				Category: a.Category, RiskLevel: "high",
			}
			if auditFn != nil {
				auditFn(a, verdict)
			}
			return verdict, nil
		}
	}

	// Severe detections always require confirmation.
	severe := false
	for _, d := range detections {
		if d.Severe {
			severe = true
			break
		}
	}

	autoAllowed := false
	for _, c := range policy.AutoAllow {
		if c == a.Category {
			autoAllowed = true
			break
		}
	}

	// Per-category extra checks beyond AutoAllow membership.
	if autoAllowed {
		switch a.Category {
		case CategoryShell:
			cmd, _ := a.Payload["command"].(string)
			if policy.MaxShellLength > 0 && len(cmd) > policy.MaxShellLength {
				autoAllowed = false
				detections = append(detections, Detection{
					Rule:   "shell.too_long",
					Detail: fmt.Sprintf("command length %d exceeds auto-approve cap %d", len(cmd), policy.MaxShellLength),
				})
			}
		case CategoryNetwork:
			if !networkHostAllowed(a.Payload, policy.AllowedHosts) {
				autoAllowed = false
				detections = append(detections, Detection{
					Rule:   "network.host_not_allowlisted",
					Detail: "destination host is not in the allowed list",
				})
			}
		}
	}

	if autoAllowed && !severe {
		verdict.Decision = DecisionAllow
		verdict.Detections = detections
		if auditFn != nil {
			auditFn(a, verdict)
		}
		return verdict, nil
	}

	verdict.Decision = DecisionConfirm
	verdict.Detections = detections
	verdict.Prompt = buildPrompt(a, detections, severe)
	if auditFn != nil {
		auditFn(a, verdict)
	}
	return verdict, nil
}

func buildPrompt(a Action, detections []Detection, severe bool) ApprovalPrompt {
	risk := "medium"
	if severe {
		risk = "high"
	}
	highlights := make([]string, 0, len(detections))
	for _, d := range detections {
		highlights = append(highlights, fmt.Sprintf("[%s] %s", d.Rule, d.Detail))
	}
	title := titleFor(a.Category)
	body := a.Reason
	if body == "" {
		body = fmt.Sprintf("The agent is requesting permission to run %q.", a.Tool)
	}
	return ApprovalPrompt{
		Title: title, Body: body, Category: a.Category, RiskLevel: risk,
		Highlights:  highlights,
		ActionTool:  a.Tool,
		ActionInput: summarisePayload(a.Payload),
	}
}

func titleFor(c Category) string {
	switch c {
	case CategoryShell:
		return "Confirm shell command"
	case CategoryNetwork:
		return "Confirm network request"
	case CategoryDestructive:
		return "Confirm destructive action"
	case CategoryCredentials:
		return "Confirm credential access"
	case CategoryExternalComms:
		return "Confirm external communication"
	case CategoryComputerUse:
		return "Confirm computer-use action"
	case CategoryFilesystemWrite:
		return "Confirm filesystem write"
	default:
		return "Confirm action"
	}
}

func summarisePayload(p map[string]any) string {
	if len(p) == 0 {
		return ""
	}
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	// Stable but minimal: just join keys with previews when short.
	var parts []string
	for _, k := range keys {
		v := fmt.Sprintf("%v", p[k])
		if len(v) > 80 {
			v = v[:77] + "..."
		}
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, " ")
}

func inferCategory(tool string) Category {
	switch strings.ToLower(tool) {
	case "bash", "sh", "shell", "exec":
		return CategoryShell
	case "fetch", "http", "curl", "web_search":
		return CategoryNetwork
	case "rm", "rmdir", "delete_file", "drop_table":
		return CategoryDestructive
	case "write_file", "edit_file", "notebook_edit":
		return CategoryFilesystemWrite
	case "send_email", "send_slack", "post_message":
		return CategoryExternalComms
	}
	return ""
}

func networkHostAllowed(payload map[string]any, allowed []string) bool {
	if len(allowed) == 0 {
		return false
	}
	host := extractHost(payload)
	if host == "" {
		return false
	}
	for _, h := range allowed {
		if strings.EqualFold(host, h) {
			return true
		}
	}
	return false
}

func extractHost(payload map[string]any) string {
	for _, key := range []string{"host", "hostname"} {
		if v, ok := payload[key].(string); ok && v != "" {
			return v
		}
	}
	if u, ok := payload["url"].(string); ok && u != "" {
		if parsed, err := url.Parse(u); err == nil {
			h := parsed.Hostname()
			// Strip ports if any sneaked in via host header.
			if host, _, err := net.SplitHostPort(h); err == nil {
				return host
			}
			return h
		}
	}
	return ""
}

// PolicyFromMap parses a config-friendly map into a policy. Unknown keys
// are ignored so old code can read newer config without breaking.
func PolicyFromMap(in map[string]any) AutomaticReviewPolicy {
	p := DefaultPolicy()
	if v, ok := in["auto_allow"].([]any); ok {
		p.AutoAllow = nil
		for _, item := range v {
			if s, ok := item.(string); ok {
				p.AutoAllow = append(p.AutoAllow, Category(s))
			}
		}
	}
	if v, ok := in["auto_deny"].([]any); ok {
		for _, item := range v {
			if s, ok := item.(string); ok {
				p.AutoDeny = append(p.AutoDeny, Category(s))
			}
		}
	}
	if v, ok := in["allowed_hosts"].([]any); ok {
		for _, item := range v {
			if s, ok := item.(string); ok {
				p.AllowedHosts = append(p.AllowedHosts, s)
			}
		}
	}
	if v, ok := in["max_shell_length"].(int); ok && v > 0 {
		p.MaxShellLength = v
	}
	return p
}

// NewAuditWriter builds an audit callback that calls fn with a single line
// per verdict, suitable for passing to Reviewer.SetAuditFn. Time format is
// RFC3339 to keep grep-ability with the managed-config audit log.
func NewAuditWriter(fn func(line string) error) func(Action, Verdict) {
	return func(a Action, v Verdict) {
		_ = fn(fmt.Sprintf("%s\t%s\t%s\t%s\t%s", time.Now().UTC().Format(time.RFC3339), v.Decision, v.Category, a.Tool, a.SessionID))
	}
}

// ---------------------------------------------------------------------------
// Built-in detectors
// ---------------------------------------------------------------------------

// DefaultDetectors returns the built-in detector set used by Conduit. The
// list is intentionally small and high-precision: we want false-positive
// confirmations, not false negatives.
func DefaultDetectors() []Detector {
	return []Detector{
		ShellDetector{},
		ExfilDetector{},
		CredentialProbeDetector{},
		PersistenceDetector{},
	}
}

// ShellDetector flags shell payloads that look destructive.
type ShellDetector struct{}

var (
	rmPattern   = regexp.MustCompile(`(?i)\brm\s+(-[a-z]*r[a-z]*\s+)?(/\S*|~\S*|\.\s|\.\*)`)
	sudoPattern = regexp.MustCompile(`(?i)\bsudo\b`)
	pipeShPipe  = regexp.MustCompile(`(?i)curl\s+[^|]+\|\s*(sh|bash|zsh)`)
)

// Inspect implements Detector.
func (ShellDetector) Inspect(a Action) []Detection {
	if a.Category != "" && a.Category != CategoryShell {
		return nil
	}
	cmd, _ := a.Payload["command"].(string)
	if cmd == "" {
		return nil
	}
	var out []Detection
	if rmPattern.MatchString(cmd) {
		out = append(out, Detection{Rule: "shell.rm_recursive", Detail: "recursive or root-targeted rm", Severe: true})
	}
	if sudoPattern.MatchString(cmd) {
		out = append(out, Detection{Rule: "shell.sudo", Detail: "sudo invocation", Severe: true})
	}
	if pipeShPipe.MatchString(cmd) {
		out = append(out, Detection{Rule: "shell.curl_pipe_sh", Detail: "curl piped to a shell — common malware pattern", Severe: true})
	}
	return out
}

// ExfilDetector flags payloads containing what looks like API keys or large
// secret-like blobs being sent off-machine.
type ExfilDetector struct{}

var (
	apiKeyPattern = regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password)\s*[:=]\s*['"]?[A-Za-z0-9_\-]{16,}`)
)

// Inspect implements Detector.
func (ExfilDetector) Inspect(a Action) []Detection {
	if a.Category == CategoryFilesystemWrite {
		return nil
	}
	for _, key := range []string{"body", "data", "payload", "content"} {
		v, _ := a.Payload[key].(string)
		if v != "" && apiKeyPattern.MatchString(v) {
			return []Detection{{
				Rule: "exfil.secret_in_payload", Severe: true,
				Detail: "payload contains a value that looks like an API key or password",
			}}
		}
	}
	return nil
}

// CredentialProbeDetector flags reads from common secret stores or env-var
// sweeps.
type CredentialProbeDetector struct{}

var credentialPaths = []string{
	"~/.aws/credentials", "/.aws/credentials",
	"~/.ssh/id_rsa", "/.ssh/id_rsa",
	"~/.netrc", "/.netrc",
	".env",
}

// Inspect implements Detector.
func (CredentialProbeDetector) Inspect(a Action) []Detection {
	for _, key := range []string{"path", "command", "url"} {
		v, _ := a.Payload[key].(string)
		if v == "" {
			continue
		}
		low := strings.ToLower(v)
		for _, p := range credentialPaths {
			if strings.Contains(low, p) {
				return []Detection{{
					Rule: "credentials.probe", Severe: true,
					Detail: "action references a known credential store path",
				}}
			}
		}
	}
	return nil
}

// PersistenceDetector flags writes to autostart locations and shell rc
// files — actions that would weaken the host beyond the current session.
type PersistenceDetector struct{}

var persistencePaths = []string{
	"/launchagents/", "/launchdaemons/",
	".bashrc", ".zshrc", ".bash_profile", ".profile",
	"/etc/cron", "/etc/sudoers",
}

// Inspect implements Detector.
func (PersistenceDetector) Inspect(a Action) []Detection {
	for _, key := range []string{"path", "command"} {
		v, _ := a.Payload[key].(string)
		if v == "" {
			continue
		}
		low := strings.ToLower(v)
		for _, p := range persistencePaths {
			if strings.Contains(low, p) {
				return []Detection{{
					Rule: "persistence.host_weakening", Severe: true,
					Detail: fmt.Sprintf("action targets a persistence path (%s)", p),
				}}
			}
		}
	}
	return nil
}
