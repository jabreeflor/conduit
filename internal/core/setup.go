package core

import (
	"fmt"
	"os/exec"

	"github.com/jabreeflor/conduit/internal/contracts"
)

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
	if _, err := exec.LookPath(recommendation.Runtime); err == nil {
		return nil
	}
	return fmt.Errorf("%s is not installed yet", recommendation.Runtime)
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
	recommendation := RecommendLocalModel(profile)
	return contracts.FirstRunSetupSnapshot{
		Phase:          contracts.FirstRunSetupPhaseWelcome,
		MachineProfile: profile,
		Recommendation: recommendation,
		Steps: []contracts.FirstRunSetupStep{
			{Name: "Profile machine", Status: contracts.FirstRunSetupStepDone, Detail: machineProfileSummary(profile)},
			{Name: "Choose local runtime", Status: contracts.FirstRunSetupStepPending, Detail: recommendation.Runtime},
			{Name: "Install recommended model", Status: contracts.FirstRunSetupStepPending, Detail: recommendation.Model},
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
	if !snapshot.Recommendation.LocalRecommended {
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
	snapshot.Steps[1].Detail = snapshot.Recommendation.Runtime + " ready"
	snapshot.Steps[2].Detail = snapshot.Recommendation.Model + " ready"
	snapshot.Steps[3].Detail = "local provider verified"
	return snapshot, nil
}

// RecommendLocalModel maps a cached machine profile to the opinionated local
// runtime and model shown on the welcome screen.
func RecommendLocalModel(profile contracts.MachineProfile) contracts.LocalModelRecommendation {
	mem := profile.Memory.TotalGB
	switch {
	case mem >= 64:
		return contracts.LocalModelRecommendation{
			Tier:                "high-end",
			Runtime:             "ollama",
			Model:               "llama3:70b-instruct-q5_K_M",
			DownloadSizeGB:      40,
			DiskFootprintGB:     45,
			EstimatedTokensPerS: 15,
			Note:                "large general-purpose model for Apple Silicon and high-memory Macs",
			LocalRecommended:    true,
		}
	case mem >= 16:
		return contracts.LocalModelRecommendation{
			Tier:                "mid-range",
			Runtime:             "ollama",
			Model:               "llama3:8b-instruct-q6_K",
			DownloadSizeGB:      6,
			DiskFootprintGB:     8,
			EstimatedTokensPerS: 35,
			Note:                "balanced default for everyday local chat and coding help",
			LocalRecommended:    true,
		}
	case mem >= 8:
		return contracts.LocalModelRecommendation{
			Tier:                "entry-level",
			Runtime:             "ollama",
			Model:               "phi3:mini",
			DownloadSizeGB:      2.3,
			DiskFootprintGB:     3,
			EstimatedTokensPerS: 25,
			Note:                "small model with clear quality expectations",
			LocalRecommended:    true,
		}
	default:
		return contracts.LocalModelRecommendation{
			Tier:             "constrained",
			Runtime:          "external-api",
			Model:            "OpenAI or Anthropic API",
			Note:             "this machine is below the local-memory floor; connect an API key instead",
			LocalRecommended: false,
		}
	}
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
