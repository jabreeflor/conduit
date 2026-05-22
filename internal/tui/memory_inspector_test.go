package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/memory"
)

func sampleEntries() []memory.Entry {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	return []memory.Entry{
		{ID: "id-fact-1", Kind: memory.KindFact, Title: "Likes dark theme",
			Body: "User prefers dark UI everywhere.", Tags: []string{"ui"},
			CreatedAt: now, UpdatedAt: now},
		{ID: "id-decision-1", Kind: memory.KindDecision, Title: "Use FlatFileProvider",
			Body: "Spotlight indexes ~/.conduit/memory/.", Tags: []string{"memory", "design"},
			CreatedAt: now.Add(time.Hour), UpdatedAt: now.Add(time.Hour),
			Pinned: true},
		{ID: "id-pref-1", Kind: memory.KindPreference, Title: "Voice off by default",
			Body: "TTS disabled until invoked.", Tags: []string{"voice"},
			CreatedAt: now.Add(2 * time.Hour), UpdatedAt: now.Add(2 * time.Hour)},
	}
}

func TestInspectorEmptyState(t *testing.T) {
	mi := NewMemoryInspector()
	out := mi.Render()
	if !strings.Contains(out, "no memory entries") {
		t.Fatalf("empty inspector should render placeholder, got:\n%s", out)
	}
}

func TestInspectorListShowsEntries(t *testing.T) {
	mi := NewMemoryInspector()
	mi.SetEntries(sampleEntries())

	out := mi.Render()
	for _, want := range []string{"Likes dark theme", "Use FlatFileProvider", "Voice off by default"} {
		if !strings.Contains(out, want) {
			t.Errorf("inspector list missing %q in:\n%s", want, out)
		}
	}
	// pinned glyph should appear at least once
	if !strings.Contains(out, "📌") {
		t.Errorf("inspector list missing pinned glyph in:\n%s", out)
	}
}

func TestInspectorPinnedSurfaceFirst(t *testing.T) {
	mi := NewMemoryInspector()
	mi.SetEntries(sampleEntries())

	got := mi.Filtered()
	if len(got) == 0 || !got[0].Pinned {
		t.Fatalf("expected first filtered entry to be pinned, got %+v", got)
	}
}

func TestInspectorFilterMatchesTitleBodyTagsKind(t *testing.T) {
	mi := NewMemoryInspector()
	mi.SetEntries(sampleEntries())

	cases := map[string]int{
		"dark":      1, // title
		"Spotlight": 1, // body
		"voice":     1, // tag
		"decision":  1, // kind
		"nope-zzz":  0,
		"":          3,
	}
	for q, want := range cases {
		mi.SetFilter(q)
		got := len(mi.Filtered())
		if got != want {
			t.Errorf("filter %q: got %d entries, want %d", q, got, want)
		}
	}
}

func TestInspectorPruneCandidatesExcludePinned(t *testing.T) {
	mi := NewMemoryInspector()
	mi.SetEntries(sampleEntries())
	mi.SetFilter("") // include all
	candidates := mi.PruneCandidates()
	for _, e := range candidates {
		if e.Pinned {
			t.Fatalf("PruneCandidates returned pinned entry %s", e.ID)
		}
	}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 prune candidates (sample has 1 pinned of 3), got %d", len(candidates))
	}
}

func TestInspectorCursorBoundsClamped(t *testing.T) {
	mi := NewMemoryInspector()
	mi.SetEntries(sampleEntries())

	mi.CursorUp() // already at 0
	if mi.Cursor() != 0 {
		t.Fatalf("CursorUp at top should be no-op, cursor=%d", mi.Cursor())
	}
	mi.CursorEnd()
	if mi.Cursor() != len(mi.Filtered())-1 {
		t.Fatalf("CursorEnd should land on last entry, got %d", mi.Cursor())
	}
	mi.CursorDown()
	if mi.Cursor() != len(mi.Filtered())-1 {
		t.Fatalf("CursorDown at end should be no-op, got %d", mi.Cursor())
	}
}

