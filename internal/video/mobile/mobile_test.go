package mobile

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

func TestNewRecorder_PlatformValidation(t *testing.T) {
	if _, err := NewRecorder("symbian", "", nil); err == nil {
		t.Error("expected unsupported-platform error")
	}
	if _, err := NewRecorder(PlatformIOS, "", nil); err != nil {
		t.Fatal(err)
	}
}

func TestNewRecorder_FrameMismatch(t *testing.T) {
	if _, err := NewRecorder(PlatformIOS, "pixel-8-pro", nil); err == nil {
		t.Error("expected platform/frame mismatch error")
	}
	if _, err := NewRecorder(PlatformIOS, "ghost-phone", nil); err == nil {
		t.Error("expected unknown-frame error")
	}
}

func TestRecorder_Lifecycle(t *testing.T) {
	t0 := time.Unix(0, 0)
	r, err := NewRecorder(PlatformAndroid, "pixel-8-pro",
		clockSeq(t0, t0.Add(time.Second), t0.Add(2*time.Second), t0.Add(3*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Record(Touch{Kind: TouchTap}); err == nil {
		t.Error("expected not-started error")
	}
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	if err := r.Start(); err == nil {
		t.Error("expected already-started error")
	}
	if err := r.Record(Touch{Kind: TouchTap, X: 1, Y: 2}); err != nil {
		t.Fatal(err)
	}
	if err := r.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := r.Stop(); err == nil {
		t.Error("expected double-stop error")
	}
	if len(r.Touches()) != 1 {
		t.Errorf("touches = %d", len(r.Touches()))
	}
	if r.Frame() == nil || r.Frame().Name != "pixel-8-pro" {
		t.Errorf("frame not attached: %+v", r.Frame())
	}
}

func TestFrameNamesIncludesAll(t *testing.T) {
	got := FrameNames()
	want := []string{"galaxy-s24", "iphone-15-pro", "iphone-se", "pixel-8-pro"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("FrameNames = %v, want %v", got, want)
	}
}

func TestAnnotateGestures(t *testing.T) {
	touches := []Touch{
		{Kind: TouchTap, X: 1, Y: 2},
		{Kind: TouchLongPress, X: 5, Y: 6, Duration: 1500 * time.Millisecond},
		{Kind: TouchSwipe, X: 0, Y: 0, EndX: 100, EndY: 100},
	}
	a := AnnotateGestures(touches)
	if len(a) != 3 {
		t.Fatalf("annotations = %d", len(a))
	}
	if a[0].Text != "Tap" {
		t.Errorf("tap text = %q", a[0].Text)
	}
	if !strings.Contains(a[1].Text, "1500ms") {
		t.Errorf("long-press text = %q", a[1].Text)
	}
	if a[2].EndX != 100 {
		t.Errorf("swipe end coord lost: %+v", a[2])
	}
}

func TestPlanComposition_AllLayouts(t *testing.T) {
	cases := []struct {
		layout LayoutKind
		wantW  int
	}{
		{LayoutDesktopOnly, 1920},
		{LayoutMobileOnly, 1080},
		{LayoutPiP, 1920},
	}
	for _, tc := range cases {
		t.Run(string(tc.layout), func(t *testing.T) {
			c, err := PlanComposition(tc.layout, 1920, 1080, 1080, 2340)
			if err != nil {
				t.Fatal(err)
			}
			if c.OutW != tc.wantW {
				t.Errorf("OutW = %d, want %d", c.OutW, tc.wantW)
			}
		})
	}
}

func TestPlanComposition_Split(t *testing.T) {
	c, err := PlanComposition(LayoutSplit, 1920, 1080, 1080, 2340)
	if err != nil {
		t.Fatal(err)
	}
	if c.OutW <= 1920 {
		t.Errorf("split OutW = %d, expected > 1920", c.OutW)
	}
}

func TestPlanComposition_Errors(t *testing.T) {
	if _, err := PlanComposition(LayoutSplit, 0, 0, 100, 100); err == nil {
		t.Error("expected invalid-desktop error")
	}
	if _, err := PlanComposition(LayoutSplit, 100, 100, 0, 0); err == nil {
		t.Error("expected invalid-mobile error")
	}
	if _, err := PlanComposition("crazy", 100, 100, 100, 100); err == nil {
		t.Error("expected unknown-layout error")
	}
}

func TestFrameScreenRect(t *testing.T) {
	f, _ := LookupFrame("iphone-15-pro")
	x, y, w, h := FrameScreenRect(f)
	if x != f.ScreenX || y != f.ScreenY || w != f.ScreenW || h != f.ScreenH {
		t.Errorf("FrameScreenRect mismatch")
	}
}
