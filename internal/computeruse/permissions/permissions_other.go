//go:build !darwin

package permissions

import (
	"context"
	"runtime"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// noopProber is the non-darwin Prober. macOS TCC permissions do not exist on
// other platforms, so every probe returns NotApplicable. The Manager treats
// NotApplicable as a satisfied gate, so computer-use sessions are not blocked
// by missing macOS-only grants on Linux/Windows hosts.
type noopProber struct{}

func (noopProber) Probe(ctx context.Context, perm contracts.ComputerUsePermission) contracts.ComputerUsePermissionStatus {
	return contracts.ComputerUsePermissionStatus{
		Permission: perm,
		State:      contracts.ComputerUsePermissionStateNotApplicable,
		Detail:     "macOS-only permission not applicable on " + runtime.GOOS,
		ProbedAt:   time.Now().UTC(),
	}
}

func defaultProber() Prober {
	return noopProber{}
}

// noopOpener satisfies SettingsOpener on non-darwin hosts. No System Settings
// deep-link to launch, so the call is a no-op.
type noopOpener struct{}

func (noopOpener) Open(ctx context.Context, url string) error { return nil }

func defaultOpener() SettingsOpener {
	return noopOpener{}
}
