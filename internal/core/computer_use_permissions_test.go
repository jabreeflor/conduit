package core

import (
	"context"
	"errors"
	"testing"

	cupermissions "github.com/jabreeflor/conduit/internal/computeruse/permissions"
	"github.com/jabreeflor/conduit/internal/contracts"
)

type stubProber struct {
	statuses map[contracts.ComputerUsePermission]contracts.ComputerUsePermissionState
}

func (s *stubProber) Probe(_ context.Context, p contracts.ComputerUsePermission) contracts.ComputerUsePermissionStatus {
	state := contracts.ComputerUsePermissionStateUnknown
	if v, ok := s.statuses[p]; ok {
		state = v
	}
	return contracts.ComputerUsePermissionStatus{Permission: p, State: state}
}

func TestEngineExposesComputerUsePermissionsManager(t *testing.T) {
	engine := New("test")
	if engine.ComputerUsePermissions() == nil {
		t.Fatal("ComputerUsePermissions() returned nil")
	}
}

func TestEnginePermissionReportLogsMissingPermissions(t *testing.T) {
	engine := New("test")
	prober := &stubProber{statuses: map[contracts.ComputerUsePermission]contracts.ComputerUsePermissionState{
		contracts.ComputerUsePermissionScreenRecording: contracts.ComputerUsePermissionStateMissing,
		contracts.ComputerUsePermissionAccessibility:   contracts.ComputerUsePermissionStateGranted,
	}}
	engine.computerUsePermMgr = cupermissions.NewManager(cupermissions.WithProber(prober))

	report := engine.ComputerUsePermissionReport(context.Background())
	if report.AllGranted {
		t.Fatal("AllGranted should be false when Screen Recording missing")
	}
	log := engine.SessionLog()
	hasMissing := false
	for _, entry := range log {
		if contains(entry.Message, "screen_recording") && contains(entry.Message, "missing") {
			hasMissing = true
		}
	}
	if !hasMissing {
		t.Fatalf("session log missing 'screen_recording missing' entry: %#v", log)
	}
}

func TestEngineEnsureComputerUseAllowedBlocksOnMissing(t *testing.T) {
	engine := New("test")
	prober := &stubProber{statuses: map[contracts.ComputerUsePermission]contracts.ComputerUsePermissionState{
		contracts.ComputerUsePermissionScreenRecording: contracts.ComputerUsePermissionStateMissing,
		contracts.ComputerUsePermissionAccessibility:   contracts.ComputerUsePermissionStateGranted,
	}}
	engine.computerUsePermMgr = cupermissions.NewManager(cupermissions.WithProber(prober))

	err := engine.EnsureComputerUseAllowed(context.Background())
	if err == nil {
		t.Fatal("EnsureComputerUseAllowed = nil, want UngrantedError")
	}
	var ungranted *cupermissions.UngrantedError
	if !errors.As(err, &ungranted) {
		t.Fatalf("err type = %T, want *UngrantedError", err)
	}
	if len(ungranted.Missing) != 1 {
		t.Fatalf("Missing len = %d, want 1", len(ungranted.Missing))
	}
}

func TestEngineEnsureComputerUseAllowedPassesWhenAllGranted(t *testing.T) {
	engine := New("test")
	prober := &stubProber{statuses: map[contracts.ComputerUsePermission]contracts.ComputerUsePermissionState{
		contracts.ComputerUsePermissionScreenRecording: contracts.ComputerUsePermissionStateGranted,
		contracts.ComputerUsePermissionAccessibility:   contracts.ComputerUsePermissionStateGranted,
	}}
	engine.computerUsePermMgr = cupermissions.NewManager(cupermissions.WithProber(prober))

	if err := engine.EnsureComputerUseAllowed(context.Background()); err != nil {
		t.Fatalf("EnsureComputerUseAllowed err = %v, want nil", err)
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
