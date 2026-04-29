package core

import (
	"slices"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// EscalationConfig controls cheap-to-capable model routing.
type EscalationConfig struct {
	DefaultModel        string
	EscalationModel     string
	ConfidenceThreshold float64
	HighStakesTags      []contracts.TaskTag
}

// DefaultEscalationConfig matches the PRD's cheap-fast to capable model path.
func DefaultEscalationConfig() EscalationConfig {
	return EscalationConfig{
		DefaultModel:        "claude-haiku-4-5",
		EscalationModel:     "claude-opus-4-6",
		ConfidenceThreshold: 0.65,
		HighStakesTags: []contracts.TaskTag{
			contracts.TaskTagDestructive,
			contracts.TaskTagPublish,
			contracts.TaskTagFinancial,
		},
	}
}

// ModelRouter chooses an inference model and tracks workflow first-runs.
type ModelRouter struct {
	config        EscalationConfig
	seenWorkflows map[string]bool
}

// NewModelRouter creates a router with the supplied escalation configuration.
func NewModelRouter(config EscalationConfig) *ModelRouter {
	return &ModelRouter{
		config:        config,
		seenWorkflows: make(map[string]bool),
	}
}

// Route chooses the default model unless one or more escalation triggers fire.
func (r *ModelRouter) Route(req contracts.ModelRouteRequest) contracts.ModelRouteDecision {
	reasons := r.escalationReasons(req)
	selected := r.config.DefaultModel
	if len(reasons) > 0 {
		selected = r.config.EscalationModel
	}

	if req.WorkflowType != "" {
		r.seenWorkflows[req.WorkflowType] = true
	}

	return contracts.ModelRouteDecision{
		DefaultModel:    r.config.DefaultModel,
		EscalationModel: r.config.EscalationModel,
		SelectedModel:   selected,
		Escalated:       len(reasons) > 0,
		Reasons:         reasons,
	}
}

// Status returns a non-mutating model route summary for UI status surfaces.
func (r *ModelRouter) Status() contracts.ModelRouteDecision {
	return contracts.ModelRouteDecision{
		DefaultModel:    r.config.DefaultModel,
		EscalationModel: r.config.EscalationModel,
		SelectedModel:   r.config.DefaultModel,
	}
}

func (r *ModelRouter) escalationReasons(req contracts.ModelRouteRequest) []contracts.EscalationReason {
	var reasons []contracts.EscalationReason

	if req.Confidence > 0 && req.Confidence < r.config.ConfidenceThreshold {
		reasons = append(reasons, contracts.EscalationReasonLowConfidence)
	}
	if req.WorkflowType != "" && !r.seenWorkflows[req.WorkflowType] {
		reasons = append(reasons, contracts.EscalationReasonFirstWorkflowRun)
	}
	if hasHighStakesTag(req.Tags, r.config.HighStakesTags) {
		reasons = append(reasons, contracts.EscalationReasonHighStakesTask)
	}
	if req.SelfSignalsUncertainty {
		reasons = append(reasons, contracts.EscalationReasonModelUncertainty)
	}

	return reasons
}

func hasHighStakesTag(tags, highStakes []contracts.TaskTag) bool {
	for _, tag := range tags {
		if slices.Contains(highStakes, tag) {
			return true
		}
	}
	return false
}
