// Package contracts defines the shared data shapes used between the core
// engine and every user-facing surface.
package contracts

import "time"

// HookEvent identifies which lifecycle point a user-defined hook fires.
// The seven values are the Codex-aligned events from PRD §6.25.17.
type HookEvent string

const (
	HookEventPreToolUse        HookEvent = "PreToolUse"
	HookEventPostToolUse       HookEvent = "PostToolUse"
	HookEventPermissionRequest HookEvent = "PermissionRequest"
	HookEventUserPromptSubmit  HookEvent = "UserPromptSubmit"
	HookEventStop              HookEvent = "Stop"
	HookEventAgentTurnComplete HookEvent = "agent-turn-complete"
	HookEventApprovalRequested HookEvent = "approval-requested"
)

// HookRegistration describes a user-defined command bound to a lifecycle event.
type HookRegistration struct {
	Event   HookEvent
	Command string
}

// MachineProfile is the hardware snapshot cached in ~/.conduit/machine-profile.json.
type MachineProfile struct {
	ProfiledAt   time.Time `json:"profiled_at"`
	MacOSVersion string    `json:"macos_version"`
	CPU          CPUInfo   `json:"cpu"`
	Memory       MemInfo   `json:"memory"`
	GPU          []GPUInfo `json:"gpu"`
	Disk         DiskInfo  `json:"disk"`
}

// CPUInfo holds processor identification and core counts.
type CPUInfo struct {
	Brand         string `json:"brand"`
	PhysicalCores int    `json:"physical_cores"`
	LogicalCores  int    `json:"logical_cores"`
}

// MemInfo holds total installed RAM.
type MemInfo struct {
	TotalBytes int64   `json:"total_bytes"`
	TotalGB    float64 `json:"total_gb"`
}

// GPUInfo holds one GPU adapter's name, VRAM, and whether it is shared (Apple
// Unified Memory) or dedicated (discrete AMD/NVIDIA).
type GPUInfo struct {
	Name     string  `json:"name"`
	VRAMGB   float64 `json:"vram_gb"`
	VRAMType string  `json:"vram_type"` // "shared" | "dedicated"
}

// DiskInfo holds capacity and free space for the root filesystem.
type DiskInfo struct {
	TotalBytes     int64   `json:"total_bytes"`
	AvailableBytes int64   `json:"available_bytes"`
	TotalGB        float64 `json:"total_gb"`
	AvailableGB    float64 `json:"available_gb"`
}

// FirstRunSetupPhase identifies where the zero-terminal onboarding flow is.
type FirstRunSetupPhase string

const (
	FirstRunSetupPhaseWelcome    FirstRunSetupPhase = "welcome"
	FirstRunSetupPhaseInstalling FirstRunSetupPhase = "installing"
	FirstRunSetupPhaseReady      FirstRunSetupPhase = "ready"
	FirstRunSetupPhaseExternal   FirstRunSetupPhase = "external_api"
)

// FirstRunSetupStepStatus is the user-visible progress state for one setup
// step.
type FirstRunSetupStepStatus string

const (
	FirstRunSetupStepPending FirstRunSetupStepStatus = "pending"
	FirstRunSetupStepRunning FirstRunSetupStepStatus = "running"
	FirstRunSetupStepDone    FirstRunSetupStepStatus = "done"
)

// FirstRunSetupStep is one item in the welcome setup progress list.
type FirstRunSetupStep struct {
	Name   string
	Status FirstRunSetupStepStatus
	Detail string
}

// ExternalAPIOption describes a non-local model path visible from welcome.
type ExternalAPIOption struct {
	Provider string
	Label    string
	EnvVar   string
}

// MachineClass is the local-inference capability tier inferred from a machine
// profile.
type MachineClass string

const (
	MachineClassHighEnd     MachineClass = "high_end"
	MachineClassMidRange    MachineClass = "mid_range"
	MachineClassEntryLevel  MachineClass = "entry_level"
	MachineClassConstrained MachineClass = "constrained"
)

