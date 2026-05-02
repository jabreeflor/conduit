//go:build darwin

package permissions

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// fakeExitError is a stand-in for *exec.ExitError so we can drive the
// isAccessibilityDenied branch that inspects exit code + empty output.
type fakeExitError struct {
	code int
}

func (e *fakeExitError) Error() string { return "exit status fake" }

func TestIsAccessibilityDeniedFromAppleScriptErrorCodes(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want bool
	}{
		{"errAEEventNotPermitted -1719", "execution error: System Events got an error: -1719", true},
		{"legacy 1002 code", "(error 1002)", true},
		{"assistive access string", "is not allowed assistive access", true},
		{"keystrokes string", "is not allowed to send keystrokes", true},
		{"unrelated error", "syntax error: missing close brace", false},
		{"empty output", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isAccessibilityDenied(tc.out, errors.New("any error"))
			if got != tc.want {
				t.Errorf("isAccessibilityDenied(%q) = %v, want %v", tc.out, got, tc.want)
			}
		})
	}
}

func TestProbeScreenRecordingReportsUnknownWhenSwiftMissing(t *testing.T) {
	p := &darwinProber{
		commandRunner:  func(ctx context.Context, name string, args ...string) ([]byte, error) { return nil, nil },
		swiftAvailable: func() bool { return false },
		stdinScript:    func(ctx context.Context, name, script string, args ...string) ([]byte, error) { return nil, nil },
	}
	status := p.Probe(context.Background(), contracts.ComputerUsePermissionScreenRecording)
	if status.State != contracts.ComputerUsePermissionStateUnknown {
		t.Fatalf("State = %q, want unknown when swift is missing", status.State)
	}
	if status.Detail == "" {
		t.Error("Detail should explain why the probe could not run")
	}
}

func TestProbeScreenRecordingReportsGrantedWhenSwiftPrintsGranted(t *testing.T) {
	p := &darwinProber{
		commandRunner:  func(ctx context.Context, name string, args ...string) ([]byte, error) { return nil, nil },
		swiftAvailable: func() bool { return true },
		stdinScript: func(ctx context.Context, name, script string, args ...string) ([]byte, error) {
			return []byte("granted\n"), nil
		},
	}
	status := p.Probe(context.Background(), contracts.ComputerUsePermissionScreenRecording)
	if status.State != contracts.ComputerUsePermissionStateGranted {
		t.Fatalf("State = %q, want granted", status.State)
	}
}

func TestProbeScreenRecordingReportsMissingWhenSwiftPrintsMissing(t *testing.T) {
	p := &darwinProber{
		commandRunner:  func(ctx context.Context, name string, args ...string) ([]byte, error) { return nil, nil },
		swiftAvailable: func() bool { return true },
		stdinScript: func(ctx context.Context, name, script string, args ...string) ([]byte, error) {
			return []byte("missing\n"), nil
		},
	}
	status := p.Probe(context.Background(), contracts.ComputerUsePermissionScreenRecording)
	if status.State != contracts.ComputerUsePermissionStateMissing {
		t.Fatalf("State = %q, want missing", status.State)
	}
}

func TestProbeScreenRecordingReportsUnknownOnSwiftError(t *testing.T) {
	p := &darwinProber{
		commandRunner:  func(ctx context.Context, name string, args ...string) ([]byte, error) { return nil, nil },
		swiftAvailable: func() bool { return true },
		stdinScript: func(ctx context.Context, name, script string, args ...string) ([]byte, error) {
			return []byte("compilation failed"), errors.New("swift exit 1")
		},
	}
	status := p.Probe(context.Background(), contracts.ComputerUsePermissionScreenRecording)
	if status.State != contracts.ComputerUsePermissionStateUnknown {
		t.Fatalf("State = %q, want unknown on swift error", status.State)
	}
}

func TestProbeAccessibilityReportsGrantedWhenAppleScriptSucceeds(t *testing.T) {
	p := &darwinProber{
		commandRunner: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return []byte("42\n"), nil
		},
		swiftAvailable: func() bool { return true },
	}
	status := p.Probe(context.Background(), contracts.ComputerUsePermissionAccessibility)
	if status.State != contracts.ComputerUsePermissionStateGranted {
		t.Fatalf("State = %q, want granted", status.State)
	}
}

func TestProbeAccessibilityReportsMissingOnDeniedError(t *testing.T) {
	p := &darwinProber{
		commandRunner: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return []byte("execution error: System Events got an error: -1719"), errors.New("exit 1")
		},
		swiftAvailable: func() bool { return true },
	}
	status := p.Probe(context.Background(), contracts.ComputerUsePermissionAccessibility)
	if status.State != contracts.ComputerUsePermissionStateMissing {
		t.Fatalf("State = %q, want missing", status.State)
	}
}

func TestProbeUnknownPermissionReturnsUnknown(t *testing.T) {
	p := &darwinProber{
		commandRunner:  func(ctx context.Context, name string, args ...string) ([]byte, error) { return nil, nil },
		swiftAvailable: func() bool { return true },
	}
	status := p.Probe(context.Background(), contracts.ComputerUsePermission("imaginary"))
	if status.State != contracts.ComputerUsePermissionStateUnknown {
		t.Fatalf("State = %q, want unknown", status.State)
	}
}

// Sanity check that exec.LookPath integration in defaultProber compiles. We
// don't call defaultProber().Probe() because that would shell out on a real
// machine, but we do confirm it returns a non-nil Prober.
func TestDefaultProberIsNonNil(t *testing.T) {
	if defaultProber() == nil {
		t.Fatal("defaultProber() returned nil")
	}
	if defaultOpener() == nil {
		t.Fatal("defaultOpener() returned nil")
	}
	// touch exec.ExitError to prove the import is needed.
	_ = (*exec.ExitError)(nil)
}
