package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestImporterImportsSingleFile(t *testing.T) {
	sourceDir := t.TempDir()
	importDir := t.TempDir()

	// Create a source skill file
	skillFile := filepath.Join(sourceDir, "my-skill.md")
	skillBody := "---\nname: test-skill\ndescription: Test\n---\nBody content\n"
	writeFile(t, skillFile, skillBody)

	// Create a minimal registry (just need it to exist)
	reg := &Registry{}

	importer, err := NewImporter(reg, importDir)
	if err != nil {
		t.Fatalf("NewImporter: %v", err)
	}

	// Import the file
	if err := importer.Import(skillFile); err != nil {
		t.Fatalf("Import: %v", err)
	}

	// Check that the file was copied to imported directory
	expectedPath := filepath.Join(importDir, "imported", "my-skill.md")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Errorf("skill file not found at %q: %v", expectedPath, err)
	}

	// Verify content was copied
	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read imported file: %v", err)
	}
	if !strings.Contains(string(data), "Body content") {
		t.Errorf("imported file content corrupted, got %q", string(data))
	}
}

func TestImporterImportsDirectory(t *testing.T) {
	sourceDir := t.TempDir()
	importDir := t.TempDir()

	// Create multiple skill files
	skillsDir := filepath.Join(sourceDir, "skills")
	os.MkdirAll(skillsDir, 0o755)

	writeFile(t, filepath.Join(skillsDir, "skill1.md"), "---\nname: skill1\n---\nContent1")
	writeFile(t, filepath.Join(skillsDir, "skill2.md"), "---\nname: skill2\n---\nContent2")

	reg := &Registry{}
	importer, err := NewImporter(reg, importDir)
	if err != nil {
		t.Fatalf("NewImporter: %v", err)
	}

	// Import the directory
	if err := importer.Import(skillsDir); err != nil {
		t.Fatalf("Import: %v", err)
	}

	// Check both files were imported
	path1 := filepath.Join(importDir, "imported", "skill1.md")
	path2 := filepath.Join(importDir, "imported", "skill2.md")

	if _, err := os.Stat(path1); err != nil {
		t.Errorf("skill1 not found: %v", err)
	}
	if _, err := os.Stat(path2); err != nil {
		t.Errorf("skill2 not found: %v", err)
	}
}

func TestImporterRecordsSources(t *testing.T) {
	sourceDir := t.TempDir()
	importDir := t.TempDir()

	skillFile := filepath.Join(sourceDir, "skill.md")
	writeFile(t, skillFile, "---\nname: skill\n---\nBody")

	reg := &Registry{}
	importer, err := NewImporter(reg, importDir)
	if err != nil {
		t.Fatalf("NewImporter: %v", err)
	}

	// Import and record source
	if err := importer.recordSource("test-provider", sourceDir); err != nil {
		t.Fatalf("recordSource: %v", err)
	}

	// Check .sources.json was created
	sourcesPath := filepath.Join(importDir, ".sources.json")
	data, err := os.ReadFile(sourcesPath)
	if err != nil {
		t.Fatalf("read sources: %v", err)
	}

	var sources sourcesFile
	if err := json.Unmarshal(data, &sources); err != nil {
		t.Fatalf("unmarshal sources: %v", err)
	}

	if len(sources.Sources) != 1 {
		t.Errorf("expected 1 source, got %d", len(sources.Sources))
	}

	rec := sources.Sources[0]
	if rec.Provider != "test-provider" {
		t.Errorf("provider: got %q want test-provider", rec.Provider)
	}
	if rec.Source != sourceDir {
		t.Errorf("source: got %q want %q", rec.Source, sourceDir)
	}
}

