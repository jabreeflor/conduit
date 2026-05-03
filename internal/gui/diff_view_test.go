package gui

import (
	"strings"
	"testing"
)

func TestDiffView_PushAndActive(t *testing.T) {
	v := NewDiffView()
	if v.Active() != nil {
		t.Error("empty view should have no active entry")
	}
	e := NewDiffEntry("e1", "main.go", true, "abc")
	v.Push(e)
	if v.Active() == nil || v.Active().ID != "e1" {
		t.Error("Push should make the entry active")
	}
	if len(v.Entries()) != 1 {
		t.Errorf("entries = %d, want 1", len(v.Entries()))
	}
}

func TestDiffView_PushReplacesByID(t *testing.T) {
	v := NewDiffView()
	v.Push(NewDiffEntry("e1", "main.go", true, "abc"))
	e2 := NewDiffEntry("e1", "main.go", true, "def")
	v.Push(e2)
	if len(v.Entries()) != 1 {
		t.Errorf("re-push should replace; entries = %d", len(v.Entries()))
	}
	if v.Active().BaseSHA != "def" {
		t.Error("re-push should swap the entry")
	}
}

func TestDiffView_ModeToggle(t *testing.T) {
	v := NewDiffView()
	if v.Mode() != DiffModeUnified {
		t.Error("default mode should be unified")
	}
	v.SetMode(DiffModeSideBySide)
	if v.Mode() != DiffModeSideBySide {
		t.Error("SetMode failed")
	}
}

func TestDiffView_ApproveRejectHunk(t *testing.T) {
	v := NewDiffView()
	e := NewDiffEntry("e1", "main.go", true, "abc")
	e.AppendHunk(Hunk{ID: "h1"})
	e.AppendHunk(Hunk{ID: "h2"})
	v.Push(e)

	v.ApproveHunk("e1", "h1")
	v.RejectHunk("e1", "h2")

	hunks := v.Active().Hunks()
	if hunks[0].Status != HunkApproved {
		t.Errorf("h1 status = %v, want approved", hunks[0].Status)
	}
	if hunks[1].Status != HunkRejected {
		t.Errorf("h2 status = %v, want rejected", hunks[1].Status)
	}
}

func TestDiffView_PendingHunks(t *testing.T) {
	v := NewDiffView()
	e1 := NewDiffEntry("e1", "a.go", true, "")
	e1.AppendHunk(Hunk{ID: "h1"})
	e1.AppendHunk(Hunk{ID: "h2"})
	v.Push(e1)
	e2 := NewDiffEntry("e2", "b.go", true, "")
	e2.AppendHunk(Hunk{ID: "h3"})
	v.Push(e2)

	v.ApproveHunk("e1", "h1")
	pending := v.PendingHunks()
	if len(pending) != 2 {
		t.Errorf("pending = %d, want 2", len(pending))
	}
	want := map[string]bool{"h2": true, "h3": true}
	for _, p := range pending {
		if !want[p.HunkID] {
			t.Errorf("unexpected pending hunk %s", p.HunkID)
		}
	}
}

func TestDiffView_AnnotateAndForAgent(t *testing.T) {
	v := NewDiffView()
	e := NewDiffEntry("e1", "x.go", true, "")
	e.AppendHunk(Hunk{ID: "h1"})
	v.Push(e)

	v.Annotate("e1", "h1", 0, "use a constant here")
	v.Annotate("e1", "h1", 1, "extract this into a helper")

	notes := v.AnnotationsForAgent()
	if len(notes) != 2 {
		t.Errorf("got %d notes, want 2", len(notes))
	}
	if notes[0].Body != "use a constant here" {
		t.Errorf("note[0] body = %q", notes[0].Body)
	}
}

func TestParseUnifiedDiff_SimpleHunk(t *testing.T) {
	in := strings.Join([]string{
		"@@ -1,3 +1,3 @@",
		" line1",
		"-line2",
		"+line2-modified",
		" line3",
	}, "\n")

	e := ParseUnifiedDiff("e1", "x.go", true, "abc", in)
	hunks := e.Hunks()
	if len(hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(hunks))
	}
	h := hunks[0]
	if h.OldStart != 1 || h.NewStart != 1 {
		t.Errorf("starts = (%d,%d), want (1,1)", h.OldStart, h.NewStart)
	}
	if len(h.Lines) != 4 {
		t.Fatalf("lines = %d, want 4", len(h.Lines))
	}
	want := []LineKind{LineContext, LineRemoved, LineAdded, LineContext}
	for i, k := range want {
		if h.Lines[i].Kind != k {
			t.Errorf("line[%d].Kind = %v, want %v", i, h.Lines[i].Kind, k)
		}
	}
	if h.Lines[2].Text != "line2-modified" {
		t.Errorf("added line text = %q", h.Lines[2].Text)
	}
	// New-file line numbers should track adds + context.
	if h.Lines[2].NewLine != 2 {
		t.Errorf("added line NewLine = %d, want 2", h.Lines[2].NewLine)
	}
}

func TestParseUnifiedDiff_MultipleHunks(t *testing.T) {
	in := strings.Join([]string{
		"@@ -1,2 +1,2 @@",
		" a",
		"+b",
		"@@ -10,1 +11,1 @@",
		"-old",
		"+new",
	}, "\n")
	e := ParseUnifiedDiff("e", "f", true, "", in)
	hunks := e.Hunks()
	if len(hunks) != 2 {
		t.Fatalf("hunks = %d, want 2", len(hunks))
	}
	if hunks[1].OldStart != 10 || hunks[1].NewStart != 11 {
		t.Errorf("hunk2 starts = (%d,%d), want (10,11)", hunks[1].OldStart, hunks[1].NewStart)
	}
}

func TestDiffView_ClearAndSelect(t *testing.T) {
	v := NewDiffView()
	v.Push(NewDiffEntry("e1", "a", true, ""))
	v.Push(NewDiffEntry("e2", "b", true, ""))
	v.Select("e1")
	if v.Active().ID != "e1" {
		t.Errorf("Select failed; active = %s", v.Active().ID)
	}
	v.Clear()
	if len(v.Entries()) != 0 || v.Active() != nil {
		t.Error("Clear failed")
	}
}

func TestDiffView_Concurrent(t *testing.T) {
	v := NewDiffView()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			e := NewDiffEntry("e"+itoa(i), "f", true, "")
			e.AppendHunk(Hunk{ID: "h"})
			v.Push(e)
		}
		done <- struct{}{}
	}()
	for i := 0; i < 100; i++ {
		_ = v.Entries()
		_ = v.PendingHunks()
	}
	<-done
}
