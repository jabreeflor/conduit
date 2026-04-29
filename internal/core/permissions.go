package core

import (
	"fmt"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// PermissionRule defines a default action for a permission category.
type PermissionRule struct {
	Category contracts.PermissionCategory
	Action   contracts.PermissionAction
	Scope    contracts.PermissionScope
	Reason   string
}

// PermissionConfig configures safe-by-default permission decisions.
type PermissionConfig struct {
	DefaultScope contracts.PermissionScope
	Rules        []PermissionRule
}

// DefaultPermissionConfig mirrors the PRD 15.4 category defaults.
func DefaultPermissionConfig() PermissionConfig {
	return PermissionConfig{
		DefaultScope: contracts.PermissionScopeSession,
		Rules: []PermissionRule{
			{
				Category: contracts.PermissionFilesystemHost,
				Action:   contracts.PermissionActionDeny,
				Scope:    contracts.PermissionScopeTask,
				Reason:   "host filesystem access requires an explicit mount",
			},
			{
				Category: contracts.PermissionFilesystemSandbox,
				Action:   contracts.PermissionActionAllow,
				Reason:   "sandbox-local filesystem is the agent workspace",
			},
			{
				Category: contracts.PermissionNetwork,
				Action:   contracts.PermissionActionConfirm,
				Reason:   "network access must match an approved allowlist",
			},
			{
				Category: contracts.PermissionShell,
				Action:   contracts.PermissionActionAllow,
				Reason:   "shell execution is allowed inside the sandbox",
			},
			{
				Category: contracts.PermissionComputerUse,
				Action:   contracts.PermissionActionConfirm,
				Scope:    contracts.PermissionScopeTask,
				Reason:   "desktop computer use requires separate approval",
			},
			{
				Category: contracts.PermissionMobile,
				Action:   contracts.PermissionActionConfirm,
				Scope:    contracts.PermissionScopeTask,
				Reason:   "mobile control requires separate approval",
			},
			{
				Category: contracts.PermissionDestructive,
				Action:   contracts.PermissionActionConfirm,
				Scope:    contracts.PermissionScopeTask,
				Reason:   "destructive operations require confirmation each time",
			},
			{
				Category: contracts.PermissionExternalComms,
				Action:   contracts.PermissionActionConfirm,
				Scope:    contracts.PermissionScopeTask,
				Reason:   "external communication requires confirmation each time",
			},
			{
				Category: contracts.PermissionCredentials,
				Action:   contracts.PermissionActionConfirm,
				Scope:    contracts.PermissionScopeTask,
				Reason:   "credential access must be scoped to a specific secret",
			},
		},
	}
}

// PermissionManager evaluates permission requests and records every outcome.
type PermissionManager struct {
	config PermissionConfig
	rules  map[contracts.PermissionCategory]PermissionRule
	audit  []contracts.PermissionAuditEntry
	now    func() time.Time
}

// NewPermissionManager creates a permission gate with the supplied config.
func NewPermissionManager(config PermissionConfig) *PermissionManager {
	if config.DefaultScope == "" {
		config.DefaultScope = contracts.PermissionScopeSession
	}

	rules := make(map[contracts.PermissionCategory]PermissionRule, len(config.Rules))
	for _, rule := range config.Rules {
		if rule.Category == "" {
			continue
		}
		if rule.Scope == "" {
			rule.Scope = config.DefaultScope
		}
		rules[rule.Category] = rule
	}

	return &PermissionManager{
		config: config,
		rules:  rules,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

// Evaluate checks one permission request and appends its audit record.
func (m *PermissionManager) Evaluate(req contracts.PermissionRequest) contracts.PermissionDecision {
	rule, ok := m.rules[req.Category]
	if !ok {
		rule = PermissionRule{
			Category: req.Category,
			Action:   contracts.PermissionActionDeny,
			Scope:    contracts.PermissionScopeTask,
			Reason:   "unknown permission category is denied by default",
		}
	}
	if rule.Scope == "" {
		rule.Scope = m.config.DefaultScope
	}
	if req.Scope == "" {
		req.Scope = rule.Scope
	}

	decision := contracts.PermissionDecision{
		Action:   rule.Action,
		Category: req.Category,
		Resource: req.Resource,
		Scope:    req.Scope,
		Reason:   rule.Reason,
	}
	if decision.Scope == "" {
		decision.Scope = m.config.DefaultScope
	}
	if decision.Reason == "" {
		decision.Reason = fmt.Sprintf("%s permission %s", req.Category, decision.Action)
	}

	m.audit = append(m.audit, contracts.PermissionAuditEntry{
		At:        m.now(),
		Request:   req,
		Decision:  decision,
		Granted:   decision.Action == contracts.PermissionActionAllow,
		Denied:    decision.Action == contracts.PermissionActionDeny,
		Triggered: req.TaskID,
	})

	return decision
}

// AuditTrail returns a copy of every permission grant, denial, and confirmation.
func (m *PermissionManager) AuditTrail() []contracts.PermissionAuditEntry {
	return append([]contracts.PermissionAuditEntry(nil), m.audit...)
}

func formatPermissionDecision(decision contracts.PermissionDecision) string {
	parts := []string{
		fmt.Sprintf("permission %s", decision.Action),
		string(decision.Category),
	}
	if strings.TrimSpace(decision.Resource) != "" {
		parts = append(parts, decision.Resource)
	}
	if decision.Scope != "" {
		parts = append(parts, fmt.Sprintf("scope=%s", decision.Scope))
	}
	if strings.TrimSpace(decision.Reason) != "" {
		parts = append(parts, fmt.Sprintf("reason=%s", decision.Reason))
	}
	return strings.Join(parts, " ")
}
