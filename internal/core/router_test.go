package core

import (
	"slices"
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestRouteModelEscalatesOnConfiguredTriggers(t *testing.T) {
	engine := New("test")

	decision := engine.RouteModel(contracts.ModelRouteRequest{
		WorkflowType:           "release",
		Confidence:             0.42,
		Tags:                   []contracts.TaskTag{contracts.TaskTagPublish},
		SelfSignalsUncertainty: true,
	})

	if !decision.Escalated {
		t.Fatal("decision did not escalate")
	}
	if decision.SelectedModel != "claude-opus-4-6" {
		t.Fatalf("SelectedModel = %q, want claude-opus-4-6", decision.SelectedModel)
	}

	wantReasons := []contracts.EscalationReason{
		contracts.EscalationReasonLowConfidence,
		contracts.EscalationReasonFirstWorkflowRun,
		contracts.EscalationReasonHighStakesTask,
		contracts.EscalationReasonModelUncertainty,
	}
	for _, reason := range wantReasons {
		if !slices.Contains(decision.Reasons, reason) {
			t.Fatalf("Reasons = %v, want %q", decision.Reasons, reason)
		}
	}

	log := engine.SessionLog()
	if len(log) != 1 {
		t.Fatalf("len(SessionLog) = %d, want 1", len(log))
	}
	if !strings.Contains(log[0].Message, "model escalated from claude-haiku-4-5 to claude-opus-4-6") {
		t.Fatalf("log message %q does not describe escalation", log[0].Message)
	}
}

func TestRouteModelDoesNotRepeatFirstRunEscalation(t *testing.T) {
	engine := New("test")

	first := engine.RouteModel(contracts.ModelRouteRequest{
		WorkflowType: "daily-review",
		Confidence:   0.9,
	})
	if !slices.Contains(first.Reasons, contracts.EscalationReasonFirstWorkflowRun) {
		t.Fatalf("first Reasons = %v, want first workflow run", first.Reasons)
	}

	next := engine.RouteModel(contracts.ModelRouteRequest{
		WorkflowType: "daily-review",
		Confidence:   0.9,
	})
	if next.Escalated {
		t.Fatalf("second decision escalated with Reasons = %v", next.Reasons)
	}
	if next.SelectedModel != "claude-haiku-4-5" {
		t.Fatalf("SelectedModel = %q, want claude-haiku-4-5", next.SelectedModel)
	}
}

func TestModelStatusDoesNotMarkWorkflowSeen(t *testing.T) {
	engine := New("test")

	status := engine.ModelStatus()
	if status.SelectedModel != "claude-haiku-4-5" {
		t.Fatalf("status SelectedModel = %q, want claude-haiku-4-5", status.SelectedModel)
	}

	decision := engine.RouteModel(contracts.ModelRouteRequest{
		WorkflowType: "new-workflow",
		Confidence:   0.9,
	})
	if !slices.Contains(decision.Reasons, contracts.EscalationReasonFirstWorkflowRun) {
		t.Fatalf("Reasons = %v, want first workflow run", decision.Reasons)
	}
}
