package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// ── Toggle / Visible ─────────────────────────────────────────────────────────

func TestNewContextPanelHiddenByDefault(t *testing.T) {
	p := NewContextPanel()
	if p.Visible() {
		t.Fatal("new panel should be hidden")
	}
}

func TestToggleFlipsVisibility(t *testing.T) {
	p := NewContextPanel()
	p.Toggle()
	if !p.Visible() {
		t.Fatal("after first toggle panel should be visible")
	}
	p.Toggle()
	if p.Visible() {
		t.Fatal("after second toggle panel should be hidden")
	}
}

func TestRenderReturnsEmptyWhenHidden(t *testing.T) {
	p := NewContextPanel()
	p.SetWorkflow([]WorkflowStep{{Index: 1, Name: "step one", Status: StepRunning}})
	if got := p.Render(); got != "" {
		t.Fatalf("hidden panel should render empty string, got %q", got)
	}
}

// ── View cycling ─────────────────────────────────────────────────────────────

func TestNewContextPanelStartsOnWorkflow(t *testing.T) {
	p := NewContextPanel()
	if p.ActiveView() != PanelWorkflow {
		t.Fatalf("expected PanelWorkflow, got %v", p.ActiveView())
	}
}

func TestCycleViewWrapsAround(t *testing.T) {
	p := NewContextPanel()
	views := []PanelView{PanelWorkflow, PanelMemory, PanelHooks, PanelSessionTree, PanelWorkflow}
	for i, want := range views {
		if p.ActiveView() != want {
			t.Fatalf("step %d: want view %v, got %v", i, want, p.ActiveView())
		}
		if i < len(views)-1 {
			p.CycleView()
		}
	}
}

func TestSetViewClampsToValidRange(t *testing.T) {
	p := NewContextPanel()
	p.SetView(PanelHooks)
	if p.ActiveView() != PanelHooks {
		t.Fatalf("expected PanelHooks")
	}
	p.SetView(PanelView(-1)) // out of range — should no-op
	if p.ActiveView() != PanelHooks {
		t.Fatal("out-of-range SetView should not change active view")
	}
	p.SetView(panelViewCount) // out of range
	if p.ActiveView() != PanelHooks {
		t.Fatal("out-of-range SetView should not change active view")
	}
}

// ── Tab bar ──────────────────────────────────────────────────────────────────

func TestTabBarMarksActiveView(t *testing.T) {
	p := NewContextPanel()
	tab := p.TabBar()
	if !strings.Contains(tab, "[workflow]") {
		t.Fatalf("tab bar %q should mark active view with brackets", tab)
	}
	if strings.Contains(tab, "[memory]") {
		t.Fatalf("tab bar %q should not bracket inactive views", tab)
	}
}

// ── Workflow view ─────────────────────────────────────────────────────────────

func TestRenderWorkflowSteps(t *testing.T) {
	p := NewContextPanel()
	p.Toggle()
	p.SetWorkflow([]WorkflowStep{
		{Index: 1, Name: "read context", Status: StepDone},
		{Index: 2, Name: "call model", Status: StepRunning},
		{Index: 3, Name: "write output", Status: StepPending},
	})
	out := p.Render()
	for _, want := range []string{"✓", "⟳", "○", "read context", "call model", "write output"} {
		if !strings.Contains(out, want) {
			t.Errorf("workflow render missing %q in:\n%s", want, out)
		}
	}
}

func TestRenderWorkflowEmptyState(t *testing.T) {
	p := NewContextPanel()
	p.Toggle()
	if !strings.Contains(p.Render(), "no workflow steps") {
		t.Fatal("empty workflow should show placeholder")
	}
}

func TestStepIconFailedShowsX(t *testing.T) {
	p := NewContextPanel()
	p.Toggle()
	p.SetWorkflow([]WorkflowStep{{Index: 1, Name: "broken step", Status: StepFailed}})
	if !strings.Contains(p.Render(), "✗") {
		t.Fatal("failed step should render ✗ icon")
	}
}

// ── Hook log view ────────────────────────────────────────────────────────────

func TestRenderHookLog(t *testing.T) {
	p := NewContextPanel()
	p.Toggle()
	p.SetView(PanelHooks)
	p.AppendHookEvent(HookEvent{
		At:     time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC),
		Hook:   "pre_tool_use",
		Result: "ok",
	})
	out := p.Render()
	if !strings.Contains(out, "pre_tool_use") {
		t.Errorf("hook log missing hook name in:\n%s", out)
	}
	if !strings.Contains(out, "ok") {
		t.Errorf("hook log missing result in:\n%s", out)
	}
}

func TestRenderHookLogEmptyState(t *testing.T) {
	p := NewContextPanel()
	p.Toggle()
	p.SetView(PanelHooks)
	if !strings.Contains(p.Render(), "no hook events") {
		t.Fatal("empty hook log should show placeholder")
	}
}

// ── Session tree view ─────────────────────────────────────────────────────────

func TestRenderSessionLogEntries(t *testing.T) {
	p := NewContextPanel()
	p.Toggle()
	p.SetView(PanelSessionTree)
	p.SetSessionLog([]contracts.SessionLogEntry{
		{At: time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC), Message: "model escalated"},
	})
	out := p.Render()
	if !strings.Contains(out, "model escalated") {
		t.Errorf("session tree missing log entry in:\n%s", out)
	}
}

func TestRenderSessionTreeNodes(t *testing.T) {
	p := NewContextPanel()
	p.Toggle()
	p.SetView(PanelSessionTree)
	p.SetSessionNodes([]SessionNode{
		{
			ID:    "sess-001",
			Label: "root session",
			Depth: 0,
			Children: []SessionNode{
				{ID: "sess-002", Label: "fork 1", Depth: 1},
			},
		},
	})
	out := p.Render()
	if !strings.Contains(out, "root session") {
		t.Errorf("session tree missing root node in:\n%s", out)
	}
	if !strings.Contains(out, "fork 1") {
		t.Errorf("session tree missing child node in:\n%s", out)
	}
}

func TestRenderSessionTreeEmptyState(t *testing.T) {
	p := NewContextPanel()
	p.Toggle()
	p.SetView(PanelSessionTree)
	if !strings.Contains(p.Render(), "no session events") {
		t.Fatal("empty session tree should show placeholder")
	}
}
