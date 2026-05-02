//go:build darwin

package permissions

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// probeTimeout bounds every shell-out so a hung System Events daemon never
// blocks the welcome flow.
const probeTimeout = 4 * time.Second

// darwinProber probes the live macOS host. It avoids CGO/native dependencies
// by shelling out to small AppleScript and Swift snippets:
//
//   - Accessibility: AppleScript "tell application System Events" fails with
//     errAEEventNotPermitted (-1719) when the calling process is not in the
//     Accessibility allowlist. Anything else is treated as granted.
//   - Screen Recording: a one-line Swift snippet calls
//     CGPreflightScreenCaptureAccess() which is the supported way to read
//     the TCC bit without triggering the prompt. Falls back to Unknown when
//     the Swift toolchain is not on PATH (e.g. machines without Xcode CLI
//     tools); Manager treats Unknown as not-granted, so the user is still
//     prompted to grant before a session starts.
type darwinProber struct {
	commandRunner  func(ctx context.Context, name string, args ...string) ([]byte, error)
	swiftAvailable func() bool
	stdinScript    func(ctx context.Context, name string, script string, args ...string) ([]byte, error)
}

func defaultProber() Prober {
	return &darwinProber{
		commandRunner: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		},
		swiftAvailable: func() bool {
			_, err := exec.LookPath("swift")
			return err == nil
		},
		stdinScript: func(ctx context.Context, name, script string, args ...string) ([]byte, error) {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Stdin = strings.NewReader(script)
			return cmd.CombinedOutput()
		},
	}
}

// Probe implements Prober. The returned status is filled in with platform
// metadata so a surface can render the exact probe that ran.
func (p *darwinProber) Probe(ctx context.Context, perm contracts.ComputerUsePermission) contracts.ComputerUsePermissionStatus {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	switch perm {
	case contracts.ComputerUsePermissionAccessibility:
		return p.probeAccessibility(ctx)
	case contracts.ComputerUsePermissionScreenRecording:
		return p.probeScreenRecording(ctx)
	default:
		return contracts.ComputerUsePermissionStatus{
			Permission: perm,
			State:      contracts.ComputerUsePermissionStateUnknown,
			Detail:     "no probe registered for permission",
		}
	}
}

// probeAccessibility runs:
//
//	osascript -e 'tell application "System Events" to count processes'
//
// On a host where the calling process lacks Accessibility, AppleScript exits
// with status 1 and stderr containing "1002" or "-1719". On a host with the
// grant, the command prints a process count and exits 0.
func (p *darwinProber) probeAccessibility(ctx context.Context) contracts.ComputerUsePermissionStatus {
	const script = `tell application "System Events" to count processes`
	out, err := p.commandRunner(ctx, "osascript", "-e", script)
	combined := strings.ToLower(strings.TrimSpace(string(out)))
	probedAt := time.Now().UTC()
	probeCmd := "osascript -e 'tell application \"System Events\" to count processes'"

	if err == nil {
		// AppleScript prints the process count on success.
		return contracts.ComputerUsePermissionStatus{
			Permission:   contracts.ComputerUsePermissionAccessibility,
			State:        contracts.ComputerUsePermissionStateGranted,
			Detail:       "AppleScript probe to System Events succeeded",
			ProbedAt:     probedAt,
			ProbeCommand: probeCmd,
		}
	}

	if isAccessibilityDenied(combined, err) {
		return contracts.ComputerUsePermissionStatus{
			Permission:   contracts.ComputerUsePermissionAccessibility,
			State:        contracts.ComputerUsePermissionStateMissing,
			Detail:       "System Events denied access — grant Accessibility in System Settings",
			ProbedAt:     probedAt,
			ProbeCommand: probeCmd,
		}
	}

	return contracts.ComputerUsePermissionStatus{
		Permission:   contracts.ComputerUsePermissionAccessibility,
		State:        contracts.ComputerUsePermissionStateUnknown,
		Detail:       "Accessibility probe failed: " + truncate(combined, 200),
		ProbedAt:     probedAt,
		ProbeCommand: probeCmd,
	}
}

