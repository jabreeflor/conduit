// Package core contains the shared Conduit engine used by all surfaces.
package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
	"github.com/jabreeflor/conduit/internal/hooks"
	"github.com/jabreeflor/conduit/internal/security"
	"github.com/jabreeflor/conduit/internal/usage"
)

// Engine owns the long-lived runtime state for Conduit.
type Engine struct {
	name        string
	version     string
	sessionID   string
	startedAt   time.Time
	surfaces    []contracts.Surface
	identity    *IdentityManager
	router      *ModelRouter
	network     *NetworkSandbox
	permissions *PermissionManager
	sandbox     *SandboxManager
	hooks       *hooks.Manager
	sessionLog  []contracts.SessionLogEntry
	usage       *usage.Tracker
}

// New creates a core engine instance with the surfaces planned for the
// monorepo scaffold.
func New(version string) *Engine {
	sessionID := fmt.Sprintf("%d", time.Now().UnixMilli())
	tracker, _ := usage.New(sessionID) // best-effort; nil tracker is handled in RecordUsage

	hookCfg, _ := hooks.LoadConfig() // best-effort; empty config means no hooks run

	return &Engine{
		name:      "Conduit",
		version:   version,
		sessionID: sessionID,
		startedAt: time.Now().UTC(),
		surfaces: []contracts.Surface{
			contracts.SurfaceTUI,
			contracts.SurfaceGUI,
			contracts.SurfaceSpotlight,
		},
		identity:    NewIdentityManager(DefaultIdentityConfig()),
		router:      NewModelRouter(DefaultEscalationConfig()),
		network:     NewNetworkSandbox(DefaultNetworkSandboxConfig()),
		permissions: NewPermissionManager(DefaultPermissionConfig()),
		sandbox:     NewSandboxManager(DefaultSandboxArchitecture()),
		hooks:       hooks.New(hookCfg, sessionID),
		usage:       tracker,
	}
}

// Info returns a stable summary that frontends can use during startup.
func (e *Engine) Info() contracts.EngineInfo {
	return contracts.EngineInfo{
		Name:      e.name,
		Version:   e.version,
		Surfaces:  append([]contracts.Surface(nil), e.surfaces...),
		StartedAt: e.startedAt,
	}
}

// SanitizeInjectedContent scans untrusted model context and strips injection
// attempts before the content reaches prompt assembly.
func (e *Engine) SanitizeInjectedContent(source security.ContentSource, content string) security.ScanResult {
	return security.ScanInjectedContent(source, content)
}

// Identity returns the engine-owned three-layer identity manager.
func (e *Engine) Identity() *IdentityManager {
	return e.identity
}

// Permissions returns the engine-owned permission gate.
func (e *Engine) Permissions() *PermissionManager {
	return e.permissions
}

// EvaluatePermission gates a protected resource access and records the decision
// in both the permission audit trail and the session log.
func (e *Engine) EvaluatePermission(req contracts.PermissionRequest) contracts.PermissionDecision {
	decision := e.permissions.Evaluate(req)
	e.sessionLog = append(e.sessionLog, contracts.SessionLogEntry{
		At:      time.Now().UTC(),
		Message: formatPermissionDecision(decision),
	})
	return decision
}

// RouteModel selects a model for an inference request and logs transparent
// escalation events for all surfaces.
func (e *Engine) RouteModel(req contracts.ModelRouteRequest) contracts.ModelRouteDecision {
	decision := e.router.Route(req)
	if decision.Escalated {
		e.sessionLog = append(e.sessionLog, contracts.SessionLogEntry{
			At:      time.Now().UTC(),
			Message: fmt.Sprintf("model escalated from %s to %s (%s)", decision.DefaultModel, decision.EscalationModel, joinReasons(decision.Reasons)),
		})
	}
	return decision
}

// ModelStatus returns the router state that a surface can show in its status
// area without mutating workflow first-run tracking.
func (e *Engine) ModelStatus() contracts.ModelRouteDecision {
	return e.router.Status()
}

// SandboxArchitecture returns the engine-owned execution sandbox policy.
func (e *Engine) SandboxArchitecture() contracts.SandboxArchitecture {
	return e.sandbox.Architecture()
}

// RecordUsage appends one model-call record to ~/.conduit/usage.jsonl and
// updates the in-memory session totals shown in the status bar.
// A nil tracker (e.g. disk unavailable at startup) is a no-op.
func (e *Engine) RecordUsage(provider, model string, inputTokens, outputTokens int) (contracts.UsageEntry, error) {
	if e.usage == nil {
		return contracts.UsageEntry{}, nil
	}
	return e.usage.Record(provider, model, inputTokens, outputTokens)
}

// UsageSummary returns the running session totals for the status bar.
func (e *Engine) UsageSummary() contracts.UsageSummary {
	if e.usage == nil {
		return contracts.UsageSummary{}
	}
	return e.usage.Summary()
}

// Hooks returns the engine-owned hook manager.
func (e *Engine) Hooks() *hooks.Manager {
	return e.hooks
}

// FireHook runs all hooks registered for event and records the decision in the
// session log when the outcome is not a plain allow.
func (e *Engine) FireHook(ctx context.Context, event hooks.EventType, input hooks.HookInput) (hooks.HookOutput, error) {
	out, err := e.hooks.Fire(ctx, event, input)
	if out.Decision != hooks.DecisionAllow {
		e.sessionLog = append(e.sessionLog, contracts.SessionLogEntry{
			At:      time.Now().UTC(),
			Message: hooks.FormatDecision(string(event), out),
		})
	}
	return out, err
}

// SessionLog returns a copy of user-visible engine events.
func (e *Engine) SessionLog() []contracts.SessionLogEntry {
	return append([]contracts.SessionLogEntry(nil), e.sessionLog...)
}

func joinReasons(reasons []contracts.EscalationReason) string {
	parts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		parts = append(parts, string(reason))
	}
	return strings.Join(parts, ", ")
}
