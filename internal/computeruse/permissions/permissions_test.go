package permissions

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

type stubProber struct {
	calls    int32
	statuses map[contracts.ComputerUsePermission]contracts.ComputerUsePermissionStatus
	// per-call queue for VerifyAfterGrant tests; key is permission.
	queue map[contracts.ComputerUsePermission][]contracts.ComputerUsePermissionStatus
}

func (s *stubProber) Probe(_ context.Context, p contracts.ComputerUsePermission) contracts.ComputerUsePermissionStatus {
	atomic.AddInt32(&s.calls, 1)
	if q, ok := s.queue[p]; ok && len(q) > 0 {
		next := q[0]
		s.queue[p] = q[1:]
		if next.Permission == "" {
			next.Permission = p
		}
		return next
	}
	if status, ok := s.statuses[p]; ok {
		if status.Permission == "" {
			status.Permission = p
		}
		return status
	}
	return contracts.ComputerUsePermissionStatus{
		Permission: p,
		State:      contracts.ComputerUsePermissionStateUnknown,
	}
}

type stubOpener struct {
	urls []string
	err  error
}

func (s *stubOpener) Open(_ context.Context, url string) error {
	s.urls = append(s.urls, url)
	return s.err
}

func TestSettingsURLReturnsKnownDeepLinks(t *testing.T) {
	cases := map[contracts.ComputerUsePermission]string{
		contracts.ComputerUsePermissionScreenRecording: settingsURLScreenRecording,
		contracts.ComputerUsePermissionAccessibility:   settingsURLAccessibility,
	}
	for perm, want := range cases {
		got := SettingsURL(perm)
		if got != want {
			t.Errorf("SettingsURL(%q) = %q, want %q", perm, got, want)
		}
	}
	if SettingsURL("nope") != "" {
		t.Errorf("SettingsURL on unknown permission should return empty string")
	}
}

func TestRequiredPermissionsCoversScreenRecordingAndAccessibility(t *testing.T) {
	required := RequiredPermissions()
	if len(required) != 2 {
		t.Fatalf("RequiredPermissions length = %d, want 2", len(required))
	}
	have := map[contracts.ComputerUsePermission]bool{}
	for _, p := range required {
		have[p] = true
	}
	if !have[contracts.ComputerUsePermissionScreenRecording] {
		t.Error("RequiredPermissions missing screen_recording")
	}
	if !have[contracts.ComputerUsePermissionAccessibility] {
		t.Error("RequiredPermissions missing accessibility")
	}
}

func TestManagerReportFillsSettingsURLsAndProbedAt(t *testing.T) {
	prober := &stubProber{statuses: map[contracts.ComputerUsePermission]contracts.ComputerUsePermissionStatus{
		contracts.ComputerUsePermissionScreenRecording: {State: contracts.ComputerUsePermissionStateMissing},
		contracts.ComputerUsePermissionAccessibility:   {State: contracts.ComputerUsePermissionStateMissing},
	}}
	clock := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	m := NewManager(WithProber(prober), WithClock(func() time.Time { return clock }))

	report := m.Report(context.Background())
	if len(report.Statuses) != 2 {
		t.Fatalf("report.Statuses len = %d, want 2", len(report.Statuses))
	}
	for _, s := range report.Statuses {
		if s.SettingsURL == "" {
			t.Errorf("status %s has empty SettingsURL", s.Permission)
		}
		if s.ProbedAt.IsZero() {
			t.Errorf("status %s has zero ProbedAt", s.Permission)
		}
	}
	if report.Platform != runtime.GOOS {
		t.Errorf("report.Platform = %q, want %q", report.Platform, runtime.GOOS)
	}
	if !report.GeneratedAt.Equal(clock) {
		t.Errorf("report.GeneratedAt = %v, want %v", report.GeneratedAt, clock)
	}
	if report.AllGranted {
		t.Error("AllGranted should be false when probes report missing")
	}
}

func TestManagerReportAllGrantedWhenEverythingGranted(t *testing.T) {
	prober := &stubProber{statuses: map[contracts.ComputerUsePermission]contracts.ComputerUsePermissionStatus{
		contracts.ComputerUsePermissionScreenRecording: {State: contracts.ComputerUsePermissionStateGranted},
		contracts.ComputerUsePermissionAccessibility:   {State: contracts.ComputerUsePermissionStateGranted},
	}}
	m := NewManager(WithProber(prober))
	report := m.Report(context.Background())
	if !report.AllGranted {
		t.Fatalf("AllGranted = false, want true: %#v", report.Statuses)
	}
}

