package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jabreeflor/conduit/internal/contracts"
)

func testSetupSnapshot() contracts.FirstRunSetupSnapshot {
	return contracts.FirstRunSetupSnapshot{
		Phase: contracts.FirstRunSetupPhaseWelcome,
		MachineProfile: contracts.MachineProfile{
			Memory: contracts.MemInfo{TotalGB: 16},
			Disk:   contracts.DiskInfo{AvailableGB: 200},
		},
		Recommendation: contracts.LocalModelRecommendation{
			ID:                    "llama3:8b-q8",
			Name:                  "Llama 3 8B",
			MachineClass:          contracts.MachineClassMidRange,
			DownloadSizeGB:        6,
			EstimatedTokensPerSec: 35,
			Recommended:           true,
		},
		Runtime: "ollama",
		Steps: []contracts.FirstRunSetupStep{
			{Name: "Profile machine", Status: contracts.FirstRunSetupStepDone, Detail: "Apple M3, 16GB RAM"},
			{Name: "Choose local runtime", Status: contracts.FirstRunSetupStepPending, Detail: "ollama"},
			{Name: "Install recommended model", Status: contracts.FirstRunSetupStepPending, Detail: "Llama 3 8B"},
		},
		ExternalAPI: []contracts.ExternalAPIOption{
			{Provider: "openai", Label: "Connect OpenAI", EnvVar: "OPENAI_API_KEY"},
			{Provider: "anthropic", Label: "Connect Anthropic", EnvVar: "ANTHROPIC_API_KEY"},
		},
	}
}

func TestConversationContentShowsWelcomeSetupChoices(t *testing.T) {
	m := newModel("claude-haiku-4-5", testSetupSnapshot(), nil)

	got := m.conversationContent()

	for _, want := range []string{"Welcome to Conduit", "Set up local AI", "Llama 3 8B", "External API", "Connect OpenAI"} {
		if !strings.Contains(got, want) {
			t.Fatalf("conversation content missing %q in:\n%s", want, got)
		}
	}
}

func TestContextContentShowsFirstRunProgress(t *testing.T) {
	m := newModel("claude-haiku-4-5", testSetupSnapshot(), nil)

	got := m.contextContent()

	for _, want := range []string{"first run", "Profile machine", "Choose local runtime", "Install recommended model"} {
		if !strings.Contains(got, want) {
			t.Fatalf("context content missing %q in:\n%s", want, got)
		}
	}
}

func TestLocalSetupKeyMarksWelcomeReady(t *testing.T) {
	model := newModel("claude-haiku-4-5", testSetupSnapshot(), func() (contracts.FirstRunSetupSnapshot, error) {
		setup := testSetupSnapshot()
		setup.Phase = contracts.FirstRunSetupPhaseReady
		setup.Ready = true
		for i := range setup.Steps {
			setup.Steps[i].Status = contracts.FirstRunSetupStepDone
		}
		return setup, nil
	})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m := updated.(Model)

	if m.setup.Phase != contracts.FirstRunSetupPhaseReady || !m.setup.Ready {
		t.Fatalf("setup not marked ready: %+v", m.setup)
	}
	if !strings.Contains(m.conversationContent(), "Local AI is ready") {
		t.Fatalf("ready message missing from conversation:\n%s", m.conversationContent())
	}
}