// probeScreenRecording calls CGPreflightScreenCaptureAccess via a swift -
// one-liner. This API is the documented non-prompting way to read the TCC bit.
// We pipe the script via stdin so we never have to write a temp file.
func (p *darwinProber) probeScreenRecording(ctx context.Context) contracts.ComputerUsePermissionStatus {
	probedAt := time.Now().UTC()
	probeCmd := `swift - <<'SWIFT' (CGPreflightScreenCaptureAccess)`

	if !p.swiftAvailable() {
		return contracts.ComputerUsePermissionStatus{
			Permission:   contracts.ComputerUsePermissionScreenRecording,
			State:        contracts.ComputerUsePermissionStateUnknown,
			Detail:       "swift not on PATH — install Xcode command-line tools to enable Screen Recording probe",
			ProbedAt:     probedAt,
			ProbeCommand: probeCmd,
		}
	}

	const script = `import Foundation
import CoreGraphics
let granted = CGPreflightScreenCaptureAccess()
print(granted ? "granted" : "missing")
`
	out, err := p.stdinScript(ctx, "swift", script, "-")
	if err != nil {
		return contracts.ComputerUsePermissionStatus{
			Permission:   contracts.ComputerUsePermissionScreenRecording,
			State:        contracts.ComputerUsePermissionStateUnknown,
			Detail:       "swift probe failed: " + truncate(strings.TrimSpace(string(out)), 200),
			ProbedAt:     probedAt,
			ProbeCommand: probeCmd,
		}
	}

	switch strings.TrimSpace(string(out)) {
	case "granted":
		return contracts.ComputerUsePermissionStatus{
			Permission:   contracts.ComputerUsePermissionScreenRecording,
			State:        contracts.ComputerUsePermissionStateGranted,
			Detail:       "CGPreflightScreenCaptureAccess returned true",
			ProbedAt:     probedAt,
			ProbeCommand: probeCmd,
		}
	case "missing":
		return contracts.ComputerUsePermissionStatus{
			Permission:   contracts.ComputerUsePermissionScreenRecording,
			State:        contracts.ComputerUsePermissionStateMissing,
			Detail:       "CGPreflightScreenCaptureAccess returned false — grant Screen Recording in System Settings",
			ProbedAt:     probedAt,
			ProbeCommand: probeCmd,
		}
	default:
		return contracts.ComputerUsePermissionStatus{
			Permission:   contracts.ComputerUsePermissionScreenRecording,
			State:        contracts.ComputerUsePermissionStateUnknown,
			Detail:       "swift probe returned unexpected output: " + truncate(string(out), 200),
			ProbedAt:     probedAt,
			ProbeCommand: probeCmd,
		}
	}
}

// isAccessibilityDenied checks for the AppleScript error signature emitted
// when the controlling process is not in the Accessibility allowlist. The
// signal is "errAEEventNotPermitted" (-1719) or the legacy macOS error code
// 1002 surfaced by some macOS versions.
func isAccessibilityDenied(combined string, err error) bool {
	if combined == "" && err == nil {
		return false
	}
	if strings.Contains(combined, "-1719") || strings.Contains(combined, "1002") {
		return true
	}
	if strings.Contains(combined, "not allowed assistive access") ||
		strings.Contains(combined, "not authorized to send apple events") ||
		strings.Contains(combined, "is not allowed to send keystrokes") {
		return true
	}
	// exec.ExitError with status 1 plus empty output on some macOS versions
	// is an Accessibility refusal. Treat it as missing rather than unknown so
	// the user is prompted to grant.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() == 1 && combined == "" {
			return true
		}
	}
	return false
}

// truncate keeps probe Detail strings short enough for UI display.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// macSettingsOpener launches the System Settings deep-link via `open`.
type macSettingsOpener struct{}

func (macSettingsOpener) Open(ctx context.Context, url string) error {
	return exec.CommandContext(ctx, "open", url).Run()
}

func defaultOpener() SettingsOpener {
	return macSettingsOpener{}
}