func TestManagerReportTreatsNotApplicableAsSatisfied(t *testing.T) {
	prober := &stubProber{statuses: map[contracts.ComputerUsePermission]contracts.ComputerUsePermissionStatus{
		contracts.ComputerUsePermissionScreenRecording: {State: contracts.ComputerUsePermissionStateNotApplicable},
		contracts.ComputerUsePermissionAccessibility:   {State: contracts.ComputerUsePermissionStateNotApplicable},
	}}
	m := NewManager(WithProber(prober))
	report := m.Report(context.Background())
	if !report.AllGranted {
		t.Fatalf("AllGranted = false on all-NotApplicable, want true")
	}
}

func TestManagerOpenSettingsForwardsKnownDeepLinks(t *testing.T) {
	opener := &stubOpener{}
	m := NewManager(WithOpener(opener), WithProber(&stubProber{}))
	if err := m.OpenSettings(context.Background(), contracts.ComputerUsePermissionScreenRecording); err != nil {
		t.Fatalf("OpenSettings(screen_recording) error: %v", err)
	}
	if err := m.OpenSettings(context.Background(), contracts.ComputerUsePermissionAccessibility); err != nil {
		t.Fatalf("OpenSettings(accessibility) error: %v", err)
	}
	if got := len(opener.urls); got != 2 {
		t.Fatalf("opener received %d urls, want 2", got)
	}
	if opener.urls[0] != settingsURLScreenRecording {
		t.Errorf("urls[0] = %q, want %q", opener.urls[0], settingsURLScreenRecording)
	}
	if opener.urls[1] != settingsURLAccessibility {
		t.Errorf("urls[1] = %q, want %q", opener.urls[1], settingsURLAccessibility)
	}
}

func TestManagerOpenSettingsRejectsUnknownPermission(t *testing.T) {
	m := NewManager(WithOpener(&stubOpener{}), WithProber(&stubProber{}))
	if err := m.OpenSettings(context.Background(), "no-such"); !errors.Is(err, ErrUnsupportedPermission) {
		t.Fatalf("OpenSettings(unknown) = %v, want ErrUnsupportedPermission", err)
	}
}

func TestManagerEnsureSessionAllowedReturnsUngrantedError(t *testing.T) {
	prober := &stubProber{statuses: map[contracts.ComputerUsePermission]contracts.ComputerUsePermissionStatus{
		contracts.ComputerUsePermissionScreenRecording: {State: contracts.ComputerUsePermissionStateMissing},
		contracts.ComputerUsePermissionAccessibility:   {State: contracts.ComputerUsePermissionStateGranted},
	}}
	m := NewManager(WithProber(prober))
	err := m.EnsureSessionAllowed(context.Background())
	if err == nil {
		t.Fatal("EnsureSessionAllowed = nil, want UngrantedError")
	}
	var ungranted *UngrantedError
	if !errors.As(err, &ungranted) {
		t.Fatalf("EnsureSessionAllowed err type = %T, want *UngrantedError", err)
	}
	if len(ungranted.Missing) != 1 {
		t.Fatalf("Missing len = %d, want 1: %#v", len(ungranted.Missing), ungranted.Missing)
	}
	if ungranted.Missing[0].Permission != contracts.ComputerUsePermissionScreenRecording {
		t.Errorf("Missing[0].Permission = %q, want screen_recording", ungranted.Missing[0].Permission)
	}
	if ungranted.Missing[0].SettingsURL != settingsURLScreenRecording {
		t.Errorf("Missing[0].SettingsURL = %q, want %q", ungranted.Missing[0].SettingsURL, settingsURLScreenRecording)
	}
}

func TestManagerEnsureSessionAllowedNilWhenAllGranted(t *testing.T) {
	prober := &stubProber{statuses: map[contracts.ComputerUsePermission]contracts.ComputerUsePermissionStatus{
		contracts.ComputerUsePermissionScreenRecording: {State: contracts.ComputerUsePermissionStateGranted},
		contracts.ComputerUsePermissionAccessibility:   {State: contracts.ComputerUsePermissionStateGranted},
	}}
	m := NewManager(WithProber(prober))
	if err := m.EnsureSessionAllowed(context.Background()); err != nil {
		t.Fatalf("EnsureSessionAllowed = %v, want nil", err)
	}
}