func TestImporterSync(t *testing.T) {
	sourceDir := t.TempDir()
	importDir := t.TempDir()

	// Create initial skill file
	skillFile := filepath.Join(sourceDir, "skill.md")
	writeFile(t, skillFile, "---\nname: original\n---\nOriginal body")

	reg := &Registry{}
	importer, err := NewImporter(reg, importDir)
	if err != nil {
		t.Fatalf("NewImporter: %v", err)
	}

	// First import
	if err := importer.Import(skillFile); err != nil {
		t.Fatalf("Import: %v", err)
	}

	// Record source
	if err := importer.recordSource("imported", sourceDir); err != nil {
		t.Fatalf("recordSource: %v", err)
	}

	// Update the source file
	updatedBody := "---\nname: updated\n---\nUpdated body"
	if err := os.WriteFile(skillFile, []byte(updatedBody), 0o644); err != nil {
		t.Fatalf("write updated skill: %v", err)
	}

	// Sync should re-import updated file
	if err := importer.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Check that imported file was updated
	importedPath := filepath.Join(importDir, "imported", "skill.md")
	data, err := os.ReadFile(importedPath)
	if err != nil {
		t.Fatalf("read imported skill: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Updated body") {
		t.Errorf("expected updated content, got %q", content)
	}
}

func TestImporterSyncWithoutSourcesFile(t *testing.T) {
	importDir := t.TempDir()

	reg := &Registry{}
	importer, err := NewImporter(reg, importDir)
	if err != nil {
		t.Fatalf("NewImporter: %v", err)
	}

	// Sync when no sources file exists should not error
	if err := importer.Sync(); err != nil {
		t.Errorf("Sync with no sources: %v", err)
	}
}

func TestImporterSkipsUnsupportedFiles(t *testing.T) {
	sourceDir := t.TempDir()
	importDir := t.TempDir()

	// Create mixed file types
	writeFile(t, filepath.Join(sourceDir, "skill.md"), "---\nname: skill\n---\nBody")
	writeFile(t, filepath.Join(sourceDir, "readme.txt"), "Just some text")
	writeFile(t, filepath.Join(sourceDir, ".gitignore"), "*.tmp")

	reg := &Registry{}
	importer, err := NewImporter(reg, importDir)
	if err != nil {
		t.Fatalf("NewImporter: %v", err)
	}

	// Import directory
	if err := importer.importDir(sourceDir, "test"); err != nil {
		t.Fatalf("importDir: %v", err)
	}

	// Only skill.md should have been imported
	skillPath := filepath.Join(importDir, "test", "skill.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Errorf("skill.md not found: %v", err)
	}

	txtPath := filepath.Join(importDir, "test", "readme.txt")
	if _, err := os.Stat(txtPath); err == nil {
		t.Errorf("readme.txt should not have been imported")
	}
}

func TestShouldImportFile(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		expected bool
	}{
		{"markdown", "skill.md", true},
		{"skill", "SKILL.md", true},
		{"cursor rules", ".cursorrules", true},
		{"cursor rules in folder", ".cursor/rules/style.mdc", true},
		{"mdc file", "custom.mdc", true},
		{"openclaw soul", "SOUL.md", true},
		{"agents file", "AGENTS.md", true},
		{"text file", "readme.txt", false},
		{"python file", "script.py", false},
		{"json file", "config.json", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldImportFile(tt.filename)
			if result != tt.expected {
				t.Errorf("shouldImportFile(%q): got %v want %v", tt.filename, result, tt.expected)
			}
		})
	}
}

func TestImporterUpdatesSyncTime(t *testing.T) {
	sourceDir := t.TempDir()
	importDir := t.TempDir()

	reg := &Registry{}
	importer, err := NewImporter(reg, importDir)
	if err != nil {
		t.Fatalf("NewImporter: %v", err)
	}

	// Record source twice
	if err := importer.recordSource("provider", sourceDir); err != nil {
		t.Fatalf("first recordSource: %v", err)
	}

	firstTime := time.Now()
	time.Sleep(10 * time.Millisecond)

	if err := importer.recordSource("provider", sourceDir); err != nil {
		t.Fatalf("second recordSource: %v", err)
	}

	// Read sources and verify LastSyncedAt was updated
	data, err := os.ReadFile(filepath.Join(importDir, ".sources.json"))
	if err != nil {
		t.Fatalf("read sources: %v", err)
	}

	var sources sourcesFile
	if err := json.Unmarshal(data, &sources); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(sources.Sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources.Sources))
	}

	// LastSyncedAt should be after firstTime
	if sources.Sources[0].LastSyncedAt.Before(firstTime) {
		t.Errorf("LastSyncedAt not updated")
	}

	// ImportedAt should not have changed
	if sources.Sources[0].ImportedAt.After(sources.Sources[0].LastSyncedAt) {
		t.Errorf("ImportedAt should not change on re-record")
	}
}
