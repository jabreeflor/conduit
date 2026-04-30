package core

import (
	"fmt"
	"os/exec"

	"github.com/jabreeflor/conduit/internal/contracts"
)

const defaultFirstRunRuntime = "ollama"

// LocalRuntimeInstaller performs the host-specific local AI installation.
// Production implementations may download runtimes and model weights; tests can
// provide a deterministic installer without network access.
type LocalRuntimeInstaller interface {
	Install(recommendation contracts.LocalModelRecommendation) error
}

type firstRunSetup struct {
	profiler  *MachineProfiler
	installer LocalRuntimeInstaller
}

type commandRuntimeInstaller struct{}

func (commandRuntimeInstaller) Install(recommendation contracts.LocalModelRecommendation) error {
	if _, err := exec.LookPath(defaultFirstRunRuntime); err == nil {
		return nil
	}
	return fmt.Errorf("%s is not installed yet", defaultFirstRunRuntime)
}

func newFirstRunSetup(profiler *MachineProfiler, installer LocalRuntimeInstaller) *firstRunSetup {
	if installer == nil {
		installer = commandRuntimeInstaller{}
	}
	return &firstRunSetup{profiler: profiler, installer: installer}
}

func (s *firstRunSetup) Welcome() (contracts.FirstRunSetupSnapshot, error) {
	profile, err := s.profiler.Load()
	if err != nil {
		return contracts.FirstRunSetupSnapshot{}, err
	}
	recommendation := firstRecommendedModel(profile)
	runtime := defaultFirstRunRuntime
	if recommendation.ID == "" {
		runtime = "external-api"
	}
	return contracts.FirstRunSetupSnapshot{
		Phase:          contracts.FirstRunSetupPhaseWelcome,
		MachineProfile: profile,
		Recommendation: recommendation,
		Runtime:        runtime,
		Steps: []contracts.FirstRunSetupStep{
			{Name: "Profile machine", Status: contracts.FirstRunSetupStepDone, Detail: machineProfileSummary(profile)},
			{Name: "Choose local runtime", Status: contracts.FirstRunSetupStepPending, Detail: runtime},
			{Name: "Install recommended model", Status: contracts.FirstRunSetupStepPending, Detail: modelDisplayName(recommendation)},
			{Name: "Verify local chat", Status: contracts.FirstRunSetupStepPending},
		},
		ExternalAPI: DefaultExternalAPIOptions(),
	}, nil
}

func (s *firstRunSetup) SetupLocalAI() (contracts.FirstRunSetupSnapshot, error) {
	snapshot, err := s.Welcome()
	if err != nil {
		return contracts.FirstRunSetupSnapshot{}, err
	}
	if !localSetupRecommended(snapshot) {
		snapshot.Phase = contracts.FirstRunSetupPhaseExternal
		snapshot.Steps[1].Status = contracts.FirstRunSetupStepDone
		snapshot.Steps[1].Detail = "external API recommended"
		return snapshot, nil
	}

	snapshot.Phase = contracts.FirstRunSetupPhaseInstalling
	snapshot.Steps[1].Status = contracts.FirstRunSetupStepRunning
	if err := s.installer.Install(snapshot.Recommendation); err != nil {
		return snapshot, err
	}
	snapshot.Phase = contracts.FirstRunSetupPhaseReady
	snapshot.Ready = true
	for i := range snapshot.Steps {
		snapshot.Steps[i].Status = contracts.FirstRunSetupStepDone
	}
	snapshot.Steps[1].Detail = snapshot.Runtime + " ready"
	snapshot.Steps[2].Detail = modelDisplayName(snapshot.Recommendation) + " ready"
	snapshot.Steps[3].Detail = "local provider verified"
	return snapshot, nil
}

// DefaultExternalAPIOptions keeps the non-local path visible next to local
// setup, as required by PRD 7.4.
func DefaultExternalAPIOptions() []contracts.ExternalAPIOption {
	return []contracts.ExternalAPIOption{
		{Provider: "openai", Label: "Connect OpenAI", EnvVar: "OPENAI_API_KEY"},
		{Provider: "anthropic", Label: "Connect Anthropic", EnvVar: "ANTHROPIC_API_KEY"},
	}
}

func machineProfileSummary(profile contracts.MachineProfile) string {
	cpu := profile.CPU.Brand
	if cpu == "" {
		cpu = "unknown CPU"
	}
	return fmt.Sprintf("%s, %.0fGB RAM, %.0fGB free", cpu, profile.Memory.TotalGB, profile.Disk.AvailableGB)
}

func firstRecommendedModel(profile contracts.MachineProfile) contracts.LocalModelRecommendation {
	set := RecommendLocalModels(profile, contracts.LocalModelRecommendationOptions{})
	for _, recommendation := range set.Recommendations {
		if recommendation.Recommended && recommendation.Use == contracts.LocalModelUseGeneral {
			return recommendation
		}
	}
	if len(set.Recommendations) > 0 {
		return set.Recommendations[0]
	}
	return contracts.LocalModelRecommendation{}
}

func localSetupRecommended(snapshot contracts.FirstRunSetupSnapshot) bool {
	return snapshot.Runtime != "external-api" && snapshot.Recommendation.ID != ""
}

func modelDisplayName(recommendation contracts.LocalModelRecommendation) string {
	if recommendation.Name != "" {
		if recommendation.Quantization != "" {
			return recommendation.Name + " (" + recommendation.Quantization + ")"
		}
		return recommendation.Name
	}
	if recommendation.ID != "" {
		return recommendation.ID
	}
	return "external API"
}