func TestManagerVerifyAfterGrantSucceedsWhenProbeFlipsToGranted(t *testing.T) {
	prober := &stubProber{queue: map[contracts.ComputerUsePermission][]contracts.ComputerUsePermissionStatus{
		contracts.ComputerUsePermissionAccessibility: {
			{State: contracts.ComputerUsePermissionStateMissing},
			{State: contracts.ComputerUsePermissionStateMissing},
			{State: contracts.ComputerUsePermissionStateGranted},
		},
	}}
	// Drive the now() clock forward in steps so the deadline never fires
	// before the queued sequence is exhausted.
	base := time.Unix(0, 0).UTC()
	var step int64
	now := func() time.Time {
		t := base.Add(time.Duration(atomic.LoadInt64(&step)) * 10 * time.Millisecond)
		atomic.AddInt64(&step, 1)
		return t
	}
	m := NewManager(WithProber(prober), WithClock(now), WithVerifyTimeout(10*time.Second))

	ok, status := m.VerifyAfterGrant(context.Background(), contracts.ComputerUsePermissionAccessibility)
	if !ok {
		t.Fatalf("VerifyAfterGrant ok = false, status = %#v", status)
	}
	if status.State != contracts.ComputerUsePermissionStateGranted {
		t.Errorf("status.State = %q, want granted", status.State)
	}
	if atomic.LoadInt32(&prober.calls) < 3 {
		t.Errorf("expected at least 3 probes before grant flipped, got %d", prober.calls)
	}
}

func TestManagerVerifyAfterGrantHonorsDeadline(t *testing.T) {
	prober := &stubProber{statuses: map[contracts.ComputerUsePermission]contracts.ComputerUsePermissionStatus{
		contracts.ComputerUsePermissionAccessibility: {State: contracts.ComputerUsePermissionStateMissing},
	}}
	// Clock that jumps past the verify timeout immediately, so the loop
	// returns without sleeping.
	base := time.Unix(0, 0).UTC()
	var step int64
	now := func() time.Time {
		t := base.Add(time.Duration(atomic.LoadInt64(&step)) * time.Hour)
		atomic.AddInt64(&step, 1)
		return t
	}
	m := NewManager(WithProber(prober), WithClock(now), WithVerifyTimeout(1*time.Millisecond))

	ok, status := m.VerifyAfterGrant(context.Background(), contracts.ComputerUsePermissionAccessibility)
	if ok {
		t.Fatalf("VerifyAfterGrant ok = true on missing permission past deadline")
	}
	if status.State != contracts.ComputerUsePermissionStateMissing {
		t.Errorf("status.State = %q, want missing", status.State)
	}
}

func TestManagerVerifyAfterGrantTreatsNotApplicableAsSuccess(t *testing.T) {
	prober := &stubProber{statuses: map[contracts.ComputerUsePermission]contracts.ComputerUsePermissionStatus{
		contracts.ComputerUsePermissionAccessibility: {State: contracts.ComputerUsePermissionStateNotApplicable},
	}}
	m := NewManager(WithProber(prober))
	ok, status := m.VerifyAfterGrant(context.Background(), contracts.ComputerUsePermissionAccessibility)
	if !ok {
		t.Fatalf("VerifyAfterGrant ok = false on NotApplicable, status=%v", status)
	}
}

func TestManagerVerifyAfterGrantHonorsContextCancel(t *testing.T) {
	prober := &stubProber{statuses: map[contracts.ComputerUsePermission]contracts.ComputerUsePermissionStatus{
		contracts.ComputerUsePermissionAccessibility: {State: contracts.ComputerUsePermissionStateMissing},
	}}
	m := NewManager(WithProber(prober), WithVerifyTimeout(10*time.Second))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ok, _ := m.VerifyAfterGrant(ctx, contracts.ComputerUsePermissionAccessibility)
	if ok {
		t.Fatal("VerifyAfterGrant ok = true on cancelled context")
	}
}

func TestUngrantedErrorMessageNamesEachMissingPermission(t *testing.T) {
	err := &UngrantedError{Missing: []contracts.ComputerUsePermissionStatus{
		{Permission: contracts.ComputerUsePermissionScreenRecording},
		{Permission: contracts.ComputerUsePermissionAccessibility},
	}}
	got := err.Error()
	if got == "" {
		t.Fatal("Error() returned empty string")
	}
	if !contains(got, "screen_recording") || !contains(got, "accessibility") {
		t.Errorf("Error() = %q, want it to mention both missing permissions", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
