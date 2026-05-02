package computeruse

import (
	"context"
	"runtime"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestPreflightReportListsRequiredPermissions(t *testing.T) {
	report := PreflightReport(context.Background())
	if len(report.Statuses) != 2 {
		t.Fatalf("Statuses len = %d, want 2", len(report.Statuses))
	}
	want := map[contracts.ComputerUsePermission]bool{
		contracts.ComputerUsePermissionScreenRecording: false,
		contracts.ComputerUsePermissionAccessibility:   false,
	}
	for _, s := range report.Statuses {
		want[s.Permission] = true
	}
	for perm, seen := range want {
		if !seen {
			t.Errorf("PreflightReport missing required permission %q", perm)
		}
	}
}

// On non-darwin hosts Preflight should be a no-op gate. On darwin it may pass
// or fail depending on the host TCC state, so we only assert the no-op case.
func TestPreflightIsNoopOnNonDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("test only meaningful on non-darwin hosts")
	}
	if err := Preflight(context.Background()); err != nil {
		t.Fatalf("Preflight on %s = %v, want nil", runtime.GOOS, err)
	}
}
