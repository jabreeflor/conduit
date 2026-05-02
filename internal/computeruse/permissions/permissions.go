// Package permissions implements the macOS first-launch permissions flow for
// computer-use sessions (PRD §6.8). It detects whether Screen Recording and
// Accessibility grants are present, opens the relevant System Settings panes
// for the user, and re-verifies grants before allowing a session to start.
//
// The package is deliberately self-contained so the MCP-based computer-use
// runtime (issue #37) can plug in without touching this code: callers pass a
// permissions.Manager to whatever component starts the session.
package permissions

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// Settings deep-link URLs documented at
// https://developer.apple.com/documentation/devicemanagement/systempreferences
// and confirmed working on macOS 13+ (Ventura, Sonoma, Sequoia). The
// `x-apple.systempreferences:` scheme is the supported way to deep-link into
// the modern System Settings app.
const (
	settingsURLScreenRecording = "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture"
	settingsURLAccessibility   = "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"
)

// SettingsURL returns the deep-link to the System Settings pane that grants
// the supplied permission.
func SettingsURL(p contracts.ComputerUsePermission) string {
	switch p {
	case contracts.ComputerUsePermissionScreenRecording:
		return settingsURLScreenRecording
	case contracts.ComputerUsePermissionAccessibility:
		return settingsURLAccessibility
	default:
		return ""
	}
}

// RequiredPermissions returns the permissions a computer-use session needs.
// The order is stable so reports are deterministic.
func RequiredPermissions() []contracts.ComputerUsePermission {
	return []contracts.ComputerUsePermission{
		contracts.ComputerUsePermissionScreenRecording,
		contracts.ComputerUsePermissionAccessibility,
	}
}

// Prober runs a single permission check. Implementations are platform-specific
// (see permissions_darwin.go and permissions_other.go). Return Unknown rather
// than failing when the underlying probe is unavailable; callers treat Unknown
// as not-granted.
type Prober interface {
	Probe(ctx context.Context, permission contracts.ComputerUsePermission) contracts.ComputerUsePermissionStatus
}

// SettingsOpener launches the System Settings deep-link URL.
type SettingsOpener interface {
	Open(ctx context.Context, url string) error
}

// Manager is the engine-facing API: probe permissions, open the right
// settings pane, and verify grants before allowing a computer-use session.
type Manager struct {
	prober  Prober
	opener  SettingsOpener
	now     func() time.Time
	verifyT time.Duration
}

// Option configures the Manager.
type Option func(*Manager)

// WithProber overrides the platform-default Prober. Useful for tests and for
// surfaces (TUI/GUI) that want to wrap probe results with their own UI events.
func WithProber(p Prober) Option {
	return func(m *Manager) {
		if p != nil {
			m.prober = p
		}
	}
}

// WithOpener overrides the SettingsOpener. The default uses `open <url>` on
// macOS and is a no-op on other platforms.
func WithOpener(o SettingsOpener) Option {
	return func(m *Manager) {
		if o != nil {
			m.opener = o
		}
	}
}

// WithClock injects a deterministic clock for tests.
func WithClock(now func() time.Time) Option {
	return func(m *Manager) {
		if now != nil {
			m.now = now
		}
	}
}

// WithVerifyTimeout bounds VerifyAfterGrant. Defaults to 30s.
func WithVerifyTimeout(d time.Duration) Option {
	return func(m *Manager) {
		if d > 0 {
			m.verifyT = d
		}
	}
}