// LocalModelUse identifies why a model is recommended.
type LocalModelUse string

const (
	LocalModelUseGeneral LocalModelUse = "general"
	LocalModelUseCode    LocalModelUse = "code"
)

// LocalModelRecommendation describes a locally bundled heuristic candidate.
type LocalModelRecommendation struct {
	Rank                  int           `json:"rank"`
	ID                    string        `json:"id"`
	Name                  string        `json:"name"`
	Use                   LocalModelUse `json:"use"`
	MachineClass          MachineClass  `json:"machine_class"`
	Quantization          string        `json:"quantization"`
	DownloadSizeGB        float64       `json:"download_size_gb"`
	DiskFootprintGB       float64       `json:"disk_footprint_gb"`
	EstimatedTokensPerSec float64       `json:"estimated_tokens_per_sec"`
	Recommended           bool          `json:"recommended"`
	FitsAvailableDisk     bool          `json:"fits_available_disk"`
	Notes                 []string      `json:"notes,omitempty"`
}

// LocalModelRecommendationOptions controls optional recommendation lanes.
type LocalModelRecommendationOptions struct {
	IncludeCodeModel bool
}

// LocalModelRecommendationSet is the ranked local-model recommendation result.
type LocalModelRecommendationSet struct {
	MachineClass    MachineClass               `json:"machine_class"`
	Recommendations []LocalModelRecommendation `json:"recommendations"`
	FallbackReason  string                     `json:"fallback_reason,omitempty"`
	GeneratedAt     time.Time                  `json:"generated_at"`
}

// FirstRunSetupSnapshot is the shared core-to-surface contract for the
// one-click setup welcome flow.
type FirstRunSetupSnapshot struct {
	Phase          FirstRunSetupPhase
	MachineProfile MachineProfile
	Recommendation LocalModelRecommendation
	Runtime        string
	Steps          []FirstRunSetupStep
	ExternalAPI    []ExternalAPIOption
	Ready          bool
}

// MemoryProviderKind names a bundled memory provider implementation.
type MemoryProviderKind string

const (
	MemoryProviderKindFlatFile MemoryProviderKind = "flatfile"
	MemoryProviderKindLanceDB  MemoryProviderKind = "lancedb"
	MemoryProviderKindSQLite   MemoryProviderKind = "sqlite"
	MemoryProviderKindNoOp     MemoryProviderKind = "noop"
)

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

// UsageEntry is one model-call record written to the usage JSONL log.
type UsageEntry struct {
	Timestamp       time.Time `json:"timestamp"`
	SessionID       string    `json:"session_id"`
	Provider        string    `json:"provider"`
	Model           string    `json:"model"`
	TokensIn        int       `json:"tokens_in"`
	TokensOut       int       `json:"tokens_out"`
	TotalTokens     int       `json:"total_tokens"`
	TTFMS           int64     `json:"ttft_ms"`
	TotalMS         int64     `json:"total_ms"`
	TokensPerSecond float64   `json:"tokens_per_sec"`
	Status          string    `json:"status"`
	ErrorType       string    `json:"error_type,omitempty"`
	Feature         string    `json:"feature,omitempty"`
	Plugin          string    `json:"plugin,omitempty"`
	CostUSD         float64   `json:"cost_usd"`
	CostCurrency    string    `json:"cost_currency,omitempty"`
	CostEstimated   bool      `json:"cost_estimated,omitempty"`
	CostSource      string    `json:"cost_source,omitempty"`

	InferenceSeconds         float64 `json:"inference_seconds,omitempty"`
	EstimatedPowerDrawWatts  float64 `json:"estimated_power_draw_watts,omitempty"`
	ElectricityRateUSDPerKWh float64 `json:"electricity_rate_usd_per_kwh,omitempty"`
	EstimatedLocalCostUSD    float64 `json:"estimated_local_cost_usd,omitempty"`
	LocalComparisonEstimated bool    `json:"local_comparison_estimated,omitempty"`

	// Legacy fields are retained so budget readers can still scan older
	// ~/.conduit/usage.jsonl records written before daily session logs.
	At           time.Time `json:"at,omitempty"`
	InputTokens  int       `json:"input_tokens,omitempty"`
	OutputTokens int       `json:"output_tokens,omitempty"`
}

