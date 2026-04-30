// Package core contains the shared Conduit engine used by all surfaces.
package core

import (
	"fmt"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/budget"
	"github.com/jabreeflor/conduit/internal/config"
	"github.com/jabreeflor/conduit/internal/contracts"
	"github.com/jabreeflor/conduit/internal/security"
	"github.com/jabreeflor/conduit/internal/usage"
)

// Engine owns the long-lived runtime state for Conduit.
type Engine struct {
	name            string
	version         string
	startedAt       time.Time
	surfaces        []contracts.Surface
	identity        *IdentityManager
	router          *ModelRouter
	network         *NetworkSandbox
	permissions     *PermissionManager
	sandbox         *SandboxManager
	machineProfiler *MachineProfiler
	budgetEnforcer  *budget.Enforcer
	sessionLog      []contracts.SessionLogEntry
	usage           *usage.Tracker
	sessionID       string
	activeWorkflow  string
}

// New creates a core engine instance with the surfaces planned for the
// monorepo scaffold.
func New(version string) *Engine {
	sessionID := fmt.Sprintf("%d", time.Now().UnixMilli())
	tracker, _ := usage.New(sessionID) // best-effort; nil tracker is handled in RecordUsage

	return &Engine{
		name:      "Conduit",
		version:   version,
		startedAt: time.Now().UTC(),
		surfaces: []contracts.Surface{
			contracts.SurfaceTUI,
			contracts.SurfaceGUI,
			contracts.SurfaceSpotlight,
		},
		identity:        NewIdentityManager(DefaultIdentityConfig()),
		router:          NewModelRouter(DefaultEscalationConfig()),
		network:         NewNetworkSandbox(DefaultNetworkSandboxConfig()),
		permissions:     NewPermissionManager(DefaultPermissionConfig()),
		sandbox:         NewSandboxManager(DefaultSandboxArchitecture()),
		machineProfiler: NewMachineProfiler(DefaultMachineProfilerConfig()),
		budgetEnforcer:  newBudgetEnforcer(config.BudgetsConfig{}),
		usage:           tracker,
		sessionID:       sessionID,
	}
}

// NewFromConfig creates a core engine initialised from a root config.
// Fields left at zero values in cfg fall back to their built-in defaults.
func NewFromConfig(version string, cfg config.Config) *Engine {
	sessionID := fmt.Sprintf("%d", time.Now().UnixMilli())
	tracker, _ := usage.New(sessionID)

	escalation := DefaultEscalationConfig()
	if cfg.Escalation.DefaultModel != "" {
		escalation.DefaultModel = cfg.Escalation.DefaultModel
	}
	if cfg.Escalation.EscalationModel != "" {
		escalation.EscalationModel = cfg.Escalation.EscalationModel
	}
	if cfg.Escalation.ConfidenceThreshold > 0 {
		escalation.ConfidenceThreshold = cfg.Escalation.ConfidenceThreshold
	}

	return &Engine{
		name:      "Conduit",
		version:   version,
		startedAt: time.Now().UTC(),
		surfaces: []contracts.Surface{
			contracts.SurfaceTUI,
			contracts.SurfaceGUI,
			contracts.SurfaceSpotlight,
		},
		identity:        NewIdentityManager(DefaultIdentityConfig()),
		router:          NewModelRouter(escalation),
		network:         NewNetworkSandbox(DefaultNetworkSandboxConfig()),
		permissions:     NewPermissionManager(DefaultPermissionConfig()),
		sandbox:         NewSandboxManager(DefaultSandboxArchitecture()),
		machineProfiler: NewMachineProfiler(DefaultMachineProfilerConfig()),
		budgetEnforcer:  newBudgetEnforcer(cfg.Budgets),
		usage:           tracker,
		sessionID:       sessionID,
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
	if req.WorkflowType != "" {
		e.activeWorkflow = req.WorkflowType
	}
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
	var s contracts.UsageSummary
	if e.usage != nil {
		s = e.usage.Summary()
	}
	s.SessionID = e.sessionID
	s.ActiveWorkflow = e.activeWorkflow
	return s
}

// SessionLog returns a copy of user-visible engine events.
func (e *Engine) SessionLog() []contracts.SessionLogEntry {
	return append([]contracts.SessionLogEntry(nil), e.sessionLog...)
}

// MachineProfile returns the cached hardware profile, running a fresh scan on
// first call or when no cache exists.
func (e *Engine) MachineProfile() (contracts.MachineProfile, error) {
	return e.machineProfiler.Load()
}

// RescanMachine runs a fresh hardware probe, overwrites the cache, and returns
// the new profile. Call this on user-triggered re-scan requests.
func (e *Engine) RescanMachine() (contracts.MachineProfile, error) {
	return e.machineProfiler.Scan()
}

// CheckBudget evaluates whether a model call with the given estimated cost is
// allowed under the configured monthly budgets. Returns ErrHardStop when the
// call would breach a hard-stop limit.
func (e *Engine) CheckBudget(model string, estimatedCostUSD float64) (budget.Decision, error) {
	if e.budgetEnforcer == nil {
		return budget.Decision{Allowed: true}, nil
	}
	return e.budgetEnforcer.Check(model, estimatedCostUSD)
}

// BudgetReport returns the full budget status for all configured limits.
func (e *Engine) BudgetReport() budget.Report {
	if e.budgetEnforcer == nil {
		return budget.Report{}
	}
	return e.budgetEnforcer.Report()
}

// newBudgetEnforcer constructs a budget enforcer, returning nil on failure so
// a missing home directory never prevents the engine from starting.
func newBudgetEnforcer(cfg config.BudgetsConfig) *budget.Enforcer {
	e, err := budget.New(cfg)
	if err != nil {
		return nil
	}
	return e
}

func joinReasons(reasons []contracts.EscalationReason) string {
	parts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		parts = append(parts, string(reason))
	}
	return strings.Join(parts, ", ")
}
