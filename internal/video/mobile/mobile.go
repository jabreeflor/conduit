// Package mobile records phone-screen demos with native-resolution
// capture, touch visualization, optional device frames, and combined
// desktop+phone composition.
//
// PRD §13.4. Captures via the mobile-control transport (ADB on Android,
// libimobiledevice on iOS); this package owns the platform-independent
// concepts: device descriptors, touch overlays, frame compositor, and
// gesture annotation.
package mobile

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// Platform names a supported mobile OS.
type Platform string

const (
	PlatformIOS     Platform = "ios"
	PlatformAndroid Platform = "android"
)

// DeviceFrame is the bezel/screen geometry of a phone frame.
type DeviceFrame struct {
	Name             string
	Platform         Platform
	OuterW, OuterH   int // frame image size, px
	ScreenX, ScreenY int // top-left of the screen area within the frame
	ScreenW, ScreenH int // screen dimensions in px
}

// Frames is the catalog of supported device frames.
var frames = map[string]DeviceFrame{
	"iphone-15-pro": {
		Name: "iphone-15-pro", Platform: PlatformIOS,
		OuterW: 1290, OuterH: 2628, ScreenX: 30, ScreenY: 32, ScreenW: 1230, ScreenH: 2664,
	},
	"iphone-se": {
		Name: "iphone-se", Platform: PlatformIOS,
		OuterW: 750, OuterH: 1334, ScreenX: 0, ScreenY: 0, ScreenW: 750, ScreenH: 1334,
	},
	"pixel-8-pro": {
		Name: "pixel-8-pro", Platform: PlatformAndroid,
		OuterW: 1344, OuterH: 2992, ScreenX: 24, ScreenY: 24, ScreenW: 1296, ScreenH: 2944,
	},
	"galaxy-s24": {
		Name: "galaxy-s24", Platform: PlatformAndroid,
		OuterW: 1080, OuterH: 2340, ScreenX: 18, ScreenY: 18, ScreenW: 1044, ScreenH: 2304,
	},
}

