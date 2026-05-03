package pluginapi

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMemory_RecordLifecycle(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	id, err := m.StartRecording(ctx, RecordOptions{Width: 1920, Height: 1080, FPS: 60})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(id), "session-") {
		t.Errorf("session id = %q", id)
	}
	started, stopped, _, _, _, _, _, _, ok := m.Inspect(id)
	if !ok || !started || stopped {
		t.Errorf("inspect = started:%v stopped:%v ok:%v", started, stopped, ok)
	}
	if err := m.StopRecording(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := m.StopRecording(ctx, id); err == nil {
		t.Error("expected double-stop error")
	}
}

func TestMemory_UnknownSession(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	if err := m.StopRecording(ctx, "nope"); err == nil {
		t.Error("expected unknown-session error")
	}
}

func TestMemory_Screenshot(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	id, _ := m.StartRecording(ctx, RecordOptions{})
	s, err := m.Screenshot(ctx, id, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if s.Session != id || s.At != time.Second {
		t.Errorf("screenshot meta wrong: %+v", s)
	}
	if len(s.PNG) < 8 || string(s.PNG[1:4]) != "PNG" {
		t.Errorf("png header missing: %v", s.PNG[:8])
	}
}

func TestMemory_EditValidation(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	id, _ := m.StartRecording(ctx, RecordOptions{})
	if err := m.Edit(ctx, EditRequest{Session: id, Intent: ""}); err == nil {
		t.Error("expected empty-intent error")
	}
	if err := m.Edit(ctx, EditRequest{Session: id, Intent: "remove the ums"}); err != nil {
		t.Fatal(err)
	}
	_, _, edits, _, _, _, _, _, _ := m.Inspect(id)
	if edits != 1 {
		t.Errorf("edits = %d", edits)
	}
}

func TestMemory_ExportValidation(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	id, _ := m.StartRecording(ctx, RecordOptions{})
	cases := []ExportRequest{
		{Session: id},
		{Session: id, Preset: "youtube"},
		{Session: id, OutPath: "out.mp4"},
	}
	for i, r := range cases {
		if err := m.Export(ctx, r); err == nil {
			t.Errorf("case %d: expected validation error", i)
		}
	}
	if err := m.Export(ctx, ExportRequest{Session: id, Preset: "youtube", OutPath: "out.mp4"}); err != nil {
		t.Fatal(err)
	}
}

func TestMemory_Caption(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	id, _ := m.StartRecording(ctx, RecordOptions{})
	out, err := m.Caption(ctx, CaptionRequest{Session: id, Format: "vtt"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), "WEBVTT") {
		t.Errorf("caption header missing")
	}
}

func TestMemory_Narrate(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	id, _ := m.StartRecording(ctx, RecordOptions{})
	if err := m.Narrate(ctx, NarrateRequest{Session: id, Voice: "default"}); err != nil {
		t.Fatal(err)
	}
}

func TestMemory_AnnotateValidation(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	id, _ := m.StartRecording(ctx, RecordOptions{})
	if err := m.Annotate(ctx, AnnotateRequest{Session: id}); err == nil {
		t.Error("expected text-required error")
	}
	if err := m.Annotate(ctx, AnnotateRequest{Session: id, Text: "click here", X: 1, Y: 2}); err != nil {
		t.Fatal(err)
	}
}

func TestMemory_HighlightValidation(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	id, _ := m.StartRecording(ctx, RecordOptions{})
	for _, bad := range []float64{-0.1, 1.5} {
		if err := m.Highlight(ctx, HighlightRequest{Session: id, Score: bad}); err == nil {
			t.Errorf("expected score-range error for %v", bad)
		}
	}
	if err := m.Highlight(ctx, HighlightRequest{Session: id, Score: 0.8}); err != nil {
		t.Fatal(err)
	}
}

// Verify Memory satisfies Service.
var _ Service = (*Memory)(nil)
