// Package contracts defines the shared data shapes used between the core
// engine and every user-facing surface.
package contracts

import "time"

// Surface identifies a frontend attached to the Conduit core.
type Surface string

const (
	SurfaceTUI       Surface = "tui"
	SurfaceGUI       Surface = "gui"
	SurfaceSpotlight Surface = "spotlight"
)

// EngineInfo is the minimal boot-time contract exposed by the core engine.
type EngineInfo struct {
	Name      string
	Version   string
	Surfaces  []Surface
	StartedAt time.Time
}

// TaskTag marks inference requests that need special routing treatment.
type TaskTag string

const (
	TaskTagDestructive TaskTag = "destructive"
	TaskTagPublish     TaskTag = "publish"
	TaskTagFinancial   TaskTag = "financial"
)

// PermissionCategory identifies the protected resource class for a tool or
// workflow action.
type PermissionCategory string

const (
	PermissionFilesystemHost    PermissionCategory = "filesystem_host"
	PermissionFilesystemSandbox PermissionCategory = "filesystem_sandbox"
	PermissionNetwork           PermissionCategory = "network"
	PermissionShell             PermissionCategory = "shell"
	PermissionComputerUse       PermissionCategory = "computer_use"
	PermissionMobile            PermissionCategory = "mobile"
	PermissionDestructive       PermissionCategory = "destructive"
	PermissionExternalComms     PermissionCategory = "external_comms"
	PermissionCredentials       PermissionCategory = "credentials"
)

// PermissionScope controls how long an approval remains valid.
type PermissionScope string

const (
	PermissionScopeTask       PermissionScope = "task"
	PermissionScopeSession    PermissionScope = "session"
	PermissionScopePersistent PermissionScope = "persistent"
)

// PermissionAction records whether a permission request was allowed, denied,
// or still needs explicit user confirmation.
type PermissionAction string

const (
	PermissionActionAllow   PermissionAction = "allow"
	PermissionActionDeny    PermissionAction = "deny"
	PermissionActionConfirm PermissionAction = "confirm"
)

// PermissionRequest describes one gated resource access attempt.
type PermissionRequest struct {
	Category PermissionCategory
	Resource string
	Scope    PermissionScope
	TaskID   string
	Reason   string
}

// PermissionDecision is the gate's answer for a permission request.
type PermissionDecision struct {
	Action   PermissionAction
	Category PermissionCategory
	Resource string
	Scope    PermissionScope
	Reason   string
}

// PermissionAuditEntry is the user-visible record for a grant, denial, or
// confirmation requirement.
type PermissionAuditEntry struct {
	At        time.Time
	Request   PermissionRequest
	Decision  PermissionDecision
	Granted   bool
	Denied    bool
	Triggered string
}

// EscalationReason explains why a request moved to the escalation model.
type EscalationReason string

const (
	EscalationReasonLowConfidence    EscalationReason = "low_confidence"
	EscalationReasonFirstWorkflowRun EscalationReason = "first_workflow_run"
	EscalationReasonHighStakesTask   EscalationReason = "high_stakes_task"
	EscalationReasonModelUncertainty EscalationReason = "model_uncertainty"
)

// ModelRouteRequest is the core engine's model-routing input.
type ModelRouteRequest struct {
	WorkflowType           string
	Confidence             float64
	Tags                   []TaskTag
	SelfSignalsUncertainty bool
}

// ModelRouteDecision records the selected model and transparent escalation
// details for surfaces and session logs.
type ModelRouteDecision struct {
	DefaultModel    string
	EscalationModel string
	SelectedModel   string
	Escalated       bool
	Reasons         []EscalationReason
}

// SessionLogEntry is a user-visible event emitted by the engine.
type SessionLogEntry struct {
	At      time.Time
	Message string
}
