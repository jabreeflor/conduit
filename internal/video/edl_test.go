package video

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestEDL_AddTrimSplit(t *testing.T) {
	e := New()
	id := e.AddClip(Clip{Kind: TrackVideo, Source: "a.mov", In: 0, Out: 10 * time.Second, Start: 0})
	if id == "" {
		t.Fatal("AddClip returned empty id")
	}
	if got := e.Duration(); got != 10*time.Second {
		t.Errorf("duration = %v, want 10s", got)
	}
	if err := e.Trim(id, time.Second, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if got := e.Duration(); got != 4*time.Second {
		t.Errorf("after trim duration = %v, want 4s", got)
	}
	rightID, err := e.Split(id, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if rightID == "" {
		t.Fatal("Split returned empty id")
	}
	if len(e.Clips) != 2 {
		t.Errorf("clip count = %d, want 2", len(e.Clips))
	}
}

func TestEDL_TrimErrors(t *testing.T) {
	e := New()
	if err := e.Trim("missing", 0, 1); err == nil {
		t.Error("expected not-found error")
	}
	id := e.AddClip(Clip{Out: 10 * time.Second})
	if err := e.Trim(id, 5*time.Second, 5*time.Second); err == nil {
		t.Error("expected out>in error")
	}
}

func TestEDL_SplitErrors(t *testing.T) {
	e := New()
	id := e.AddClip(Clip{Out: 4 * time.Second})
	if _, err := e.Split(id, 10*time.Second); err == nil {
		t.Error("expected outside-clip error")
	}
	if _, err := e.Split("nope", time.Second); err == nil {
		t.Error("expected not-found error")
	}
}

func TestEDL_Cut_RemovesAndShifts(t *testing.T) {
	e := New()
	e.AddClip(Clip{ID: "a", Kind: TrackVideo, In: 0, Out: 10 * time.Second, Start: 0})
	e.AddClip(Clip{ID: "b", Kind: TrackVideo, In: 0, Out: 5 * time.Second, Start: 10 * time.Second})
	// Cut middle of "a"
	e.Cut(2*time.Second, 4*time.Second)
	if got := e.Duration(); got != 13*time.Second {
		t.Errorf("duration = %v, want 13s", got)
	}
}

func TestEDL_Captions(t *testing.T) {
	e := New()
	e.AddCaption(Caption{Start: 5 * time.Second, End: 6 * time.Second, Text: "two"})
	e.AddCaption(Caption{Start: 1 * time.Second, End: 2 * time.Second, Text: "one"})
	if e.Captions[0].Text != "one" {
		t.Errorf("captions not sorted: %+v", e.Captions)
	}
}

func TestEDL_Markers(t *testing.T) {
	e := New()
	e.AddMarker(Marker{At: 3 * time.Second, Label: "b"})
	e.AddMarker(Marker{At: 1 * time.Second, Label: "a"})
	if e.Markers[0].Label != "a" {
		t.Errorf("markers not sorted: %+v", e.Markers)
	}
}

func TestEDL_Transition(t *testing.T) {
	e := New()
	a := e.AddClip(Clip{})
	b := e.AddClip(Clip{})
	if err := e.AddTransition(a, b, "fade", time.Second); err != nil {
		t.Fatal(err)
	}
	if len(e.Clips[0].Effects) != 1 {
		t.Errorf("transition not attached: %+v", e.Clips[0].Effects)
	}
	if err := e.AddTransition("missing", b, "fade", time.Second); err == nil {
		t.Error("expected not-found error")
	}
}

func TestEDL_Music(t *testing.T) {
	e := New()
	e.SetMusic(MusicTrack{Source: "bgm.mp3", Volume: 0.4, AutoDuckdB: -10})
	if e.Music == nil || e.Music.Source != "bgm.mp3" {
		t.Errorf("music not set: %+v", e.Music)
	}
}

// --- planner tests ----------------------------------------------------

type fakeModel struct {
	reply string
	err   error
}

func (f *fakeModel) Complete(_ context.Context, _, _ string) (string, error) {
	return f.reply, f.err
}

func TestPlanner_ParseAndApply(t *testing.T) {
	plan := `[
		{"kind":"cut","params":{"start":"1s","end":"2s"}},
		{"kind":"caption","params":{"start":"0s","end":"500ms","text":"Hi"}}
	]`
	p := NewPlanner(&fakeModel{reply: plan})
	e := New()
	e.AddClip(Clip{ID: "a", Out: 10 * time.Second})
	ops, err := p.Plan(context.Background(), "do stuff", e)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 {
		t.Fatalf("ops = %d, want 2", len(ops))
	}
	if err := Apply(e, ops); err != nil {
		t.Fatal(err)
	}
	if len(e.Captions) != 1 {
		t.Errorf("caption not applied: %+v", e.Captions)
	}
	if e.Duration() != 9*time.Second {
		t.Errorf("cut not applied; duration = %v", e.Duration())
	}
}

func TestPlanner_FencedJSON(t *testing.T) {
	wrapped := "```json\n[]\n```"
	p := NewPlanner(&fakeModel{reply: wrapped})
	ops, err := p.Plan(context.Background(), "x", New())
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 0 {
		t.Errorf("expected 0 ops, got %d", len(ops))
	}
}

func TestPlanner_BadJSON(t *testing.T) {
	p := NewPlanner(&fakeModel{reply: "not json"})
	_, err := p.Plan(context.Background(), "x", New())
	if err == nil || !strings.Contains(err.Error(), "parse plan") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlanner_Validations(t *testing.T) {
	if _, err := NewPlanner(nil).Plan(context.Background(), "x", New()); err == nil {
		t.Error("expected nil-model error")
	}
	if _, err := NewPlanner(&fakeModel{}).Plan(context.Background(), "", New()); err == nil {
		t.Error("expected empty-intent error")
	}
	if _, err := NewPlanner(&fakeModel{err: errors.New("boom")}).Plan(context.Background(), "x", New()); err == nil {
		t.Error("expected model error")
	}
}

func TestApply_UnknownOp(t *testing.T) {
	if err := Apply(New(), []Op{{Kind: "xyz"}}); err == nil {
		t.Error("expected unknown-op error")
	}
}

func TestApply_OpRequires(t *testing.T) {
	cases := []Op{
		{Kind: "trim", Params: map[string]any{}},
		{Kind: "split", Params: map[string]any{}},
		{Kind: "cut", Params: map[string]any{}},
		{Kind: "transition", Params: map[string]any{}},
		{Kind: "music", Params: map[string]any{}},
		{Kind: "caption", Params: map[string]any{}},
	}
	for _, op := range cases {
		t.Run(op.Kind, func(t *testing.T) {
			if err := Apply(New(), []Op{op}); err == nil {
				t.Errorf("expected validation error for %s", op.Kind)
			}
		})
	}
}

func TestTemplate_ApplyAll(t *testing.T) {
	e := New()
	e.AddClip(Clip{ID: "a"})
	e.AddClip(Clip{ID: "b"})
	tpl := Template{
		Intro: &Effect{Kind: "intro"},
		Outro: &Effect{Kind: "outro"},
		Music: &MusicTrack{Source: "bgm.mp3"},
		Speed: 1.25,
	}
	e.ApplyTemplate(tpl)
	if len(e.Clips[0].Effects) != 1 || e.Clips[0].Effects[0].Kind != "intro" {
		t.Errorf("intro not applied: %+v", e.Clips[0].Effects)
	}
	if e.Clips[1].Effects[0].Kind != "outro" {
		t.Errorf("outro not applied: %+v", e.Clips[1].Effects)
	}
	if e.Music == nil {
		t.Error("music not applied")
	}
	for _, c := range e.Clips {
		if c.Speed != 1.25 {
			t.Errorf("speed not applied: %+v", c)
		}
	}
}
