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

// SessionLogEntry is a user-visible event emitted by the engine.
type SessionLogEntry struct {
	At      time.Time
	Message string
}

// NetworkMode controls how sandboxed network access is approved.
type NetworkMode string

const (
	NetworkModeRestricted NetworkMode = "restricted"
	NetworkModePerRequest NetworkMode = "per_request"
	NetworkModeOpen       NetworkMode = "open"
	NetworkModeOffline    NetworkMode = "offline"
)

// NetworkDirection identifies the side of a network policy check.
type NetworkDirection string

const (
	NetworkDirectionOutbound NetworkDirection = "outbound"
	NetworkDirectionInbound  NetworkDirection = "inbound"
)

// NetworkRequest describes a network action before the sandbox allows it.
type NetworkRequest struct {
	Direction NetworkDirection
	Host      string
	Port      int
	Protocol  string
	URL       string
}

// NetworkDecision is the policy result for one network request.
type NetworkDecision struct {
	Allowed bool
	Reason  string
	Mode    NetworkMode
	Host    string
	Port    int
}

// PortForward is an explicit inbound exception into a sandbox.
type PortForward struct {
	Name       string
	ListenPort int
	TargetHost string
	TargetPort int
	Protocol   string
}

// NetworkEvent records DNS and traffic policy decisions for audit surfaces.
type NetworkEvent struct {
	At        time.Time
	Kind      string
	Direction NetworkDirection
	Host      string
	Port      int
	Protocol  string
	Allowed   bool
	Reason    string
}

// SandboxBackend identifies the host runtime used to create the isolated
// Linux execution environment.
type SandboxBackend string

const (
	SandboxBackendAppleVirtualization SandboxBackend = "apple_virtualization"
	SandboxBackendOCIContainer        SandboxBackend = "oci_container"
)

// SandboxNetworkPolicy controls outbound network access from agent-run code.
type SandboxNetworkPolicy string

const (
	SandboxNetworkPolicyControlledEgress SandboxNetworkPolicy = "controlled_egress"
	SandboxNetworkPolicyOffline          SandboxNetworkPolicy = "offline"
)

// SandboxMountMode describes how a user-approved host path appears inside the
// sandbox.
type SandboxMountMode string

const (
	SandboxMountReadOnly  SandboxMountMode = "read_only"
	SandboxMountReadWrite SandboxMountMode = "read_write"
	SandboxMountCopyIn    SandboxMountMode = "copy_in"
	SandboxMountCopyOut   SandboxMountMode = "copy_out"
)

// SandboxMount is an explicit filesystem grant from the host into the sandbox.
type SandboxMount struct {
	HostPath                   string
	SandboxPath                string
	Mode                       SandboxMountMode
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

// SandboxArchitecture describes the security and startup contract every agent
// execution sandbox must satisfy.
type SandboxArchitecture struct {
	Backend                   SandboxBackend
	BaseImage                 string
	ImagePrecached            bool
	WarmStartBudget           time.Duration
	Shells                    []string
	PreinstalledRuntimes      []string
	NetworkPolicy             SandboxNetworkPolicy
	Mounts                    []SandboxMount
	DenyHostFilesystem        bool
	DenyHostNetwork           bool
	DenyHostProcesses         bool
	DenyPrivilegeEscalation   bool
	DenyDockerSocket          bool
	DiscardUnexportedChanges  bool
	RequiresExplicitMounts    bool
	RequiresExplicitEgress    bool
	RequiresExplicitPortFwd   bool
	RequiresNonRootUser       bool
	RequiresProcessNamespace  bool
	RequiresFilesystemOverlay bool
}
