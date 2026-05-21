package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestMarkdownAdapterParsesFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review.md")
	body := "---\nname: review\ndescription: Run a thorough review\ntags:\n  - quality\n  - review\n---\nDo the thing.\n"
	writeFile(t, path, body)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	skill, err := NewMarkdownAdapter().Parse(path, data, contracts.SkillTierPersonal)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if skill.Name != "review" {
		t.Errorf("name: got %q want %q", skill.Name, "review")
	}
	if skill.Description != "Run a thorough review" {
		t.Errorf("description: got %q", skill.Description)
	}
	if got, want := skill.Tags, []string{"quality", "review"}; !equalStrings(got, want) {
		t.Errorf("tags: got %v want %v", got, want)
	}
	if skill.Tier != contracts.SkillTierPersonal {
		t.Errorf("tier mismatch: %q", skill.Tier)
	}
	if strings.Contains(skill.Body, "---") {
		t.Errorf("body should not contain frontmatter fence, got %q", skill.Body)
	}
	if !strings.Contains(skill.Body, "Do the thing.") {
		t.Errorf("body missing markdown content, got %q", skill.Body)
	}
}

func TestMarkdownAdapterFallsBackToFilename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy-checklist.md")
	body := "Plain markdown without frontmatter.\nLine two.\n"
	writeFile(t, path, body)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	skill, err := NewMarkdownAdapter().Parse(path, data, contracts.SkillTierWorkspace)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if skill.Name != "deploy-checklist" {
		t.Errorf("expected filename fallback, got %q", skill.Name)
	}
	if !strings.Contains(skill.Body, "Plain markdown without frontmatter.") {
		t.Errorf("body must include full file when no frontmatter, got %q", skill.Body)
	}
	if len(skill.Tags) != 0 {
		t.Errorf("expected no tags, got %v", skill.Tags)
	}
}

func TestMarkdownAdapterMalformedYAMLErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.md")
	body := "---\nname: [unterminated\n---\nbody\n"
	writeFile(t, path, body)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, err := NewMarkdownAdapter().Parse(path, data, contracts.SkillTierImported); err == nil {
		t.Fatalf("expected error on malformed YAML")
	}
}

func TestMarkdownAdapterCanHandle(t *testing.T) {
	a := NewMarkdownAdapter()
	if !a.CanHandle("/x/y.md") || !a.CanHandle("/x/y.MD") {
		t.Errorf("expected .md to be handled regardless of case")
	}
	if a.CanHandle("/x/y.txt") {
		t.Errorf("expected .txt to be rejected")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
