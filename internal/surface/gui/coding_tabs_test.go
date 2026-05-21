package gui

import "testing"

func TestCodingTabState_DefaultActiveTasks(t *testing.T) {
	c := NewCodingTabState()
	if c.Active() != TabTasks {
		t.Errorf("default active = %v, want TabTasks", c.Active())
	}
}

func TestCodingTabState_AllTabsHaveTitles(t *testing.T) {
	for _, tab := range AllCodingTabs() {
		if tab.Title() == "?" {
			t.Errorf("tab %d missing title", tab)
		}
	}
}

func TestCodingTabState_Activate(t *testing.T) {
	c := NewCodingTabState()
	c.Activate(TabPlan)
	if c.Active() != TabPlan {
		t.Errorf("active = %v, want TabPlan", c.Active())
	}
}

func TestCodingTabState_NextPrevWrap(t *testing.T) {
	c := NewCodingTabState()
	tabs := AllCodingTabs()

	c.Activate(tabs[len(tabs)-1])
	c.Next()
	if c.Active() != tabs[0] {
		t.Errorf("Next from last should wrap to first; got %v", c.Active())
	}

	c.Activate(tabs[0])
	c.Prev()
	if c.Active() != tabs[len(tabs)-1] {
		t.Errorf("Prev from first should wrap to last; got %v", c.Active())
	}
}

func TestCodingTabState_HideSkipsInNav(t *testing.T) {
	c := NewCodingTabState()
	c.Activate(TabTasks)
	c.Hide(TabPlan)
	c.Next()
	if c.Active() != TabCodingMemory {
		t.Errorf("Next should skip hidden TabPlan; got %v", c.Active())
	}
}

func TestCodingTabState_HideActiveMovesFocus(t *testing.T) {
	c := NewCodingTabState()
	c.Activate(TabPlan)
	c.Hide(TabPlan)
	if c.Active() == TabPlan {
		t.Error("hiding active tab should move focus away")
	}
}

func TestCodingTabState_ActivateHiddenIsNoop(t *testing.T) {
	c := NewCodingTabState()
	c.Hide(TabPlan)
	c.Activate(TabPlan)
	if c.Active() == TabPlan {
		t.Error("activating a hidden tab should be a no-op")
	}
}

func TestCodingTabState_BadgeSetClear(t *testing.T) {
	c := NewCodingTabState()
	c.SetBadge(TabAskQueue, 3)
	if got := c.Badge(TabAskQueue); got != 3 {
		t.Errorf("badge = %d, want 3", got)
	}
	c.SetBadge(TabAskQueue, 0)
	if got := c.Badge(TabAskQueue); got != 0 {
		t.Errorf("badge after clear = %d, want 0", got)
	}
	c.SetBadge(TabAskQueue, -5)
	if got := c.Badge(TabAskQueue); got != 0 {
		t.Errorf("badge after negative = %d, want 0", got)
	}
}

func TestCodingTabState_ErrorFlag(t *testing.T) {
	c := NewCodingTabState()
	c.SetError(TabMCP, true)
	if !c.Errored(TabMCP) {
		t.Error("expected errored")
	}
	c.SetError(TabMCP, false)
	if c.Errored(TabMCP) {
		t.Error("expected cleared")
	}
}

func TestCodingTabState_VisibleTabsExcludesHidden(t *testing.T) {
	c := NewCodingTabState()
	c.Hide(TabRemote)
	c.Hide(TabTeams)
	for _, t2 := range c.VisibleTabs() {
		if t2 == TabRemote || t2 == TabTeams {
			t.Errorf("hidden tab %v appeared in VisibleTabs", t2)
		}
	}
	if len(c.VisibleTabs()) != len(AllCodingTabs())-2 {
		t.Error("expected 2 tabs hidden from VisibleTabs")
	}
}

func TestCodingTabState_Concurrent(t *testing.T) {
	c := NewCodingTabState()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			c.SetBadge(TabHistory, i)
		}
		done <- struct{}{}
	}()
	for i := 0; i < 200; i++ {
		c.Next()
		_ = c.Active()
	}
	<-done
}
