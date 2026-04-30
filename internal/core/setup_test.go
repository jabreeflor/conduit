package core

import (
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

type recordingInstaller struct {
	called bool
	got    contracts.LocalModelRecommendation
	err    error
}

func (r *recordingInstaller) Install(rec contracts.LocalModelRecommendation) error {
	r.called = true
	r.got = rec
	return r.err
}

func setupWithProfile(t *testing.T, profile contracts.MachineProfile, installer LocalRuntimeInstaller) *firstRunSetup {
	t.Helper()
	cfg := testMachineProfilerConfig(t)
	profiler := NewMachineProfiler(cfg)
	profiler.now = func() time.Time { return profile.ProfiledAt }
	if err := profiler.writeCache(profile); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	return newFirstRunSetup(profiler, installer)
}

func TestFirstRecommendedModelMidRange(t *testing.T) {
	rec := firstRecommendedModel(contracts.MachineProfile{
		Memory: contracts.MemInfo{TotalGB: 32},
	})

	if rec.MachineClass != contracts.MachineClassMidRange {
		t.Fatalf("MachineClass = %q, want mid_range", rec.MachineClass)
	}
	if !rec.Recommended {
		t.Fatal("Recommended = false, want true")
	}
	if rec.Name == "" || rec.DownloadSizeGB == 0 || rec.EstimatedTokensPerSec == 0 {
		t.Fatalf("recommendation missing setup estimates: %+v", rec)
	}
}

func TestFirstRecommendedModelConstrainedUsesExternalFallback(t *testing.T) {
	rec := firstRecommendedModel(contracts.MachineProfile{
		Memory: contracts.MemInfo{TotalGB: 4},
	})

	if rec.ID != "" {
		t.Fatalf("ID = %q, want empty recommendation for constrained machine", rec.ID)
	}
}

func TestFirstRunSetupWelcomeIncludesProfileLocalSetupAndExternalAPI(t *testing.T) {
	profile := contracts.MachineProfile{
		ProfiledAt: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		CPU:        contracts.CPUInfo{Brand: "Apple M3 Max"},
		Memory:     contracts.MemInfo{TotalGB: 64},
		Disk:       contracts.DiskInfo{AvailableGB: 512},
	}
	setup := setupWithProfile(t, profile, nil)

	snapshot, err := setup.Welcome()
	if err != nil {
		t.Fatalf("Welcome() error: %v", err)
	}

	if snapshot.Phase != contracts.FirstRunSetupPhaseWelcome {
		t.Fatalf("Phase = %q, want welcome", snapshot.Phase)
	}
	if snapshot.MachineProfile.CPU.Brand != "Apple M3 Max" {
		t.Fatalf("profile not surfaced: %+v", snapshot.MachineProfile)
	}
	if snapshot.Recommendation.Name == "" || !localSetupRecommended(snapshot) {
		t.Fatalf("missing local recommendation: %+v", snapshot.Recommendation)
	}
	if len(snapshot.ExternalAPI) < 2 {
		t.Fatalf("ExternalAPI = %+v, want OpenAI and Anthropic options", snapshot.ExternalAPI)
	}
	if snapshot.Steps[0].Status != contracts.FirstRunSetupStepDone {
		t.Fatalf("first step = %+v, want profiled machine done", snapshot.Steps[0])
	}
}

func TestSetupLocalAIMarksReadyAfterInstallerCompletes(t *testing.T) {
	installer := &recordingInstaller{}
	setup := setupWithProfile(t, contracts.MachineProfile{
		ProfiledAt: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		Memory:     contracts.MemInfo{TotalGB: 16},
	}, installer)

	snapshot, err := setup.SetupLocalAI()
	if err != nil {
		t.Fatalf("SetupLocalAI() error: %v", err)
	}

	if !installer.called {
		t.Fatal("installer was not called")
	}
	if installer.got.Name == "" {
		t.Fatal("installer did not receive the recommended model")
	}
	if snapshot.Phase != contracts.FirstRunSetupPhaseReady || !snapshot.Ready {
		t.Fatalf("snapshot not ready: %+v", snapshot)
	}
	for _, step := range snapshot.Steps {
		if step.Status != contracts.FirstRunSetupStepDone {
			t.Fatalf("step not done after setup: %+v", step)
		}
	}
}

func TestSetupLocalAIConstrainedMachineKeepsExternalPathVisible(t *testing.T) {
	installer := &recordingInstaller{}
	setup := setupWithProfile(t, contracts.MachineProfile{
		ProfiledAt: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		Memory:     contracts.MemInfo{TotalGB: 4},
	}, installer)

	snapshot, err := setup.SetupLocalAI()
	if err != nil {
		t.Fatalf("SetupLocalAI() error: %v", err)
	}

	if installer.called {
		t.Fatal("installer should not run when local AI is not recommended")
	}
	if snapshot.Phase != contracts.FirstRunSetupPhaseExternal {
		t.Fatalf("Phase = %q, want external_api", snapshot.Phase)
	}
	if len(snapshot.ExternalAPI) == 0 {
		t.Fatal("external API options should remain visible")
	}
}
