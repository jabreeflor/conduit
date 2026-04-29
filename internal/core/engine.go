// Package core contains the shared Conduit engine used by all surfaces.
package core

import (
	"fmt"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
	"github.com/jabreeflor/conduit/internal/security"
)

// Engine owns the long-lived runtime state for Conduit.
type Engine struct {
	name        string
	version     string
	startedAt   time.Time
	surfaces    []contracts.Surface
	identity    *IdentityManager
	router      *ModelRouter
	permissions *PermissionManager
	sandbox     *SandboxManager
	sessionLog  []contracts.SessionLogEntry
}

// New creates a core engine instance with the surfaces planned for the
// monorepo scaffold.
func New(version string) *Engine {
	return &Engine{
		name:      "Conduit",
		version:   version,
		startedAt: time.Now().UTC(),
		surfaces: []contracts.Surface{
			contracts.SurfaceTUI,
			contracts.SurfaceGUI,
			contracts.SurfaceSpotlight,
		},
		identity:    NewIdentityManager(DefaultIdentityConfig()),
		router:      NewModelRouter(DefaultEscalationConfig()),
		permissions: NewPermissionManager(DefaultPermissionConfig()),
		sandbox:     NewSandboxManager(DefaultSandboxArchitecture()),
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
