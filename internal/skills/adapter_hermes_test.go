package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestHermesAdapterParsesFrontmatter(t *testing.T) {
	dir := t.TempDir()
	hermesDir := filepath.Join(dir, "hermes-skills")
	os.MkdirAll(hermesDir, 0o755)
	path := filepath.Join(hermesDir, "example.md")
	body := "---\nname: hermes-example\ndescription: A Hermes skill\nplatforms:\n  - claude\n  - openai\ntags:\n  - hermes\n  - agent\n---\nAgent instructions here.\n"
	writeFile(t, path, body)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	skill, err := NewHermesAdapter().Parse(path, data, contracts.SkillTierImported)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if skill.Name != "hermes-example" {
		t.Errorf("name: got %q want %q", skill.Name, "hermes-example")
	}
	if skill.Description != "A Hermes skill" {
		t.Errorf("description: got %q", skill.Description)
	}
	if got, want := skill.Tags, []string{"hermes", "agent"}; !equalStrings(got, want) {
		t.Errorf("tags: got %v want %v", got, want)
	}
}

func TestHermesAdapterRejectsWithoutPlatforms(t *testing.T) {
	dir := t.TempDir()
	hermesDir := filepath.Join(dir, "hermes-skills")
	os.MkdirAll(hermesDir, 0o755)
	path := filepath.Join(hermesDir, "nothermes.md")
	body := "---\nname: nothermes\ndescription: Missing platforms\n---\nBody\n"
	writeFile(t, path, body)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	_, err = NewHermesAdapter().Parse(path, data, contracts.SkillTierImported)
	if err == nil {
		t.Fatalf("expected error when platforms field is missing")
	}
}

func TestHermesAdapterCanHandle(t *testing.T) {
	a := NewHermesAdapter()

	// Should handle .md files in hermes paths
	if !a.CanHandle("/home/user/.hermes/skills/example.md") {
		t.Errorf("expected .hermes/skills path to be handled")
	}
	if !a.CanHandle("/path/to/hermes/skill.md") {
		t.Errorf("expected 'hermes' in path to be handled")
	}

	// Should not handle other paths
	if a.CanHandle("/path/to/regular.md") {
		t.Errorf("expected non-hermes .md to be rejected")
	}
	if a.CanHandle("/path/to/hermes.txt") {
		t.Errorf("expected .txt to be rejected")
	}
}

func TestHermesAdapterFallsBackToFilename(t *testing.T) {
	dir := t.TempDir()
	hermesDir := filepath.Join(dir, "hermes")
	os.MkdirAll(hermesDir, 0o755)
	path := filepath.Join(hermesDir, "my-skill.md")
	body := "---\nplatforms:\n  - claude\n---\nInstructions.\n"
	writeFile(t, path, body)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	skill, err := NewHermesAdapter().Parse(path, data, contracts.SkillTierImported)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if skill.Name != "my-skill" {
		t.Errorf("expected filename fallback, got %q", skill.Name)
	}
}