// FrameNames returns the registered frame names, sorted.
func FrameNames() []string {
	out := make([]string, 0, len(frames))
	for k := range frames {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// LookupFrame returns a frame by name.
func LookupFrame(name string) (DeviceFrame, error) {
	f, ok := frames[name]
	if !ok {
		return DeviceFrame{}, fmt.Errorf("mobile: unknown frame %q", name)
	}
	return f, nil
}

// TouchKind enumerates gesture types.
type TouchKind string

const (
	TouchTap       TouchKind = "tap"
	TouchDoubleTap TouchKind = "double-tap"
	TouchLongPress TouchKind = "long-press"
	TouchSwipe     TouchKind = "swipe"
	TouchPinch     TouchKind = "pinch"
	TouchScroll    TouchKind = "scroll"
)

// Touch is a single timestamped gesture.
type Touch struct {
	Kind       TouchKind
	At         time.Duration
	X, Y       int           // primary point
	EndX, EndY int           // end point for swipe/pinch
	Duration   time.Duration // for long-press / pinch
}

// Recorder owns a mobile session.
type Recorder struct {
	platform  Platform
	frame     *DeviceFrame
	touches   []Touch
	started   bool
	startAt   time.Time
	stoppedAt time.Time
	now       func() time.Time
}

// NewRecorder constructs a Recorder. Pass an empty frameName to skip
// frame compositing. clock can be nil to use time.Now (typically only
// tests pass a custom clock).
func NewRecorder(platform Platform, frameName string, clock func() time.Time) (*Recorder, error) {
	if platform != PlatformIOS && platform != PlatformAndroid {
		return nil, fmt.Errorf("mobile: unsupported platform %q", platform)
	}
	r := &Recorder{platform: platform, now: clock}
	if r.now == nil {
		r.now = time.Now
	}
	if frameName != "" {
		f, err := LookupFrame(frameName)
		if err != nil {
			return nil, err
		}
		if f.Platform != platform {
			return nil, fmt.Errorf("mobile: frame %s is for %s, not %s", frameName, f.Platform, platform)
		}
		r.frame = &f
	}
	return r, nil
}

// Start begins a session.
func (r *Recorder) Start() error {
	if r.started {
		return errors.New("mobile: already started")
	}
	r.started = true
	r.startAt = r.now()
	return nil
}

// Stop ends the session.
func (r *Recorder) Stop() error {
	if !r.started {
		return errors.New("mobile: not started")
	}
	r.stoppedAt = r.now()
	r.started = false
	return nil
}

// Record stores a touch event.
func (r *Recorder) Record(t Touch) error {
	if !r.started {
		return errors.New("mobile: not started")
	}
	if t.At == 0 {
		t.At = r.now().Sub(r.startAt)
	}
	r.touches = append(r.touches, t)
	return nil
}

// Touches returns a copy of the captured touches.
func (r *Recorder) Touches() []Touch {
	out := make([]Touch, len(r.touches))
	copy(out, r.touches)
	return out
}

// Frame returns the active device frame, or nil if none was selected.
func (r *Recorder) Frame() *DeviceFrame { return r.frame }

// --- post-processing -----------------------------------------------------

// Annotation is a rendered gesture annotation for the editor's overlay
// pass — Tango-style "Tap here", "Swipe up", etc.
type Annotation struct {
	At         time.Duration
	Text       string
	X, Y       int
	EndX, EndY int
}

// AnnotateGestures generates one Annotation per touch.
func AnnotateGestures(touches []Touch) []Annotation {
	out := make([]Annotation, 0, len(touches))
	for _, t := range touches {
		a := Annotation{At: t.At, X: t.X, Y: t.Y, EndX: t.EndX, EndY: t.EndY}
		switch t.Kind {
		case TouchTap:
			a.Text = "Tap"
		case TouchDoubleTap:
			a.Text = "Double-tap"
		case TouchLongPress:
			a.Text = fmt.Sprintf("Long-press (%dms)", t.Duration/time.Millisecond)
		case TouchSwipe:
			a.Text = "Swipe"
		case TouchPinch:
			a.Text = "Pinch"
		case TouchScroll:
			a.Text = "Scroll"
		default:
			a.Text = string(t.Kind)
		}
		out = append(out, a)
	}
	return out
}

// LayoutKind describes how desktop and mobile streams are combined.
type LayoutKind string

const (
	LayoutDesktopOnly LayoutKind = "desktop"
	LayoutMobileOnly  LayoutKind = "mobile"
	LayoutSplit       LayoutKind = "split"
	LayoutPiP         LayoutKind = "pip"
)

// Composition is the combined layout description.
type Composition struct {
	Layout   LayoutKind
	DesktopW int
	DesktopH int
	MobileW  int
	MobileH  int
	OutW     int
	OutH     int
}

// PlanComposition computes a combined-output layout.
func PlanComposition(layout LayoutKind, desktopW, desktopH, mobileW, mobileH int) (*Composition, error) {
	if desktopW <= 0 || desktopH <= 0 {
		return nil, errors.New("mobile: invalid desktop size")
	}
	if mobileW <= 0 || mobileH <= 0 {
		return nil, errors.New("mobile: invalid mobile size")
	}
	c := &Composition{
		Layout: layout, DesktopW: desktopW, DesktopH: desktopH,
		MobileW: mobileW, MobileH: mobileH,
	}
	switch layout {
	case LayoutDesktopOnly:
		c.OutW, c.OutH = desktopW, desktopH
	case LayoutMobileOnly:
		c.OutW, c.OutH = mobileW, mobileH
	case LayoutSplit:
		// Side-by-side; mobile scaled to desktop height.
		ratio := float64(mobileW) / float64(mobileH)
		mScaled := int(float64(desktopH) * ratio)
		c.OutW, c.OutH = desktopW+mScaled, desktopH
	case LayoutPiP:
		// Mobile inset at 25% width in bottom-right corner.
		c.OutW, c.OutH = desktopW, desktopH
	default:
		return nil, fmt.Errorf("mobile: unknown layout %q", layout)
	}
	return c, nil
}

// FrameScreenRect returns the X/Y/W/H rectangle inside the frame image
// where the live phone capture should be composited.
func FrameScreenRect(f DeviceFrame) (x, y, w, h int) {
	return f.ScreenX, f.ScreenY, f.ScreenW, f.ScreenH
}
