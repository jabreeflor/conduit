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

// MountMode controls how a host path is exposed inside an isolated sandbox.
type MountMode string

const (
	MountModeReadOnly  MountMode = "read-only"
	MountModeReadWrite MountMode = "read-write"
	MountModeCopyIn    MountMode = "copy-in"
	MountModeCopyOut   MountMode = "copy-out"
)

// SandboxMount describes one explicit filesystem grant from the host into a
// sandbox.
type SandboxMount struct {
	HostPath                   string
	SandboxPath                string
	Mode                       MountMode
	AllowSensitivePathOverride bool
}

// DynamicMountRequest is the user-visible approval record emitted when an
// agent asks for filesystem access during a session.
type DynamicMountRequest struct {
	Mount                SandboxMount
	RequiresUserApproval bool
	Blocked              bool
	BlockReason          string
}

// SessionLogEntry is a user-visible event emitted by the engine.
type SessionLogEntry struct {
	At      time.Time
	Message string
}