// UsageSummary is the running totals for the status bar.
type UsageSummary struct {
	Model          string
	SessionID      string
	TotalTokens    int
	TotalCostUSD   float64
	ActiveWorkflow string // empty when no workflow is running
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

// SandboxRuntimeCapabilities reports what a backend can provide on the current
// host. A RuntimeProbe populates this for the selector (PRD §15.9).
type SandboxRuntimeCapabilities struct {
	Backend           SandboxBackend
	Available         bool
	UnavailableReason string
	SupportsRosetta2  bool
	SupportsVirtioFS  bool
	ColdStartBudget   time.Duration
	MemoryOverheadMB  int
}

// ToolStatus identifies the execution state of a tool call.
type ToolStatus int

const (
	ToolStatusRunning ToolStatus = iota
	ToolStatusDone
	ToolStatusFailed
)

// ToolCall holds the data for one tool invocation visible to the user.
type ToolCall struct {
	Name     string
	Input    string
	Output   string
	Status   ToolStatus
	Expanded bool
}

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
	ColdStartBudget           time.Duration
	WarmStartBudget           time.Duration
	MaxMemoryOverheadMB       int
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

// ComputerUsePermission identifies a macOS TCC permission required by the
// computer-use surface (PRD §6.8).
type ComputerUsePermission string

const (
	// ComputerUsePermissionScreenRecording is the macOS Screen Recording grant
	// required to capture before/after screenshots and observe the desktop.
	ComputerUsePermissionScreenRecording ComputerUsePermission = "screen_recording"
	// ComputerUsePermissionAccessibility is the macOS Accessibility grant
	// required to drive the system via the Accessibility API.
	ComputerUsePermissionAccessibility ComputerUsePermission = "accessibility"
)

// ComputerUsePermissionState records whether a single TCC grant is present.
type ComputerUsePermissionState string

const (
	// ComputerUsePermissionStateGranted means the host has confirmed the grant.
	ComputerUsePermissionStateGranted ComputerUsePermissionState = "granted"
	// ComputerUsePermissionStateMissing means the host explicitly reports the
	// grant is not present.
	ComputerUsePermissionStateMissing ComputerUsePermissionState = "missing"
	// ComputerUsePermissionStateUnknown means the host could not determine the
	// state (probe failed, missing toolchain, non-darwin, etc).
	ComputerUsePermissionStateUnknown ComputerUsePermissionState = "unknown"
	// ComputerUsePermissionStateNotApplicable means the host platform does not
	// require this permission (non-darwin).
	ComputerUsePermissionStateNotApplicable ComputerUsePermissionState = "not_applicable"
)

// ComputerUsePermissionStatus is the per-permission probe result with a
// deep-link URL pointing at the System Settings pane that grants it.
type ComputerUsePermissionStatus struct {
	Permission   ComputerUsePermission      `json:"permission"`
	State        ComputerUsePermissionState `json:"state"`
	SettingsURL  string                     `json:"settings_url,omitempty"`
	Detail       string                     `json:"detail,omitempty"`
	ProbedAt     time.Time                  `json:"probed_at"`
	ProbeCommand string                     `json:"probe_command,omitempty"`
}

// ComputerUsePermissionReport bundles the permissions required before a
// computer-use session can start.
type ComputerUsePermissionReport struct {
	Platform    string                        `json:"platform"`
	Required    []ComputerUsePermission       `json:"required"`
	Statuses    []ComputerUsePermissionStatus `json:"statuses"`
	AllGranted  bool                          `json:"all_granted"`
	GeneratedAt time.Time                     `json:"generated_at"`
}
