package gui

import "testing"

func TestNew_defaults(t *testing.T) {
	l := New(1440, 900)
	if l.activeTab != TabSessions {
		t.Fatalf("expected default tab Sessions, got %d", l.activeTab)
	}
	if l.activeView != ViewScreenshot {
		t.Fatalf("expected default view Screenshot, got %d", l.activeView)
	}
}

func TestCompute_threeColumns(t *testing.T) {
	l := New(1440, 900)
	d := l.Compute()

	if !d.SidebarVisible {
		t.Fatal("expected sidebar visible at 1440 px width")
	}
	if d.SidebarWidth <= 0 {
		t.Fatalf("sidebar width must be positive, got %d", d.SidebarWidth)
	}
	if d.AgentPanelWidth <= 0 {
		t.Fatalf("agent panel width must be positive, got %d", d.AgentPanelWidth)
	}
	if d.MainWidth < minMainWidth {
		t.Fatalf("main area %d < min %d", d.MainWidth, minMainWidth)
	}
	total := d.SidebarWidth + d.MainWidth + d.AgentPanelWidth
	if total != d.TotalWidth {
		t.Fatalf("column widths %d+%d+%d = %d != total %d",
			d.SidebarWidth, d.MainWidth, d.AgentPanelWidth, total, d.TotalWidth)
	}
}

func TestCompute_sidebarCollapsed(t *testing.T) {
	l := New(1440, 900)
	l.ToggleSidebar()
	d := l.Compute()

	if d.SidebarVisible {
		t.Fatal("expected sidebar hidden after ToggleSidebar")
	}
	if d.SidebarWidth != 0 {
		t.Fatalf("collapsed sidebar should have width 0, got %d", d.SidebarWidth)
	}
	total := d.SidebarWidth + d.MainWidth + d.AgentPanelWidth
	if total != d.TotalWidth {
		t.Fatalf("widths %d+%d+%d != %d", d.SidebarWidth, d.MainWidth, d.AgentPanelWidth, d.TotalWidth)
	}
}

func TestCompute_narrowAutoCollapsesSidebar(t *testing.T) {
	// At 700 px the sidebar raw width would be ~126 px (< minSidebarWidth=180).
	l := New(700, 600)
	d := l.Compute()

	if d.SidebarVisible {
		t.Fatalf("expected sidebar auto-collapsed at 700 px, got visible")
	}
	if d.MainWidth < minMainWidth {
		t.Fatalf("main area %d < min %d on narrow window", d.MainWidth, minMainWidth)
	}
}

func TestCompute_toggleRestoresSidebar(t *testing.T) {
	l := New(1440, 900)
	l.ToggleSidebar() // collapse
	l.ToggleSidebar() // expand
	d := l.Compute()
	if !d.SidebarVisible {
		t.Fatal("sidebar should be visible after two toggles on a wide window")
	}
}

func TestCompute_resize(t *testing.T) {
	l := New(1440, 900)
	l.Resize(1000, 800)
	d := l.Compute()

	if d.TotalWidth != 1000 {
		t.Fatalf("expected total width 1000 after resize, got %d", d.TotalWidth)
	}
	total := d.SidebarWidth + d.MainWidth + d.AgentPanelWidth
	if total != 1000 {
		t.Fatalf("widths sum %d != 1000 after resize", total)
	}
}

func TestSelectTab(t *testing.T) {
	l := New(1440, 900)
	l.SelectTab(TabMemory)
	if l.ActiveTab() != TabMemory {
		t.Fatalf("expected TabMemory, got %d", l.ActiveTab())
	}
}

func TestSelectView(t *testing.T) {
	l := New(1440, 900)
	l.SelectView(ViewWorkflowDAG)
	if l.ActiveView() != ViewWorkflowDAG {
		t.Fatalf("expected ViewWorkflowDAG, got %d", l.ActiveView())
	}
}

func TestSetSidebarFrac_clamped(t *testing.T) {
	l := New(1440, 900)
	l.SetSidebarFrac(0.01) // below minimum
	if l.sidebarFrac < 0.10 {
		t.Fatalf("sidebar frac not clamped: %f", l.sidebarFrac)
	}
	l.SetSidebarFrac(0.99) // above maximum
	if l.sidebarFrac > 0.30 {
		t.Fatalf("sidebar frac not clamped: %f", l.sidebarFrac)
	}
}

func TestSetAgentPanelFrac_clamped(t *testing.T) {
	l := New(1440, 900)
	l.SetAgentPanelFrac(0.01)
	if l.agentPanelFrac < 0.20 {
		t.Fatalf("agent panel frac not clamped: %f", l.agentPanelFrac)
	}
	l.SetAgentPanelFrac(0.99)
	if l.agentPanelFrac > 0.40 {
		t.Fatalf("agent panel frac not clamped: %f", l.agentPanelFrac)
	}
}
