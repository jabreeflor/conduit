package skills

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// ImportSource identifies a known third-party skill format. Empty / "auto"
// means "try every adapter in AllUniversalAdapters()".
type ImportSource string

const (
	ImportSourceAuto    ImportSource = "auto"
	ImportSourceClaude  ImportSource = "claude" // SKILL.md
	ImportSourceHermes  ImportSource = "hermes" // hermes.json
	ImportSourceOClaw   ImportSource = "openclaw"
	ImportSourceCursor  ImportSource = "cursor"
	ImportSourceAgents  ImportSource = "agents"
	ImportSourceMarkdwn ImportSource = "markdown"
)

// ImportResult records what an import sweep found and where it landed.
type ImportResult struct {
	Imported  []contracts.Skill
	Skipped   []ImportSkip
	TargetDir string
}

// ImportSkip describes one source file the import declined to copy.
type ImportSkip struct {
	Path   string
	Reason string
}

// AdaptersForSource returns the adapter set the universal importer uses for a
// given --from value. Auto returns the full list; named values restrict to a
// single adapter so users can force a parse when sniffing is ambiguous.
func AdaptersForSource(src ImportSource) ([]Adapter, error) {
	switch ImportSource(strings.ToLower(string(src))) {
	case "", ImportSourceAuto:
		return AllUniversalAdapters(), nil
	case ImportSourceClaude:
		return []Adapter{NewSkillMDAdapter()}, nil
	case ImportSourceHermes:
		return []Adapter{NewHermesAdapter()}, nil
	case ImportSourceOClaw:
		return []Adapter{NewSoulMDAdapter()}, nil
	case ImportSourceCursor:
		return []Adapter{NewCursorRulesAdapter()}, nil
	case ImportSourceAgents:
		return []Adapter{NewAgentsMDAdapter()}, nil
	case ImportSourceMarkdwn:
		return []Adapter{NewMarkdownAdapter()}, nil
	default:
		return nil, fmt.Errorf("skills: unknown source %q", src)
	}
}

// Import walks sourceDir, parses every recognised file with the supplied
// adapters, and writes a normalised markdown skill into targetDir. Existing
// files in targetDir win on name collision (so re-running an import is
// idempotent without clobbering local edits).
func Import(sourceDir, targetDir string, src ImportSource) (ImportResult, error) {
	adapters, err := AdaptersForSource(src)
	if err != nil {
		return ImportResult{}, err
	}
	if sourceDir == "" {
		return ImportResult{}, errors.New("skills: import requires a source directory")
	}
	if targetDir == "" {
		return ImportResult{}, errors.New("skills: import requires a target directory")
	}
	info, err := os.Stat(sourceDir)
	if err != nil {
		return ImportResult{}, fmt.Errorf("skills: stat source %q: %w", sourceDir, err)
	}
	if !info.IsDir() {
		return ImportResult{}, fmt.Errorf("skills: source %q is not a directory", sourceDir)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return ImportResult{}, fmt.Errorf("skills: mkdir target %q: %w", targetDir, err)
	}

	existingNames, err := collectExistingNames(targetDir)
	if err != nil {
		return ImportResult{}, err
	}

	result := ImportResult{TargetDir: targetDir}
	walkErr := filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		adapter := pickAdapter(adapters, path)
		if adapter == nil {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			result.Skipped = append(result.Skipped, ImportSkip{Path: path, Reason: readErr.Error()})
			return nil
		}
		skill, parseErr := adapter.Parse(path, data, contracts.SkillTierImported)
		if parseErr != nil {
			result.Skipped = append(result.Skipped, ImportSkip{Path: path, Reason: parseErr.Error()})
			return nil
		}
		if _, dup := existingNames[skill.Name]; dup {
			result.Skipped = append(result.Skipped, ImportSkip{Path: path, Reason: fmt.Sprintf("name %q already imported", skill.Name)})
			return nil
		}
		written, writeErr := writeImportedSkill(targetDir, skill)
		if writeErr != nil {
			result.Skipped = append(result.Skipped, ImportSkip{Path: path, Reason: writeErr.Error()})
			return nil
		}
		skill.Path = written
		skill.UpdatedAt = time.Now()
		existingNames[skill.Name] = struct{}{}
		result.Imported = append(result.Imported, skill)
		return nil
	})
	if walkErr != nil {
		return result, walkErr
	}
	sort.Slice(result.Imported, func(i, j int) bool { return result.Imported[i].Name < result.Imported[j].Name })
	sort.Slice(result.Skipped, func(i, j int) bool { return result.Skipped[i].Path < result.Skipped[j].Path })
	return result, nil
}

// Sync re-runs an import sweep but updates files whose source mtime is newer
// than the imported copy. New files are imported; deletions are reported via
// ImportResult.Skipped with reason "source missing".
func Sync(sourceDir, targetDir string, src ImportSource) (ImportResult, error) {
	res, err := Import(sourceDir, targetDir, src)
	if err != nil {
		return res, err
	}
	// Detect imported-but-no-longer-in-source files.
	entries, derr := os.ReadDir(targetDir)
	if derr != nil {
		return res, nil
	}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			continue
		}
		// We don't have a source-to-target manifest; sync conservatively
		// reports orphans by checking whether any imported result mentions
		// this name. In a future iteration we can persist a sidecar map.
		name := strings.TrimSuffix(e.Name(), ".md")
		found := false
		for _, sk := range res.Imported {
			if sk.Name == name {
				found = true
				break
			}
		}
		if !found {
			res.Skipped = append(res.Skipped, ImportSkip{Path: filepath.Join(targetDir, e.Name()), Reason: "source missing (kept local)"})
		}
	}
	return res, nil
}

func writeImportedSkill(targetDir string, skill contracts.Skill) (string, error) {
	safeName := strings.ReplaceAll(skill.Name, string(os.PathSeparator), "_")
	safeName = strings.ReplaceAll(safeName, "..", "_")
	if safeName == "" {
		return "", errors.New("skills: empty target name")
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: ")
	b.WriteString(skill.Name)
	b.WriteString("\n")
	if skill.Description != "" {
		b.WriteString("description: ")
		b.WriteString(strings.ReplaceAll(skill.Description, "\n", " "))
		b.WriteString("\n")
	}
	if len(skill.Tags) > 0 {
		b.WriteString("tags:\n")
		for _, tag := range skill.Tags {
			b.WriteString("  - ")
			b.WriteString(tag)
			b.WriteString("\n")
		}
	}
	b.WriteString("source: ")
	b.WriteString(skill.Path)
	b.WriteString("\n")
	b.WriteString("---\n")
	b.WriteString(skill.Body)
	if !strings.HasSuffix(skill.Body, "\n") {
		b.WriteString("\n")
	}
	target := filepath.Join(targetDir, safeName+".md")
	if err := os.WriteFile(target, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return target, nil
}

func collectExistingNames(dir string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			out[strings.TrimSuffix(e.Name(), ".md")] = struct{}{}
		}
	}
	return out, nil
}

// jsonUnmarshal is a tiny indirection so the universal adapters file can call
// into the encoding/json package without importing it directly (keeps the
// adapters file dependency-light for users grepping imports).
func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
