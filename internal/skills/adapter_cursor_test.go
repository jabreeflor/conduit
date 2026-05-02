package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestCursorRulesAdapterParsesPlainCursorrules(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".cursorrules")
	body := "You are a helpful assistant.\nFollow these rules...\n"
	writeFile(t, path, body)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	skill, err := NewCursorRulesAdapter().Parse(path, data, contracts.SkillTierImported)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if skill.Name != "cursor-rules" {
		t.Errorf("name: got %q want %q", skill.Name, "cursor-rules")
	}

	if !strings.Contains(skill.Body, "You are a helpful assistant.") {
		t.Errorf("body should contain rules, got %q", skill.Body)
	}

	if !contains(skill.Tags, "cursor") {
		t.Errorf("tags should include cursor, got %v", skill.Tags)
	}
}

func TestCursorRulesAdapterParsesMDCwithFrontmatter(t *testing.T) {
	dir := t.TempDir()
	cursorDir := filepath.Join(dir, ".cursor", "rules")
	os.MkdirAll(cursorDir, 0o755)
	path := filepath.Join(cursorDir, "custom.mdc")
	body := "---\nname: my-cursor-rule\ndescription: Custom cursor behavior\ntags:\n  - style\n---\nImplementation details.\n"
	writeFile(t, path, body)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	skill, err := NewCursorRulesAdapter().Parse(path, data, contracts.SkillTierImported)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if skill.Name != "my-cursor-rule" {
		t.Errorf("name: got %q want %q", skill.Name, "my-cursor-rule")
	}

	if skill.Description != "Custom cursor behavior" {
		t.Errorf("description: got %q", skill.Description)
	}

	if !strings.Contains(skill.Body, "Implementation details.") {
		t.Errorf("body should contain content, got %q", skill.Body)
	}
}

func TestCursorRulesAdapterMDCFallsBackToFilename(t *testing.T) {
	dir := t.TempDir()
	cursorDir := filepath.Join(dir, ".cursor", "rules")
	os.MkdirAll(cursorDir, 0o755)
	path := filepath.Join(cursorDir, "style-guide.mdc")
	body := "Plain content without frontmatter.\n"
	writeFile(t, path, body)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	skill, err := NewCursorRulesAdapter().Parse(path, data, contracts.SkillTierImported)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if skill.Name != "style-guide" {
		t.Errorf("expected filename fallback, got %q", skill.Name)
	}
}

func TestCursorRulesAdapterCanHandle(t *testing.T) {
	a := NewCursorRulesAdapter()

	if !a.CanHandle("/home/user/.cursorrules") {
		t.Errorf("expected .cursorrules to be handled")
	}

	if !a.CanHandle("/project/.cursor/rules/custom.mdc") {
		t.Errorf("expected .mdc in .cursor to be handled")
	}

	if a.CanHandle("/project/custom.mdc") {
		t.Errorf("expected .mdc without .cursor dir to be rejected")
	}

	if a.CanHandle("/path/to/rules.txt") {
		t.Errorf("expected .txt to be rejected")
	}
}
