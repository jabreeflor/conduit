package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// Importer handles importing skills from external sources (Hermes, OpenClaw, etc.)
// and manages skill synchronization.
type Importer struct {
	registry *Registry
	// importedTierRoot is the directory where imported skills are stored
	importedTierRoot string
	// sourcesPath is the path to .sources.json tracking imported sources
	sourcesPath string
}

// NewImporter creates an importer that persists imported skills to the
// supplied importedTierRoot directory.
func NewImporter(registry *Registry, importedTierRoot string) (*Importer, error) {
	if err := os.MkdirAll(importedTierRoot, 0o755); err != nil {
		return nil, fmt.Errorf("skills: mkdir imported tier: %w", err)
	}

	sourcesPath := filepath.Join(importedTierRoot, ".sources.json")

	return &Importer{
		registry:         registry,
		importedTierRoot: importedTierRoot,
		sourcesPath:      sourcesPath,
	}, nil
}

// SourceRecord tracks an imported skill source for later sync.
type SourceRecord struct {
	Provider     string    `json:"provider"`
	Source       string    `json:"source"`
	ImportedAt   time.Time `json:"imported_at"`
	LastSyncedAt time.Time `json:"last_synced_at"`
}

// sourcesFile is the format of .sources.json.
type sourcesFile struct {
	Sources []SourceRecord `json:"sources"`
}

// Import imports a single file or directory of skills from the given source path.
// It automatically detects the skill format and uses the appropriate adapter.
// Imported skills are written to the importedTierRoot with "imported" as the provider.
func (imp *Importer) Import(source string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("skills: stat source %q: %w", source, err)
	}

	if info.IsDir() {
		return imp.importDir(source, "imported")
	}
	return imp.importFile(source, "imported")
}

// ImportFrom imports skills from a known provider. Supported providers:
// - "hermes": ~/.hermes/skills/
// - "openclaw": ~/.openclaw/
// Imported skills are tagged with the provider name.
func (imp *Importer) ImportFrom(provider string) error {
	var sourceDir string

	switch strings.ToLower(provider) {
	case "hermes":
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("skills: get home dir: %w", err)
		}
		sourceDir = filepath.Join(home, ".hermes", "skills")
	case "openclaw":
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("skills: get home dir: %w", err)
		}
		sourceDir = filepath.Join(home, ".openclaw")
	default:
		return fmt.Errorf("skills: unknown provider %q", provider)
	}

	if err := imp.importDir(sourceDir, provider); err != nil {
		return err
	}

	// Record this source for future syncs
	return imp.recordSource(provider, sourceDir)
}

// Sync re-imports all tracked sources from .sources.json.
func (imp *Importer) Sync() error {
	data, err := os.ReadFile(imp.sourcesPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No sources file yet; nothing to sync
			return nil
		}
		return fmt.Errorf("skills: read sources: %w", err)
	}

	var sources sourcesFile
	if err := json.Unmarshal(data, &sources); err != nil {
		return fmt.Errorf("skills: parse sources: %w", err)
	}

	for _, rec := range sources.Sources {
		if strings.ToLower(rec.Provider) == "imported" {
			// Re-import from specific path
			if err := imp.Import(rec.Source); err != nil {
				return fmt.Errorf("skills: sync %q: %w", rec.Source, err)
			}
		} else {
			// Re-import from known provider
			if err := imp.ImportFrom(rec.Provider); err != nil {
				return fmt.Errorf("skills: sync provider %q: %w", rec.Provider, err)
			}
		}
	}

	return nil
}

// importDir recursively imports all skills from a directory.
func (imp *Importer) importDir(dir string, provider string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("skills: read dir %q: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.Name() == ".sources.json" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			// Recurse into subdirectories
			if err := imp.importDir(path, provider); err != nil {
				return err
			}
		} else if shouldImportFile(entry.Name()) {
			if err := imp.importFile(path, provider); err != nil {
				// Log error but continue with other files
				fmt.Fprintf(os.Stderr, "warning: import %q: %v\n", path, err)
			}
		}
	}

	return nil
}

// importFile imports a single skill file.
func (imp *Importer) importFile(path string, provider string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	// Try each adapter in order until one succeeds
	adapters := []Adapter{
		NewHermesAdapter(),
		NewOpenClawAdapter(),
		NewCursorRulesAdapter(),
		NewAgentsMDAdapter(),
		NewMarkdownAdapter(),
	}

	var parseErr error

	for _, adapter := range adapters {
		if !adapter.CanHandle(path) {
			continue
		}

		_, err := adapter.Parse(path, data, contracts.SkillTierImported)
		if err == nil {
			parseErr = nil
			break
		}
		parseErr = err
	}

	if parseErr != nil {
		return fmt.Errorf("no adapter could parse: %w", parseErr)
	}

	// Write the skill to the imported tier directory
	// Use provider as a subdirectory to avoid conflicts
	destDir := filepath.Join(imp.importedTierRoot, provider)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	// Preserve original filename
	destPath := filepath.Join(destDir, filepath.Base(path))

	if err := os.WriteFile(destPath, data, 0o644); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

// recordSource adds or updates a source record in .sources.json.
func (imp *Importer) recordSource(provider, source string) error {
	var sources sourcesFile

	// Read existing sources if available
	if data, err := os.ReadFile(imp.sourcesPath); err == nil {
		if err := json.Unmarshal(data, &sources); err != nil {
			return fmt.Errorf("skills: parse sources: %w", err)
		}
	}

	// Check if this source already exists and update it
	found := false
	now := time.Now()
	for i, rec := range sources.Sources {
		if rec.Provider == provider && rec.Source == source {
			sources.Sources[i].LastSyncedAt = now
			found = true
			break
		}
	}

	// Add new record if not found
	if !found {
		sources.Sources = append(sources.Sources, SourceRecord{
			Provider:     provider,
			Source:       source,
			ImportedAt:   now,
			LastSyncedAt: now,
		})
	}

	// Write updated sources file
	data, err := json.MarshalIndent(sources, "", "  ")
	if err != nil {
		return fmt.Errorf("skills: marshal sources: %w", err)
	}

	if err := os.WriteFile(imp.sourcesPath, data, 0o644); err != nil {
		return fmt.Errorf("skills: write sources: %w", err)
	}

	return nil
}

// shouldImportFile determines if a file should be imported based on extension.
func shouldImportFile(name string) bool {
	ext := filepath.Ext(name)
	base := filepath.Base(name)

	// Check for known skill file types
	switch {
	case strings.EqualFold(ext, ".md"):
		return true
	case strings.EqualFold(base, ".cursorrules"):
		return true
	case strings.EqualFold(ext, ".mdc"):
		return true
	case strings.EqualFold(base, "SOUL.md"):
		return true
	case strings.EqualFold(base, "AGENTS.md"):
		return true
	case strings.EqualFold(base, "SKILL.md"):
		return true
	}

	return false
}
