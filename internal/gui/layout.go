package gui

// SidebarTab identifies the active navigation item in the left sidebar.
type SidebarTab int

const (
	TabSessions  SidebarTab = iota // session history list
	TabWorkflows                   // workflow DAG browser
	TabMemory                      // SOUL.md / USER.md / memory inspector
	TabSkills                      // skills registry
)

// MainView identifies the content rendered in the centre column.
type MainView int

const (
	ViewScreenshot    MainView = iota // live computer-use screenshot stream
	ViewCanvas                        // WKWebView / HTML canvas panel
	ViewWorkflowDAG                   // workflow step graph
	ViewMemoryBrowser                 // memory / SOUL / USER entries
	ViewDiffEditor                    // GitHub-style diff view (coding mode)
)

// defaults for column proportions (fractions of total width).
const (
	defaultSidebarFrac    = 0.18 // ~18 % of window
	defaultAgentPanelFrac = 0.28 // ~28 % of window
	minSidebarWidth       = 180  // pixels — below this the sidebar collapses
	minAgentPanelWidth    = 220  // pixels — agent panel never collapses
	minMainWidth          = 300  // pixels — centre column minimum
)

// Dimensions holds pixel measurements for one layout pass.
type Dimensions struct {
	TotalWidth  int
	TotalHeight int

	SidebarWidth    int
	MainWidth       int
	AgentPanelWidth int

	SidebarVisible bool
}

// Layout holds the mutable state of the three-column GUI window.
type Layout struct {
	width  int
	height int

	// user-resizable fractions (0–1); adjusted by drag handles
	sidebarFrac    float64
	agentPanelFrac float64

	sidebarCollapsed bool
	activeTab        SidebarTab
	activeView       MainView
}

// New returns a Layout initialised to the given window dimensions with
// default column proportions.
func New(width, height int) *Layout {
	return &Layout{
		width:          width,
		height:         height,
		sidebarFrac:    defaultSidebarFrac,
		agentPanelFrac: defaultAgentPanelFrac,
		activeTab:      TabSessions,
		activeView:     ViewScreenshot,
	}
}

// Resize updates the window dimensions and recomputes column widths.
func (l *Layout) Resize(width, height int) {
	l.width = width
	l.height = height
}

// ToggleSidebar collapses or expands the left sidebar.
func (l *Layout) ToggleSidebar() {
	l.sidebarCollapsed = !l.sidebarCollapsed
}

// SetSidebarFrac updates the sidebar width fraction from a drag handle event.
// f is clamped to a sensible range.
func (l *Layout) SetSidebarFrac(f float64) {
	if f < 0.10 {
		f = 0.10
	}
	if f > 0.30 {
		f = 0.30
	}
	l.sidebarFrac = f
}

// SetAgentPanelFrac updates the agent-panel width fraction.
func (l *Layout) SetAgentPanelFrac(f float64) {
	if f < 0.20 {
		f = 0.20
	}
	if f > 0.40 {
		f = 0.40
	}
	l.agentPanelFrac = f
}

// SelectTab switches the active sidebar navigation tab.
func (l *Layout) SelectTab(tab SidebarTab) {
	l.activeTab = tab
}

// SelectView switches the active main-area view.
func (l *Layout) SelectView(view MainView) {
	l.activeView = view
}

// ActiveTab returns the currently selected sidebar tab.
func (l *Layout) ActiveTab() SidebarTab { return l.activeTab }

// ActiveView returns the currently selected main-area view.
func (l *Layout) ActiveView() MainView { return l.activeView }

// Compute calculates concrete pixel widths for the current state.
// It is pure (no side-effects) so it can be called from render paths.
func (l *Layout) Compute() Dimensions {
	d := Dimensions{
		TotalWidth:  l.width,
		TotalHeight: l.height,
	}

	sidebarVisible := !l.sidebarCollapsed
	rawSidebar := int(float64(l.width) * l.sidebarFrac)
	if rawSidebar < minSidebarWidth {
		// auto-collapse when the window is too narrow
		sidebarVisible = false
	}

	agentPanel := int(float64(l.width) * l.agentPanelFrac)
	if agentPanel < minAgentPanelWidth {
		agentPanel = minAgentPanelWidth
	}

	var sidebarW int
	if sidebarVisible {
		sidebarW = rawSidebar
	}

	mainW := l.width - sidebarW - agentPanel
	if mainW < minMainWidth {
		// shrink the sidebar first to preserve the main area
		excess := minMainWidth - mainW
		sidebarW -= excess
		if sidebarW < minSidebarWidth {
			sidebarVisible = false
			sidebarW = 0
			mainW = l.width - agentPanel
		} else {
			mainW = minMainWidth
		}
	}

	d.SidebarVisible = sidebarVisible
	d.SidebarWidth = sidebarW
	d.MainWidth = mainW
	d.AgentPanelWidth = agentPanel
	return d
}
