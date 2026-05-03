// Package core contains the shared Conduit engine used by all surfaces.
package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/budget"
	cupermissions "github.com/jabreeflor/conduit/internal/computeruse/permissions"
	"github.com/jabreeflor/conduit/internal/config"
	"github.com/jabreeflor/conduit/internal/contextassembler"
	"github.com/jabreeflor/conduit/internal/contracts"
	"github.com/jabreeflor/conduit/internal/hooks"
	"github.com/jabreeflor/conduit/internal/memory"
	"github.com/jabreeflor/conduit/internal/security"
	"github.com/jabreeflor/conduit/internal/usage"
)

// Engine owns the long-lived runtime state for Conduit.
type Engine struct {
	name               string
	version            string
	startedAt          time.Time
	surfaces           []contracts.Surface
	identity           *IdentityManager
	router             *ModelRouter
	network            *NetworkSandbox
	permissions        *PermissionManager
	sandbox            *SandboxManager
	machineProfiler    *MachineProfiler
	firstRunSetup      *firstRunSetup
	budgetEnforcer     *budget.Enforcer
	sessionLog         []contracts.SessionLogEntry
	usage              *usage.Tracker
	sessionID          string
	activeWorkflow     string
	memRegistry        *memory.Registry
	hookDispatcher     *hooks.Dispatcher
	cwd                string
	computerUsePermMgr *cupermissions.Manager
}

// New creates a core engine instance with the surfaces planned for the
// monorepo scaffold.
func New(version string) *Engine {
	sessionID := fmt.Sprintf("%d", time.Now().UnixMilli())
	tracker, _ := usage.New(sessionID) // best-effort; nil tracker is handled in RecordUsage
	cwd, _ := os.Getwd()

	reg := &memory.Registry{}
	if provider, err := memory.NewFlatFileProvider(); err == nil {
		_ = provider.Initialize(context.Background())
		_ = reg.Register(contracts.MemoryProviderKindFlatFile, provider)
	}

	machineProfiler := NewMachineProfiler(DefaultMachineProfilerConfig())
	return &Engine{
		name:      "Conduit",
		version:   version,
		startedAt: time.Now().UTC(),
		surfaces: []contracts.Surface{
			contracts.SurfaceTUI,
			contracts.SurfaceGUI,
			contracts.SurfaceSpotlight,
		},
		identity:           NewIdentityManager(DefaultIdentityConfig()),
		router:             NewModelRouter(DefaultEscalationConfig()),
		network:            NewNetworkSandbox(DefaultNetworkSandboxConfig()),
		permissions:        NewPermissionManager(DefaultPermissionConfig()),
		sandbox:            NewSandboxManager(DefaultSandboxArchitecture()),
		machineProfiler:    machineProfiler,
		firstRunSetup:      newFirstRunSetup(machineProfiler, nil),
		budgetEnforcer:     newBudgetEnforcer(config.BudgetsConfig{}),
		usage:              tracker,
		sessionID:          sessionID,
		memRegistry:        reg,
		cwd:                cwd,
		computerUsePermMgr: cupermissions.NewManager(),
	}
}

func newUsageTracker(sessionID string, cfg config.CostConfig) (*usage.Tracker, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join(home, ".conduit", "usage.jsonl")
	pricingPath := expandHome(cfg.PricingPath, home)
	return usage.NewWithPathAndOptions(sessionID, logPath, usage.Options{
		PricingPath:              pricingPath,
		ElectricityRateUSDPerKWh: cfg.ElectricityRateUSDPerKWh,
	})
}

