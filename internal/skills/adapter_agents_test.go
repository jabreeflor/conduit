package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestAgentsMDAdapterParsesFile(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "code-review-agent")
	os.MkdirAll(agentDir, 0o755)
	path := filepath.Join(agentDir, "AGENTS.md")
	body := "Review code for quality.\nCheck for:\n- Performance\n- Security\n- Style\n"
	writeFile(t, path, body)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	skill, err := NewAgentsMDAdapter().Parse(path, data, contracts.SkillTierImported)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if skill.Name != "code-review-agent" {
		t.Errorf("name should be derived from parent dir, got %q", skill.Name)
	}

	if !strings.Contains(skill.Body, "Review code for quality.") {
		t.Errorf("body should contain full file content, got %q", skill.Body)
	}

	if !contains(skill.Tags, "agents") {
		t.Errorf("tags should include 'agents', got %v", skill.Tags)
	}
}

func TestAgentsMDAdapterDerivesNameFromParent(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/home/user/my-agent/AGENTS.md", "my-agent"},
		{"/projects/research/AGENTS.md", "research"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			dir := t.TempDir()
			fullPath := filepath.Join(dir, tt.path)
			os.MkdirAll(filepath.Dir(fullPath), 0o755)
			writeFile(t, fullPath, "Instructions")

			data, err := os.ReadFile(fullPath)
			if err != nil {
				t.Fatalf("read: %v", err)
			}

			skill, err := NewAgentsMDAdapter().Parse(fullPath, data, contracts.SkillTierImported)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			if skill.Name != tt.expected {
				t.Errorf("got %q, want %q", skill.Name, tt.expected)
			}
		})
	}

	// Root-level AGENTS.md falls back to "agents"
	t.Run("root-level", func(t *testing.T) {
		skill, err := NewAgentsMDAdapter().Parse("/AGENTS.md", []byte("Instructions"), contracts.SkillTierImported)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if skill.Name != "agents" {
			t.Errorf("got %q, want %q", skill.Name, "agents")
		}
	})
}

func TestAgentsMDAdapterCanHandle(t *testing.T) {
	a := NewAgentsMDAdapter()

	if !a.CanHandle("/path/to/AGENTS.md") {
		t.Errorf("expected AGENTS.md to be handled")
	}

	if !a.CanHandle("/AGENTS.md") {
		t.Errorf("expected root AGENTS.md to be handled")
	}

	if !a.CanHandle("/agents.md") {
		t.Errorf("expected agents.md (case-insensitive) to be handled")
	}

	if a.CanHandle("/path/to/AGENTS.txt") {
		t.Errorf("expected AGENTS.txt to be rejected")
	}

	if a.CanHandle("/path/to/agents") {
		t.Errorf("expected agents (no extension) to be rejected")
	}
}

func TestAgentsMDAdapterTier(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "test-agent")
	os.MkdirAll(agentDir, 0o755)
	path := filepath.Join(agentDir, "AGENTS.md")
	writeFile(t, path, "Instructions")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	skill, err := NewAgentsMDAdapter().Parse(path, data, contracts.SkillTierImported)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if skill.Tier != contracts.SkillTierImported {
		t.Errorf("tier: got %q want %q", skill.Tier, contracts.SkillTierImported)
	}
}
