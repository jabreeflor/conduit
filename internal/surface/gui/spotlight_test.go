package gui

import (
	"testing"
)

// staticSource is a SpotlightSource whose results are fixed at construction.
type staticSource struct {
	name string
	rows []SpotlightResult
}

func (s *staticSource) Name() string { return s.name }

func (s *staticSource) Search(query string) []SpotlightResult {
	out := make([]SpotlightResult, 0, len(s.rows))
	for _, r := range s.rows {
		score := Score(query, r.Title)
		if score == 0 {
			continue
		}
		r.Score = score
		out = append(out, r)
	}
	return out
}

func TestSpotlight_HiddenByDefault(t *testing.T) {
	s := NewSpotlight()
	if s.Visible() {
		t.Error("new Spotlight should not be visible")
	}
}

func TestSpotlight_ToggleAndShowHide(t *testing.T) {
	s := NewSpotlight()
	s.Toggle()
	if !s.Visible() {
		t.Error("after Toggle: expected visible")
	}
	s.Toggle()
	if s.Visible() {
		t.Error("after second Toggle: expected hidden")
	}
	s.Show()
	if !s.Visible() {
		t.Error("Show should make Spotlight visible")
	}
	s.Hide()
	if s.Visible() {
		t.Error("Hide should make Spotlight hidden")
	}
}

func TestSpotlight_QueryRanksResults(t *testing.T) {
	s := NewSpotlight()
	s.AddSource(&staticSource{name: "wf", rows: []SpotlightResult{
		{ID: "deploy", Kind: KindWorkflow, Title: "deploy"},
		{ID: "test", Kind: KindWorkflow, Title: "test suite"},
		{ID: "release", Kind: KindWorkflow, Title: "release notes"},
	}})

	s.SetQuery("test")
	results := s.Results()
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'test', got %d", len(results))
	}
	if results[0].ID != "test" {
		t.Errorf("expected 'test' result, got %q", results[0].ID)
	}
}

func TestSpotlight_PrefixOutranksSubstring(t *testing.T) {
	s := NewSpotlight()
	s.AddSource(&staticSource{name: "x", rows: []SpotlightResult{
		{ID: "long", Title: "do a thing"},      // substring "thing"
		{ID: "short", Title: "thingy command"}, // prefix
	}})
	s.SetQuery("thing")

	results := s.Results()
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "short" {
		t.Errorf("prefix match should rank above substring; got order: %v", results)
	}
}

func TestSpotlight_CursorMovesAndWraps(t *testing.T) {
	s := NewSpotlight()
	s.AddSource(&staticSource{name: "x", rows: []SpotlightResult{
		{ID: "a", Title: "alpha"},
		{ID: "b", Title: "alphabet"},
		{ID: "c", Title: "alphanumeric"},
	}})
	s.SetQuery("alph")

	if s.Cursor() != 0 {
		t.Errorf("initial cursor = %d, want 0", s.Cursor())
	}
	s.MoveCursor(1)
	if s.Cursor() != 1 {
		t.Errorf("after +1: cursor = %d, want 1", s.Cursor())
	}
	s.MoveCursor(5)
	if s.Cursor() != (1+5)%3 {
		t.Errorf("after +5 wrap: cursor = %d, want %d", s.Cursor(), (1+5)%3)
	}
	s.MoveCursor(-10)
	if s.Cursor() < 0 || s.Cursor() >= 3 {
		t.Errorf("cursor wrapped out of range: %d", s.Cursor())
	}
}

func TestSpotlight_ActivateRecordsRecentAndHides(t *testing.T) {
	s := NewSpotlight()
	s.AddSource(&staticSource{name: "x", rows: []SpotlightResult{
		{ID: "deploy", Kind: KindWorkflow, Title: "deploy prod"},
	}})
	s.Show()
	s.SetQuery("deploy")

	r := s.Activate()
	if r == nil || r.ID != "deploy" {
		t.Fatalf("Activate returned %+v, want deploy", r)
	}
	if s.Visible() {
		t.Error("Activate should hide the overlay")
	}
	rec := s.Recents()
	if len(rec) != 1 || rec[0].ID != "deploy" {
		t.Errorf("recents = %+v, want one entry for deploy", rec)
	}
}

func TestSpotlight_ActivateNoSelectionReturnsNil(t *testing.T) {
	s := NewSpotlight()
	if r := s.Activate(); r != nil {
		t.Errorf("Activate with no results returned %+v, want nil", r)
	}
}

func TestSpotlight_RecentsDeduplicateAndSurface(t *testing.T) {
	s := NewSpotlight()
	s.AddSource(&staticSource{name: "x", rows: []SpotlightResult{
		{ID: "deploy", Title: "deploy"},
		{ID: "build", Title: "build"},
	}})
	s.Show()
	s.SetQuery("deploy")
	s.Activate()
	s.Show()
	s.SetQuery("build")
	s.Activate()
	s.Show()
	s.SetQuery("deploy")
	s.Activate()

	rec := s.Recents()
	if len(rec) != 2 {
		t.Fatalf("recents len = %d, want 2 (deduped)", len(rec))
	}
	if rec[0].ID != "deploy" {
		t.Errorf("most-recent should be deploy, got %q", rec[0].ID)
	}

	// Empty query should surface recents.
	s.Show()
	s.SetQuery("")
	results := s.Results()
	if len(results) == 0 || results[0].Kind != KindRecent {
		t.Errorf("empty query should surface recents at top; got %+v", results)
	}
}

func TestSpotlight_QueryClearedKeepsRecents(t *testing.T) {
	s := NewSpotlight()
	s.AddSource(&staticSource{name: "x", rows: []SpotlightResult{{ID: "a", Title: "alpha"}}})
	s.Show()
	s.SetQuery("alph")
	s.Activate()

	if len(s.Recents()) != 1 {
		t.Fatal("expected 1 recent after Activate")
	}
	s.SetQuery("")
	if len(s.Recents()) != 1 {
		t.Errorf("recents should survive query clear; got %d", len(s.Recents()))
	}
}

func TestSpotlight_MaxResultsCap(t *testing.T) {
	rows := make([]SpotlightResult, 0, 30)
	for i := 0; i < 30; i++ {
		rows = append(rows, SpotlightResult{ID: string(rune('a' + i%26)), Title: "alpha"})
	}
	s := NewSpotlight()
	s.AddSource(&staticSource{name: "x", rows: rows})
	s.SetQuery("alpha")
	if got := len(s.Results()); got > 10 {
		t.Errorf("results = %d, want <= 10 (maxRes)", got)
	}
}

func TestSpotlight_ConcurrentAccess(t *testing.T) {
	s := NewSpotlight()
	s.AddSource(&staticSource{name: "x", rows: []SpotlightResult{{ID: "a", Title: "alpha"}}})
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			s.SetQuery("alpha")
			s.MoveCursor(1)
		}
		done <- struct{}{}
	}()
	for i := 0; i < 100; i++ {
		_ = s.Results()
		_ = s.Visible()
	}
	<-done
}

func TestScore(t *testing.T) {
	if Score("", "anything") == 0 {
		t.Error("empty query should match everything")
	}
	if Score("foo", "bar") != 0 {
		t.Error("non-match should score 0")
	}
	if Score("foo", "foobar") <= Score("foo", "barfoo") {
		t.Error("prefix should outrank substring")
	}
	if Score("foo", "foo") <= Score("foo", "foobar") {
		t.Error("exact match should outrank prefix")
	}
}
