package recorder

import (
	"strings"
	"testing"
	"time"
)

func clockSeq(times ...time.Time) func() time.Time {
	i := 0
	return func() time.Time {
		t := times[i]
		if i < len(times)-1 {
			i++
		}
		return t
	}
}

func TestNew_ValidatesResolution(t *testing.T) {
	if _, err := New(Settings{}, nil); err == nil {
		t.Error("expected validation error")
	}
	if _, err := New(Settings{Resolution: Res1080p60}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRecorder_StartStopRecord(t *testing.T) {
	t0 := time.Unix(0, 0)
	r, err := New(Settings{Resolution: Res1080p30},
		clockSeq(t0, t0.Add(time.Second), t0.Add(2*time.Second), t0.Add(3*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Record(Event{Kind: EventClick}); err == nil {
		t.Error("expected not-started error")
	}
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	if err := r.Start(); err == nil {
		t.Error("expected already-started error")
	}
	if err := r.Record(Event{Kind: EventClick, X: 1, Y: 2}); err != nil {
		t.Fatal(err)
	}
	if err := r.Stop(); err != nil {
		t.Fatal(err)
	}
	evts := r.Events()
	if len(evts) != 1 || evts[0].At == 0 {
		t.Errorf("event time not stamped: %+v", evts)
	}
	if r.Duration() == 0 {
		t.Error("duration should be > 0")
	}
}

func TestRecorder_StopUnstarted(t *testing.T) {
	r, _ := New(Settings{Resolution: Res1080p30}, nil)
	if err := r.Stop(); err == nil {
		t.Error("expected not-started error")
	}
}

func TestPlanZoom_ClickAnchors(t *testing.T) {
	events := []Event{
		{Kind: EventClick, At: 0, X: 100, Y: 100},
		{Kind: EventClick, At: time.Second, X: 200, Y: 200},
	}
	plan := PlanZoom(events, PlanZoomOpts{})
	if len(plan) != 2 {
		t.Fatalf("plan = %d, want 2", len(plan))
	}
	if plan[0].Scale != DefaultPlanZoomOpts().ClickScale {
		t.Errorf("scale = %v, want %v", plan[0].Scale, DefaultPlanZoomOpts().ClickScale)
	}
}

func TestPlanZoom_IdleReturnsToFit(t *testing.T) {
	events := []Event{
		{Kind: EventClick, At: 0, X: 100, Y: 100},
		{Kind: EventScroll, At: 5 * time.Second},
	}
	plan := PlanZoom(events, PlanZoomOpts{IdleZoomOut: time.Second, ClickScale: 1.5, HoldOnClick: time.Second})
	if len(plan) != 2 || plan[1].Scale != 1.0 {
		t.Errorf("idle zoom-out missing: %+v", plan)
	}
}

func TestCollectClickHighlights(t *testing.T) {
	events := []Event{
		{Kind: EventClick, At: 0, X: 1, Y: 2},
		{Kind: EventScroll, At: time.Second},
		{Kind: EventClick, At: 2 * time.Second, X: 3, Y: 4},
	}
	hl := CollectClickHighlights(events)
	if len(hl) != 2 || hl[1].X != 3 {
		t.Errorf("got %+v", hl)
	}
}

func TestSmoothScroll_Interpolates(t *testing.T) {
	events := []Event{
		{Kind: EventScroll, At: 0, X: 0, Y: 0, Scroll: 0},
		{Kind: EventScroll, At: 100 * time.Millisecond, X: 100, Y: 100, Scroll: 10},
	}
	out := SmoothScroll(events, 20*time.Millisecond)
	if len(out) <= 2 {
		t.Errorf("smoothing did not interpolate: %d events", len(out))
	}
}

func TestSmoothScroll_ZeroStepDefaults(t *testing.T) {
	events := []Event{
		{Kind: EventScroll, At: 0},
		{Kind: EventScroll, At: 200 * time.Millisecond},
	}
	out := SmoothScroll(events, 0)
	if len(out) <= 2 {
		t.Errorf("default step did not interpolate: %d", len(out))
	}
}

func TestExtractSteps(t *testing.T) {
	events := []Event{
		{Kind: EventFocus, At: 0, Window: "Safari"},
		{Kind: EventClick, At: 100 * time.Millisecond, X: 1, Y: 2},
		{Kind: EventClick, At: 200 * time.Millisecond, X: 3, Y: 4}, // grouped with previous
		{Kind: EventClick, At: 5 * time.Second, X: 5, Y: 6},        // new step
		{Kind: EventKeystroke, At: 6 * time.Second, KeyText: "hi"},
	}
	steps := ExtractSteps(events, time.Second)
	if len(steps) != 4 {
		t.Errorf("steps = %d, want 4 (focus + 2 clicks + keystroke): %+v", len(steps), steps)
	}
	if !strings.Contains(steps[0].Description, "Switched to Safari") {
		t.Errorf("first step desc = %q", steps[0].Description)
	}
	if steps[len(steps)-1].Window != "Safari" {
		t.Errorf("window not propagated: %+v", steps)
	}
}
