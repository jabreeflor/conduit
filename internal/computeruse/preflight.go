package computeruse

import (
	"context"

	"github.com/jabreeflor/conduit/internal/computeruse/permissions"
	"github.com/jabreeflor/conduit/internal/contracts"
)

// Preflight runs the macOS permissions gate (PRD §6.8 / issue #38) before a
// computer-use session is allowed to start. The MCP runtime in this package
// (issue #37) calls this just before launching the open-computer-use process.
//
// On non-darwin hosts every probe returns NotApplicable and Preflight is a
// no-op gate, mirroring how Config.IsActive() respects ForceNonDarwin.
//
// The Manager is constructed fresh on every call so callers can swap the
// host-level prober via permissions.NewManager options. Surfaces that want
// stable progress events should hold their own Manager and call its Report
// directly.
func Preflight(ctx context.Context) error {
	return permissions.NewManager().EnsureSessionAllowed(ctx)
}

// PreflightReport returns the full permissions report so a surface can render
// per-permission status with deep-links to System Settings before the user
// kicks off a session. Pair this with Preflight for the gate.
func PreflightReport(ctx context.Context) contracts.ComputerUsePermissionReport {
	return permissions.NewManager().Report(ctx)
}