func expandHome(path, home string) string {
	if path == "" || home == "" {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}

// WithHooks attaches a hook dispatcher to the engine. Returns e for chaining.
func (e *Engine) WithHooks(d *hooks.Dispatcher) *Engine {
	e.hookDispatcher = d
	return e
}

// NewFromConfig creates a core engine initialised from a root config.
// Fields left at zero values in cfg fall back to their built-in defaults.
func NewFromConfig(version string, cfg config.Config) *Engine {
	sessionID := fmt.Sprintf("%d", time.Now().UnixMilli())
	tracker, _ := newUsageTracker(sessionID, cfg.Costs)

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

	reg := &memory.Registry{}
	if provider, err := memory.NewFlatFileProvider(); err == nil {
		_ = provider.Initialize(context.Background())
		_ = reg.Register(contracts.MemoryProviderKindFlatFile, provider)
	}

	machineProfiler := NewMachineProfiler(DefaultMachineProfilerConfig())
	return &Engine{
		name:      "Conduit",
		version:   version,
		startedAt: time.Now().UTC(),
		surfaces: []contracts.Surface{
			contracts.SurfaceTUI,
			contracts.SurfaceGUI,
			contracts.SurfaceSpotlight,
		},
		identity:           NewIdentityManager(DefaultIdentityConfig()),
		router:             NewModelRouter(escalation),
		network:            NewNetworkSandbox(DefaultNetworkSandboxConfig()),
		permissions:        NewPermissionManager(DefaultPermissionConfig()),
		sandbox:            NewSandboxManager(DefaultSandboxArchitecture()),
		machineProfiler:    machineProfiler,
		firstRunSetup:      newFirstRunSetup(machineProfiler, nil),
		budgetEnforcer:     newBudgetEnforcer(cfg.Budgets),
		usage:              tracker,
		sessionID:          sessionID,
		memRegistry:        reg,
		computerUsePermMgr: cupermissions.NewManager(),
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

// ComputerUsePermissions returns the engine-owned macOS Screen Recording +
// Accessibility permissions manager. Surfaces use it to drive the first-launch
// permissions flow described in PRD §6.8.
//
// The MCP-based computer-use runtime (issue #37) consumes this manager via
// EnsureComputerUseAllowed before starting a session. If #37 has not landed
// yet, this manager is still safe to call standalone.
func (e *Engine) ComputerUsePermissions() *cupermissions.Manager {
	if e.computerUsePermMgr == nil {
		e.computerUsePermMgr = cupermissions.NewManager()
	}
	return e.computerUsePermMgr
}

// ComputerUsePermissionReport runs the macOS permissions probe and emits a
// session-log entry per missing permission so the user can see why a session
// is blocked.
func (e *Engine) ComputerUsePermissionReport(ctx context.Context) contracts.ComputerUsePermissionReport {
	report := e.ComputerUsePermissions().Report(ctx)
	for _, s := range report.Statuses {
		if s.State == contracts.ComputerUsePermissionStateGranted {
			continue
		}
		if s.State == contracts.ComputerUsePermissionStateNotApplicable {
			continue
		}
		e.sessionLog = append(e.sessionLog, contracts.SessionLogEntry{
			At:      time.Now().UTC(),
			Message: fmt.Sprintf("computer-use permission %s: %s", s.Permission, s.State),
		})
	}
	return report
}

// EnsureComputerUseAllowed is the gate every computer-use session must call
// before kicking off the MCP runtime. Returns nil only when every required
// permission is granted (or NotApplicable on non-darwin). On a missing grant
// it returns *cupermissions.UngrantedError so surfaces can render the right
// "Open System Settings" UI.
//
// TODO(#37): once the MCP-based computer-use runtime is wired up, call this
// from its session-start path. Until then this is invokable standalone for
// preflight tooling.
func (e *Engine) EnsureComputerUseAllowed(ctx context.Context) error {
	return e.ComputerUsePermissions().EnsureSessionAllowed(ctx)
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

// RecordContextOptimization emits context assembler transparency in the
// session log. It satisfies router.OptimizationSink without coupling core to
// router internals.
func (e *Engine) RecordContextOptimization(_ context.Context, summary contextassembler.Summary) error {
	e.sessionLog = append(e.sessionLog, contracts.SessionLogEntry{
		At: time.Now().UTC(),
		Message: fmt.Sprintf(
			"context optimized: %d -> %d tokens, dropped=%d summarized=%d diffed=%d extracted=%d",
			summary.OriginalTokens,
			summary.FinalTokens,
			summary.DroppedItems,
			summary.SummarizedItems,
			summary.DiffedItems,
			summary.ExtractedItems,
		),
	})
	return nil
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

// RecordUsageWithOptions records richer accounting such as local inference
// duration and user electricity rate for energy-cost estimates.
func (e *Engine) RecordUsageWithOptions(provider, model string, inputTokens, outputTokens int, opts usage.RecordOptions) (contracts.UsageEntry, error) {
	if e.usage == nil {
		return contracts.UsageEntry{}, nil
	}
	if opts.MachineProfile.ProfiledAt.IsZero() && opts.LocalModel {
		if profile, err := e.machineProfiler.Load(); err == nil {
			opts.MachineProfile = profile
		}
	}
	return e.usage.RecordWithOptions(provider, model, inputTokens, outputTokens, opts)
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

// MemoryProvider returns the active memory.Provider, or nil if none is registered.
func (e *Engine) MemoryProvider() memory.Provider {
	p, _ := e.memRegistry.Active()
	return p
}

// WriteMemory persists entry via the active Provider, fires OnMemoryWrite on
// any registered HookProvider, and emits a session log entry.
func (e *Engine) WriteMemory(ctx context.Context, entry memory.Entry) error {
	p, _ := e.memRegistry.Active()
	if p == nil {
		return nil
	}
	if err := p.Write(ctx, entry); err != nil {
		return err
	}
	if hp, ok := p.(memory.HookProvider); ok {
		if fn := hp.Hooks().OnMemoryWrite; fn != nil {
			if err := fn(ctx, entry); err != nil {
				e.sessionLog = append(e.sessionLog, contracts.SessionLogEntry{
					At:      time.Now().UTC(),
					Message: fmt.Sprintf("memory hook OnMemoryWrite error: %v", err),
				})
			}
		}
	}
	e.sessionLog = append(e.sessionLog, contracts.SessionLogEntry{
		At:      time.Now().UTC(),
		Message: fmt.Sprintf("memory write: %s (%s)", entry.Title, entry.Kind),
	})
	return nil
}

// SearchMemory queries the active Provider and returns matching entries.
func (e *Engine) SearchMemory(ctx context.Context, query string) ([]memory.Entry, error) {
	p, _ := e.memRegistry.Active()
	if p == nil {
		return nil, nil
	}
	return p.Search(ctx, query)
}

// DeleteMemory removes a single memory entry by ID. Used by the TUI memory
// inspector for user-driven deletes; bypasses the Pinned guard because the
// user is acting explicitly.
func (e *Engine) DeleteMemory(ctx context.Context, id string) error {
	p, _ := e.memRegistry.Active()
	if p == nil {
		return nil
	}
	if err := p.Delete(ctx, id); err != nil {
		return err
	}
	e.sessionLog = append(e.sessionLog, contracts.SessionLogEntry{
		At:      time.Now().UTC(),
		Message: fmt.Sprintf("memory delete: %s", id),
	})
	return nil
}

// PruneMemory bulk-deletes entries matched by query, skipping pinned entries.
// Returns the IDs removed. Used by the TUI memory inspector's "prune matching"
// action.
func (e *Engine) PruneMemory(ctx context.Context, query string) ([]string, error) {
	p, _ := e.memRegistry.Active()
	if p == nil {
		return nil, nil
	}
	removed, err := p.Prune(ctx, query)
	if err != nil {
		return removed, err
	}
	e.sessionLog = append(e.sessionLog, contracts.SessionLogEntry{
		At:      time.Now().UTC(),
		Message: fmt.Sprintf("memory prune: %d entries (query=%q)", len(removed), query),
	})
	return removed, nil
}

// SetMemoryPinned toggles the protect/pin flag on a memory entry. Pinned
// entries are skipped by PruneMemory and any future automatic compactor.
func (e *Engine) SetMemoryPinned(ctx context.Context, id string, pinned bool) error {
	p, _ := e.memRegistry.Active()
	if p == nil {
		return nil
	}
	// Re-write through the provider so existing storage layout (file paths,
	// timestamps) is preserved without exposing a Pin-specific API.
	entries, err := p.Search(ctx, "")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.ID != id {
			continue
		}
		entry.Pinned = pinned
		if err := p.Write(ctx, entry); err != nil {
			return err
		}
		e.sessionLog = append(e.sessionLog, contracts.SessionLogEntry{
			At:      time.Now().UTC(),
			Message: fmt.Sprintf("memory pin: %s = %t", id, pinned),
		})
		return nil
	}
	return nil
}

// SessionLog returns a copy of user-visible engine events.
func (e *Engine) SessionLog() []contracts.SessionLogEntry {
	return append([]contracts.SessionLogEntry(nil), e.sessionLog...)
}

// FireSessionStart fires the on_session_start hook point.
func (e *Engine) FireSessionStart(ctx context.Context) hooks.Output {
	return e.hookDispatcher.Dispatch(ctx, hooks.Input{
		Event:     hooks.EventOnSessionStart,
		SessionID: e.sessionID,
		CWD:       e.cwd,
	})
}

// FireSessionEnd fires the on_session_end hook point.
func (e *Engine) FireSessionEnd(ctx context.Context) hooks.Output {
	return e.hookDispatcher.Dispatch(ctx, hooks.Input{
		Event:     hooks.EventOnSessionEnd,
		SessionID: e.sessionID,
		CWD:       e.cwd,
	})
}

// FirePreLLMCall fires the pre_llm_call hook point before every model inference.
func (e *Engine) FirePreLLMCall(ctx context.Context) hooks.Output {
	return e.hookDispatcher.Dispatch(ctx, hooks.Input{
		Event:     hooks.EventPreLLMCall,
		SessionID: e.sessionID,
		CWD:       e.cwd,
	})
}

// FirePostLLMCall fires the post_llm_call hook point after every model inference.
func (e *Engine) FirePostLLMCall(ctx context.Context) hooks.Output {
	return e.hookDispatcher.Dispatch(ctx, hooks.Input{
		Event:     hooks.EventPostLLMCall,
		SessionID: e.sessionID,
		CWD:       e.cwd,
	})
}

// FireMemoryWrite fires the on_memory_write hook point when the agent writes to
// long-term memory.
func (e *Engine) FireMemoryWrite(ctx context.Context, entry map[string]any) hooks.Output {
	return e.hookDispatcher.Dispatch(ctx, hooks.Input{
		Event:     hooks.EventOnMemoryWrite,
		ToolInput: entry,
		SessionID: e.sessionID,
		CWD:       e.cwd,
	})
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

// FirstRunSetup returns the welcome-screen state: machine profile,
// recommendation, local setup steps, and visible external API options.
func (e *Engine) FirstRunSetup() (contracts.FirstRunSetupSnapshot, error) {
	return e.firstRunSetup.Welcome()
}

// SetupLocalAI runs the one-click local setup action. The default installer
// adopts an existing local runtime; host-specific download/install backends can
// be injected in tests or platform frontends.
func (e *Engine) SetupLocalAI() (contracts.FirstRunSetupSnapshot, error) {
	snapshot, err := e.firstRunSetup.SetupLocalAI()
	if err != nil {
		e.sessionLog = append(e.sessionLog, contracts.SessionLogEntry{
			At:      time.Now().UTC(),
			Message: fmt.Sprintf("local AI setup needs attention: %v", err),
		})
		return snapshot, err
	}
	e.sessionLog = append(e.sessionLog, contracts.SessionLogEntry{
		At:      time.Now().UTC(),
		Message: fmt.Sprintf("local AI ready: %s via %s", snapshot.Recommendation.Name, snapshot.Runtime),
	})
	return snapshot, nil
}

// LocalModelRecommendations returns ranked local-model install choices derived
// from the cached machine profile. The heuristic is fully local and never phones
// home.
func (e *Engine) LocalModelRecommendations(opts contracts.LocalModelRecommendationOptions) (contracts.LocalModelRecommendationSet, error) {
	profile, err := e.machineProfiler.Load()
	if err != nil {
		return contracts.LocalModelRecommendationSet{}, err
	}
	return RecommendLocalModels(profile, opts), nil
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
