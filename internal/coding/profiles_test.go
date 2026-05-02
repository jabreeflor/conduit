package coding_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jabreeflor/conduit/internal/coding"
)

// writeFile is a test helper that writes content to path, creating parent dirs.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

const validProfile = `---
name: test-agent
description: A test coding agent
model: claude-sonnet-4-6
tools:
  - bash
  - edit
initialPrompt: |
  You are a focused coding assistant.
---
Additional body text.
`

func TestAgentProfileLoad_ValidFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "test-agent.md"), validProfile)

	profiles, err := coding.LoadProfiles(dir, "")
	if err != nil {
		t.Fatalf("LoadProfiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("want 1 profile, got %d", len(profiles))
	}
	p := profiles[0]
	if p.Name != "test-agent" {
		t.Errorf("Name = %q, want %q", p.Name, "test-agent")
	}
	if p.Description != "A test coding agent" {
		t.Errorf("Description = %q, want %q", p.Description, "A test coding agent")
	}
	if p.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q, want %q", p.Model, "claude-sonnet-4-6")
	}
	if len(p.Tools) != 2 || p.Tools[0] != "bash" || p.Tools[1] != "edit" {
		t.Errorf("Tools = %v, want [bash edit]", p.Tools)
	}
	if p.Source != coding.AgentProfileSourceUser {
		t.Errorf("Source = %q, want %q", p.Source, coding.AgentProfileSourceUser)
	}
}

func TestAgentProfileLoad_FallbackNameFromFilename(t *testing.T) {
	dir := t.TempDir()
	// No frontmatter name — should fall back to filename stem.
	writeFile(t, filepath.Join(dir, "my-agent.md"), "---\ndescription: no name field\n---\n")

	profiles, err := coding.LoadProfiles(dir, "")
	if err != nil {
		t.Fatalf("LoadProfiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("want 1 profile, got %d", len(profiles))
	}
	if profiles[0].Name != "my-agent" {
		t.Errorf("Name = %q, want %q", profiles[0].Name, "my-agent")
	}
}

func TestAgentProfileLoad_NoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "plain.md"), "Just plain markdown body text.\n")

	profiles, err := coding.LoadProfiles(dir, "")
	if err != nil {
		t.Fatalf("LoadProfiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("want 1 profile, got %d", len(profiles))
	}
	p := profiles[0]
	if p.Name != "plain" {
		t.Errorf("Name = %q, want %q", p.Name, "plain")
	}
	if p.Description != "Just plain markdown body text." {
		t.Errorf("Description = %q", p.Description)
	}
}

func TestAgentProfileLoad_ProjectOverridesUser(t *testing.T) {
	userDir := t.TempDir()
	projectDir := t.TempDir()

	// User has a profile with model=claude-haiku-4-5.
	writeFile(t, filepath.Join(userDir, "reviewer.md"), `---
name: reviewer
model: claude-haiku-4-5
description: user version
---
`)
	// Project overrides same name with model=claude-opus-4-7.
	writeFile(t, filepath.Join(projectDir, "reviewer.md"), `---
name: reviewer
model: claude-opus-4-7
description: project version
---
`)

	profiles, err := coding.LoadProfiles(userDir, projectDir)
	if err != nil {
		t.Fatalf("LoadProfiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("want 1 profile (project wins), got %d", len(profiles))
	}
	p := profiles[0]
	if p.Model != "claude-opus-4-7" {
		t.Errorf("Model = %q, want project override %q", p.Model, "claude-opus-4-7")
	}
	if p.Source != coding.AgentProfileSourceProject {
		t.Errorf("Source = %q, want %q", p.Source, coding.AgentProfileSourceProject)
	}
}

func TestAgentProfileLoad_UserAndProjectCoexist(t *testing.T) {
	userDir := t.TempDir()
	projectDir := t.TempDir()

	writeFile(t, filepath.Join(userDir, "agent-a.md"), "---\nname: agent-a\n---\n")
	writeFile(t, filepath.Join(projectDir, "agent-b.md"), "---\nname: agent-b\n---\n")

	profiles, err := coding.LoadProfiles(userDir, projectDir)
	if err != nil {
		t.Fatalf("LoadProfiles: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("want 2 profiles, got %d", len(profiles))
	}
	// Results are alphabetical.
	if profiles[0].Name != "agent-a" || profiles[1].Name != "agent-b" {
		t.Errorf("unexpected order: %q %q", profiles[0].Name, profiles[1].Name)
	}
}

func TestAgentProfileLoad_MissingDirs(t *testing.T) {
	profiles, err := coding.LoadProfiles("/no/such/user/dir", "/no/such/project/dir")
	if err != nil {
		t.Fatalf("LoadProfiles with missing dirs should not error: %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("want 0 profiles from missing dirs, got %d", len(profiles))
	}
}

func TestAgentProfileLoad_EmptyDirs(t *testing.T) {
	userDir := t.TempDir()
	projectDir := t.TempDir()

	profiles, err := coding.LoadProfiles(userDir, projectDir)
	if err != nil {
		t.Fatalf("LoadProfiles: %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("want 0 profiles from empty dirs, got %d", len(profiles))
	}
}

func TestAgentProfileLoad_NonMdFilesIgnored(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agent.md"), "---\nname: good-agent\n---\n")
	writeFile(t, filepath.Join(dir, "config.yaml"), "name: not-an-agent\n")
	writeFile(t, filepath.Join(dir, "README.txt"), "some text")

	profiles, err := coding.LoadProfiles(dir, "")
	if err != nil {
		t.Fatalf("LoadProfiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("want 1 profile (only .md), got %d", len(profiles))
	}
}

func TestWriteAndDeleteProfile(t *testing.T) {
	dir := t.TempDir()
	p := coding.AgentProfile{
		Name:          "write-test",
		Description:   "A roundtrip test agent",
		Model:         "claude-sonnet-4-6",
		Tools:         []string{"read", "bash"},
		InitialPrompt: "Be concise.",
	}

	if err := coding.WriteProfile(dir, p); err != nil {
		t.Fatalf("WriteProfile: %v", err)
	}

	// Reload and verify.
	profiles, err := coding.LoadProfiles(dir, "")
	if err != nil {
		t.Fatalf("LoadProfiles after write: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("want 1 profile, got %d", len(profiles))
	}
	got := profiles[0]
	if got.Name != p.Name {
		t.Errorf("Name = %q, want %q", got.Name, p.Name)
	}
	if got.Model != p.Model {
		t.Errorf("Model = %q, want %q", got.Model, p.Model)
	}
	if got.InitialPrompt != p.InitialPrompt {
		t.Errorf("InitialPrompt = %q, want %q", got.InitialPrompt, p.InitialPrompt)
	}

	// Delete and verify gone.
	if err := coding.DeleteProfile(dir, p.Name); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	profiles, err = coding.LoadProfiles(dir, "")
	if err != nil {
		t.Fatalf("LoadProfiles after delete: %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("want 0 profiles after delete, got %d", len(profiles))
	}
}

func TestDeleteProfile_NotFound(t *testing.T) {
	dir := t.TempDir()
	err := coding.DeleteProfile(dir, "nonexistent")
	if err == nil {
		t.Error("DeleteProfile of nonexistent profile should return error")
	}
}