// NewManager constructs a permissions Manager wired up for the host platform.
// On non-darwin hosts the manager returns NotApplicable for every permission.
func NewManager(opts ...Option) *Manager {
	m := &Manager{
		prober:  defaultProber(),
		opener:  defaultOpener(),
		now:     func() time.Time { return time.Now().UTC() },
		verifyT: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Report runs every required probe and returns the aggregated status.
//
// AllGranted is computed strictly from the Prober results: a permission
// counts as "satisfied" when its state is Granted or NotApplicable. We
// deliberately do not short-circuit on runtime.GOOS so test injection of a
// stub Prober behaves identically across platforms. The default non-darwin
// Prober returns NotApplicable for every permission, which keeps the gate
// open on Linux/Windows hosts where TCC does not exist.
func (m *Manager) Report(ctx context.Context) contracts.ComputerUsePermissionReport {
	required := RequiredPermissions()
	statuses := make([]contracts.ComputerUsePermissionStatus, 0, len(required))
	allGranted := true
	for _, p := range required {
		status := m.prober.Probe(ctx, p)
		if status.SettingsURL == "" {
			status.SettingsURL = SettingsURL(p)
		}
		if status.ProbedAt.IsZero() {
			status.ProbedAt = m.now()
		}
		if status.Permission == "" {
			status.Permission = p
		}
		statuses = append(statuses, status)
		switch status.State {
		case contracts.ComputerUsePermissionStateGranted,
			contracts.ComputerUsePermissionStateNotApplicable:
			// satisfied — leave allGranted as-is
		default:
			allGranted = false
		}
	}
	// Stable ordering by permission name for deterministic output.
	sort.SliceStable(statuses, func(i, j int) bool {
		return string(statuses[i].Permission) < string(statuses[j].Permission)
	})
	return contracts.ComputerUsePermissionReport{
		Platform:    runtime.GOOS,
		Required:    required,
		Statuses:    statuses,
		AllGranted:  allGranted,
		GeneratedAt: m.now(),
	}
}

// Probe runs a single permission probe.
func (m *Manager) Probe(ctx context.Context, p contracts.ComputerUsePermission) contracts.ComputerUsePermissionStatus {
	status := m.prober.Probe(ctx, p)
	if status.SettingsURL == "" {
		status.SettingsURL = SettingsURL(p)
	}
	if status.ProbedAt.IsZero() {
		status.ProbedAt = m.now()
	}
	if status.Permission == "" {
		status.Permission = p
	}
	return status
}

// OpenSettings launches the System Settings pane that grants p. Returns
// ErrUnsupportedPermission for permissions without a known deep-link.
func (m *Manager) OpenSettings(ctx context.Context, p contracts.ComputerUsePermission) error {
	url := SettingsURL(p)
	if url == "" {
		return ErrUnsupportedPermission
	}
	if m.opener == nil {
		return ErrOpenerUnavailable
	}
	return m.opener.Open(ctx, url)
}

// VerifyAfterGrant re-runs the probe for p with a short polling loop. Returns
// granted=true as soon as the probe reports Granted, or false at deadline.
// This is the user-returns-from-Settings checkpoint described in PRD §6.8.
func (m *Manager) VerifyAfterGrant(ctx context.Context, p contracts.ComputerUsePermission) (bool, contracts.ComputerUsePermissionStatus) {
	deadline := m.now().Add(m.verifyT)
	var last contracts.ComputerUsePermissionStatus
	for {
		select {
		case <-ctx.Done():
			return false, last
		default:
		}
		last = m.Probe(ctx, p)
		if last.State == contracts.ComputerUsePermissionStateGranted {
			return true, last
		}
		if last.State == contracts.ComputerUsePermissionStateNotApplicable {
			return true, last
		}
		if !m.now().Before(deadline) {
			return false, last
		}
		// Short sleep avoids hot-spinning while still feeling instant once
		// the user grants in System Settings.
		select {
		case <-ctx.Done():
			return false, last
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// EnsureSessionAllowed is the gate that the computer-use runtime (issue #37)
// must call before starting a session. It returns nil only when every required
// permission is granted (or NotApplicable on non-darwin). On any missing
// permission it returns an *UngrantedError that lists what is missing and the
// settings URL the surface should deep-link the user to.
func (m *Manager) EnsureSessionAllowed(ctx context.Context) error {
	report := m.Report(ctx)
	if report.AllGranted {
		return nil
	}
	missing := make([]contracts.ComputerUsePermissionStatus, 0, len(report.Statuses))
	for _, s := range report.Statuses {
		if s.State == contracts.ComputerUsePermissionStateGranted {
			continue
		}
		if s.State == contracts.ComputerUsePermissionStateNotApplicable {
			continue
		}
		missing = append(missing, s)
	}
	return &UngrantedError{Missing: missing}
}

// Sentinel errors returned by Manager.
var (
	// ErrUnsupportedPermission means SettingsURL has no entry for the given
	// permission.
	ErrUnsupportedPermission = errors.New("permissions: unsupported permission")
	// ErrOpenerUnavailable means the platform has no SettingsOpener wired up.
	ErrOpenerUnavailable = errors.New("permissions: settings opener unavailable")
)

// UngrantedError is returned by Manager.EnsureSessionAllowed when one or more
// required permissions are missing. Surfaces inspect Missing to render the
// "Open System Settings" buttons.
type UngrantedError struct {
	Missing []contracts.ComputerUsePermissionStatus
}

// Error implements error.
func (e *UngrantedError) Error() string {
	if len(e.Missing) == 0 {
		return "computer-use blocked: required permissions are not granted"
	}
	names := make([]string, 0, len(e.Missing))
	for _, s := range e.Missing {
		names = append(names, string(s.Permission))
	}
	return fmt.Sprintf("computer-use blocked: missing macOS permissions: %s", joinNames(names))
}

func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	out := names[0]
	for _, n := range names[1:] {
		out += ", " + n
	}
	return out
}
