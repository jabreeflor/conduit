package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestOpenClawAdapterParsesSOULmd(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-agent")
	os.MkdirAll(skillDir, 0o755)
	path := filepath.Join(skillDir, "SOUL.md")
	body := `## Persona
You are a helpful AI assistant.

## Instructions
Follow these guidelines:
- Be concise
- Be helpful
`
	writeFile(t, path, body)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	skill, err := NewOpenClawAdapter().Parse(path, data, contracts.SkillTierImported)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if skill.Name != "my-agent" {
		t.Errorf("name: got %q want %q", skill.Name, "my-agent")
	}

	// Should have openclaw tag
	if !contains(skill.Tags, "openclaw") {
		t.Errorf("tags should include openclaw, got %v", skill.Tags)
	}

	// Body should contain instructions
	if !strings.Contains(skill.Body, "Follow these guidelines") {
		t.Errorf("body should contain instructions, got %q", skill.Body)
	}
}

func TestOpenClawAdapterExtractsDescription(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-agent")
	os.MkdirAll(skillDir, 0o755)
	path := filepath.Join(skillDir, "SOUL.md")
	body := `## Description
This agent helps with code review.

## Instructions
Review code carefully.
`
	writeFile(t, path, body)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	skill, err := NewOpenClawAdapter().Parse(path, data, contracts.SkillTierImported)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if skill.Description != "This agent helps with code review." {
		t.Errorf("description: got %q", skill.Description)
	}
}

func TestOpenClawAdapterFallsBackToFullBody(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "simple-agent")
	os.MkdirAll(skillDir, 0o755)
	path := filepath.Join(skillDir, "SOUL.md")
	body := `Just plain content without structured sections.
This is the body.
`
	writeFile(t, path, body)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	skill, err := NewOpenClawAdapter().Parse(path, data, contracts.SkillTierImported)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if !strings.Contains(skill.Body, "Just plain content") {
		t.Errorf("body should include full content, got %q", skill.Body)
	}
}

func TestOpenClawAdapterCanHandle(t *testing.T) {
	a := NewOpenClawAdapter()

	if !a.CanHandle("/path/to/SOUL.md") {
		t.Errorf("expected SOUL.md to be handled")
	}
	if !a.CanHandle("/PATH/TO/soul.md") {
		t.Errorf("expected soul.md (lowercase) to be handled")
	}
	if a.CanHandle("/path/to/soul.txt") {
		t.Errorf("expected soul.txt to be rejected")
	}
	if a.CanHandle("/path/to/soul-md") {
		t.Errorf("expected soul-md to be rejected")
	}
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
