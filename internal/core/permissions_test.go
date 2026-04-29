package core

import (
	"slices"
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestDefaultPermissionConfigCoversPRDCategories(t *testing.T) {
	manager := NewPermissionManager(DefaultPermissionConfig())

	cases := []struct {
		category contracts.PermissionCategory
		want     contracts.PermissionAction
		scope    contracts.PermissionScope
	}{
		{contracts.PermissionFilesystemHost, contracts.PermissionActionDeny, contracts.PermissionScopeTask},
		{contracts.PermissionFilesystemSandbox, contracts.PermissionActionAllow, contracts.PermissionScopeSession},
		{contracts.PermissionNetwork, contracts.PermissionActionConfirm, contracts.PermissionScopeSession},
		{contracts.PermissionShell, contracts.PermissionActionAllow, contracts.PermissionScopeSession},
		{contracts.PermissionComputerUse, contracts.PermissionActionConfirm, contracts.PermissionScopeTask},
		{contracts.PermissionMobile, contracts.PermissionActionConfirm, contracts.PermissionScopeTask},
		{contracts.PermissionDestructive, contracts.PermissionActionConfirm, contracts.PermissionScopeTask},
		{contracts.PermissionExternalComms, contracts.PermissionActionConfirm, contracts.PermissionScopeTask},
		{contracts.PermissionCredentials, contracts.PermissionActionConfirm, contracts.PermissionScopeTask},
	}

	for _, tc := range cases {
		t.Run(string(tc.category), func(t *testing.T) {
			decision := manager.Evaluate(contracts.PermissionRequest{
				Category: tc.category,
				Resource: "target",
				TaskID:   "task-123",
			})

			if decision.Action != tc.want {
				t.Fatalf("Action = %q, want %q", decision.Action, tc.want)
			}
			if decision.Scope != tc.scope {
				t.Fatalf("Scope = %q, want %q", decision.Scope, tc.scope)
			}
			if strings.TrimSpace(decision.Reason) == "" {
				t.Fatal("Reason is empty")
			}
		})
	}

	audit := manager.AuditTrail()
	if len(audit) != len(cases) {
		t.Fatalf("len(AuditTrail) = %d, want %d", len(audit), len(cases))
	}
}

func TestPermissionManagerHonorsExplicitScopeAndAuditsOutcome(t *testing.T) {
	manager := NewPermissionManager(DefaultPermissionConfig())

	decision := manager.Evaluate(contracts.PermissionRequest{
		Category: contracts.PermissionNetwork,
		Resource: "github.com:443",
		Scope:    contracts.PermissionScopePersistent,
		TaskID:   "clone-repo",
		Reason:   "clone dependency",
	})

	if decision.Action != contracts.PermissionActionConfirm {
		t.Fatalf("Action = %q, want confirm", decision.Action)
	}
	if decision.Scope != contracts.PermissionScopePersistent {
		t.Fatalf("Scope = %q, want persistent", decision.Scope)
	}

	audit := manager.AuditTrail()
	if len(audit) != 1 {
		t.Fatalf("len(AuditTrail) = %d, want 1", len(audit))
	}
	if audit[0].Triggered != "clone-repo" {
		t.Fatalf("Triggered = %q, want clone-repo", audit[0].Triggered)
	}
	if audit[0].Granted || audit[0].Denied {
		t.Fatalf("confirm decision should be neither granted nor denied: %#v", audit[0])
	}
	if audit[0].Request.Reason != "clone dependency" {
		t.Fatalf("Request.Reason = %q, want clone dependency", audit[0].Request.Reason)
	}
}

func TestPermissionManagerDeniesUnknownCategory(t *testing.T) {
	manager := NewPermissionManager(DefaultPermissionConfig())

	decision := manager.Evaluate(contracts.PermissionRequest{
		Category: contracts.PermissionCategory("quantum_tunnel"),
		Resource: "/outside",
	})

	if decision.Action != contracts.PermissionActionDeny {
		t.Fatalf("Action = %q, want deny", decision.Action)
	}
	if decision.Scope != contracts.PermissionScopeTask {
		t.Fatalf("Scope = %q, want task", decision.Scope)
	}

	audit := manager.AuditTrail()
	if len(audit) != 1 || !audit[0].Denied {
		t.Fatalf("AuditTrail = %#v, want denied entry", audit)
	}
}

func TestPermissionAuditTrailReturnsCopy(t *testing.T) {
	manager := NewPermissionManager(DefaultPermissionConfig())
	manager.Evaluate(contracts.PermissionRequest{Category: contracts.PermissionShell})

	audit := manager.AuditTrail()
	audit[0].Decision.Action = contracts.PermissionActionDeny

	next := manager.AuditTrail()
	if next[0].Decision.Action != contracts.PermissionActionAllow {
		t.Fatalf("audit trail was mutated through returned slice")
	}
}

func TestEngineOwnsPermissionManagerAndLogsDecisions(t *testing.T) {
	engine := New("test")

	if engine.Permissions() == nil {
		t.Fatal("Permissions() returned nil")
	}

	decision := engine.EvaluatePermission(contracts.PermissionRequest{
		Category: contracts.PermissionExternalComms,
		Resource: "origin/main",
		TaskID:   "publish",
	})
	if decision.Action != contracts.PermissionActionConfirm {
		t.Fatalf("Action = %q, want confirm", decision.Action)
	}

	audit := engine.Permissions().AuditTrail()
	if len(audit) != 1 || audit[0].Triggered != "publish" {
		t.Fatalf("AuditTrail = %#v, want publish entry", audit)
	}

	log := engine.SessionLog()
	if !slices.ContainsFunc(log, func(entry contracts.SessionLogEntry) bool {
		return strings.Contains(entry.Message, "permission confirm external_comms origin/main")
	}) {
		t.Fatalf("SessionLog = %#v, want permission decision message", log)
	}
}
