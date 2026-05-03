package gui

import "sync"

// CodingTab identifies one of the workspace tabs shown in the coding-agent
// GUI panel (PRD §6.24.24). The order of the constants is the order tabs
// render in the strip.
type CodingTab int

const (
	TabTasks CodingTab = iota
	TabPlan
	TabCodingMemory
	TabHistory
	TabBackground
	TabWorktree
	TabCodingSkills
	TabAccounts
	TabRemote
	TabMCP
	TabPlugins
	TabAskQueue
	TabWorkflows
	TabSearch
	TabTriggers
	TabTeams
	TabDiagnostics
)

// allCodingTabs is the canonical render order. New tabs must be added to both
// the iota block above and this slice.
var allCodingTabs = []CodingTab{
	TabTasks, TabPlan, TabCodingMemory, TabHistory, TabBackground, TabWorktree,
	TabCodingSkills, TabAccounts, TabRemote, TabMCP, TabPlugins, TabAskQueue,
	TabWorkflows, TabSearch, TabTriggers, TabTeams, TabDiagnostics,
}

// codingTabTitles maps each tab to its display label.
var codingTabTitles = map[CodingTab]string{
	TabTasks:        "Tasks",
	TabPlan:         "Plan",
	TabCodingMemory: "Memory",
	TabHistory:      "History",
	TabBackground:   "Background",
	TabWorktree:     "Worktree",
	TabCodingSkills: "Skills",
	TabAccounts:     "Accounts",
	TabRemote:       "Remote",
	TabMCP:          "MCP",
	TabPlugins:      "Plugins",
	TabAskQueue:     "Ask queue",
	TabWorkflows:    "Workflows",
	TabSearch:       "Search",
	TabTriggers:     "Triggers",
	TabTeams:        "Teams",
	TabDiagnostics:  "Diagnostics",
}

// Title returns the human-readable label for the tab.
func (t CodingTab) Title() string {
	if s, ok := codingTabTitles[t]; ok {
		return s
	}
	return "?"
}

// AllCodingTabs returns the canonical render order. The slice is a copy and
// safe to mutate.
func AllCodingTabs() []CodingTab {
	out := make([]CodingTab, len(allCodingTabs))
	copy(out, allCodingTabs)
	return out
}

// CodingTabState is the view-model for the coding-agent tab strip. It tracks
// the active tab plus per-tab badges (unread counts, error flags) so the
// renderer can decorate the tab strip without touching backend services.
//
// Safe for concurrent use: tabs may be activated from a UI thread while
// background services update badges.
type CodingTabState struct {
	mu      sync.RWMutex
	active  CodingTab
	badges  map[CodingTab]int  // unread / pending counts
	errored map[CodingTab]bool // tab is in an error state
	hidden  map[CodingTab]bool // user-hidden tabs (still in registry, not in strip)
}

// NewCodingTabState returns the default state with the Tasks tab active and
// no badges.
func NewCodingTabState() *CodingTabState {
	return &CodingTabState{
		active:  TabTasks,
		badges:  make(map[CodingTab]int),
		errored: make(map[CodingTab]bool),
		hidden:  make(map[CodingTab]bool),
	}
}

// Activate selects a tab. Hidden tabs cannot be activated.
func (c *CodingTabState) Activate(t CodingTab) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hidden[t] {
		return
	}
	c.active = t
}

// Active returns the currently selected tab.
func (c *CodingTabState) Active() CodingTab {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.active
}

// SetBadge sets the unread / pending counter for a tab. Counts ≤ 0 clear the
// badge.
func (c *CodingTabState) SetBadge(t CodingTab, n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if n <= 0 {
		delete(c.badges, t)
		return
	}
	c.badges[t] = n
}

// Badge returns the current count (0 when no badge).
func (c *CodingTabState) Badge(t CodingTab) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.badges[t]
}

// SetError marks a tab as errored. The renderer is expected to show an error
// indicator (red dot, exclamation, etc.).
func (c *CodingTabState) SetError(t CodingTab, errored bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if errored {
		c.errored[t] = true
	} else {
		delete(c.errored, t)
	}
}

// Errored reports whether the tab is in an error state.
func (c *CodingTabState) Errored(t CodingTab) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.errored[t]
}

// Hide removes the tab from the visible strip (it is still in AllCodingTabs).
// If the hidden tab was active, focus moves to the next visible tab.
func (c *CodingTabState) Hide(t CodingTab) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hidden[t] = true
	if c.active == t {
		c.active = c.nextVisibleLocked(t)
	}
}

// Unhide re-enables a previously hidden tab.
func (c *CodingTabState) Unhide(t CodingTab) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.hidden, t)
}

// Hidden reports whether the tab is hidden from the strip.
func (c *CodingTabState) Hidden(t CodingTab) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hidden[t]
}

// VisibleTabs returns the tabs currently shown in the strip, in canonical
// order.
func (c *CodingTabState) VisibleTabs() []CodingTab {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []CodingTab
	for _, t := range allCodingTabs {
		if !c.hidden[t] {
			out = append(out, t)
		}
	}
	return out
}

// Next moves focus to the next visible tab (wraps to the first).
func (c *CodingTabState) Next() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active = c.nextVisibleLocked(c.active)
}

// Prev moves focus to the previous visible tab (wraps to the last).
func (c *CodingTabState) Prev() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active = c.prevVisibleLocked(c.active)
}

// nextVisibleLocked returns the next visible tab after t (wrapping). Falls
// back to t when no other visible tab exists. Must be called with c.mu held.
func (c *CodingTabState) nextVisibleLocked(t CodingTab) CodingTab {
	idx := -1
	for i, v := range allCodingTabs {
		if v == t {
			idx = i
			break
		}
	}
	for i := 1; i <= len(allCodingTabs); i++ {
		cand := allCodingTabs[(idx+i)%len(allCodingTabs)]
		if !c.hidden[cand] {
			return cand
		}
	}
	return t
}

// prevVisibleLocked returns the previous visible tab before t (wrapping).
// Must be called with c.mu held.
func (c *CodingTabState) prevVisibleLocked(t CodingTab) CodingTab {
	idx := 0
	for i, v := range allCodingTabs {
		if v == t {
			idx = i
			break
		}
	}
	n := len(allCodingTabs)
	for i := 1; i <= n; i++ {
		cand := allCodingTabs[((idx-i)%n+n)%n]
		if !c.hidden[cand] {
			return cand
		}
	}
	return t
}