func TestInspectorDetailModeRendersBody(t *testing.T) {
	mi := NewMemoryInspector()
	mi.SetEntries(sampleEntries())
	mi.OpenDetail()

	if mi.Mode() != inspectorDetail {
		t.Fatalf("OpenDetail should switch to detail mode, got %v", mi.Mode())
	}
	out := mi.Render()
	for _, want := range []string{"id:", "kind:", "tags:", "body"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail view missing %q in:\n%s", want, out)
		}
	}
}

// ── memory controller integration ────────────────────────────────────────────

// fakeController is an in-memory MemoryController for testing the inspector
// against the same surface the real engine implements.
type fakeController struct {
	entries map[string]memory.Entry
}

func newFakeController(seed []memory.Entry) *fakeController {
	c := &fakeController{entries: make(map[string]memory.Entry)}
	for _, e := range seed {
		c.entries[e.ID] = e
	}
	return c
}

func (c *fakeController) SearchMemory(_ context.Context, q string) ([]memory.Entry, error) {
	out := make([]memory.Entry, 0, len(c.entries))
	for _, e := range c.entries {
		if q == "" || matchesFilter(e, strings.ToLower(q)) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (c *fakeController) DeleteMemory(_ context.Context, id string) error {
	delete(c.entries, id)
	return nil
}

func (c *fakeController) PruneMemory(_ context.Context, q string) ([]string, error) {
	var removed []string
	ql := strings.ToLower(q)
	for id, e := range c.entries {
		if e.Pinned {
			continue
		}
		if q != "" && !matchesFilter(e, ql) {
			continue
		}
		delete(c.entries, id)
		removed = append(removed, id)
	}
	return removed, nil
}

func (c *fakeController) SetMemoryPinned(_ context.Context, id string, pinned bool) error {
	if e, ok := c.entries[id]; ok {
		e.Pinned = pinned
		c.entries[id] = e
	}
	return nil
}

func TestInspectorIntegration_DeleteAndPrune(t *testing.T) {
	ctrl := newFakeController(sampleEntries())
	m := newModel("test", testSetupSnapshot(), nil).WithMemoryController(ctrl)
	m = m.openMemoryInspector()

	if !m.inspectorOpen {
		t.Fatal("inspector should be open after openMemoryInspector")
	}
	if got := len(m.inspector.Filtered()); got != 3 {
		t.Fatalf("expected 3 entries loaded, got %d", got)
	}

	// Move cursor off the pinned entry then delete the highlighted entry.
	m.inspector.CursorDown()
	target, _ := m.inspector.Selected()
	if target.Pinned {
		t.Fatalf("test setup error: expected unpinned entry under cursor, got %s", target.Title)
	}
	m.inspector.PromptDelete()
	m = m.confirmDelete()
	if _, ok := ctrl.entries[target.ID]; ok {
		t.Fatalf("delete didn't remove %s from controller", target.ID)
	}

	// Prune everything (filter empty) — pinned must survive.
	m.inspector.SetFilter("")
	m = m.confirmPrune()

	leftPinned := false
	for _, e := range ctrl.entries {
		if e.Pinned {
			leftPinned = true
		} else {
			t.Fatalf("prune should have removed unpinned entry %s", e.ID)
		}
	}
	if !leftPinned {
		t.Fatal("prune removed the pinned entry; manual override broken")
	}
}

func TestInspectorIntegration_TogglePin(t *testing.T) {
	ctrl := newFakeController(sampleEntries())
	m := newModel("test", testSetupSnapshot(), nil).WithMemoryController(ctrl)
	m = m.openMemoryInspector()

	// Cursor starts on the pinned entry — toggling unpins it.
	first, _ := m.inspector.Selected()
	if !first.Pinned {
		t.Fatalf("expected first entry pinned, got %+v", first)
	}
	m = m.togglePinSelected()
	if ctrl.entries[first.ID].Pinned {
		t.Fatal("toggle did not unpin entry")
	}

	// Toggle again to re-pin.
	// Cursor index may now point at a different entry because re-sort moved
	// pinned entries around — re-find by ID to remain robust.
	for i, e := range m.inspector.Filtered() {
		if e.ID == first.ID {
			m.inspector.cursor = i
			break
		}
	}
	m = m.togglePinSelected()
	if !ctrl.entries[first.ID].Pinned {
		t.Fatal("toggle did not re-pin entry")
	}
}
