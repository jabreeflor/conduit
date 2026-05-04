package skills

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
	"gopkg.in/yaml.v3"
)

// CursorRulesAdapter parses Cursor editor rules files (.cursorrules and .mdc files).
type CursorRulesAdapter struct{}

// NewCursorRulesAdapter returns a Cursor rules adapter.
func NewCursorRulesAdapter() CursorRulesAdapter {
	return CursorRulesAdapter{}
}

// Name implements Adapter.
func (CursorRulesAdapter) Name() string { return "cursor" }

// CanHandle accepts .cursorrules files and .mdc files (Markdown with Code).
func (CursorRulesAdapter) CanHandle(path string) bool {
	base := filepath.Base(path)
	ext := filepath.Ext(path)

	// Accept .cursorrules files
	if strings.EqualFold(base, ".cursorrules") {
		return true
	}

	// Accept .mdc files in .cursor/rules/ directory
	if strings.EqualFold(ext, ".mdc") && strings.Contains(path, ".cursor") {
		return true
	}

	return false
}

// cursorFrontmatter represents optional YAML header in .mdc files.
type cursorFrontmatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
}

// Parse implements Adapter. It treats .cursorrules as plain text and .mdc files
// may have YAML frontmatter.
func (CursorRulesAdapter) Parse(path string, data []byte, tier contracts.SkillTier) (contracts.Skill, error) {
	ext := filepath.Ext(path)

	var name, description, body string

	if strings.EqualFold(ext, ".mdc") {
		// Try to parse YAML frontmatter from .mdc file
		front, b, err := splitCursorFrontmatter(data)
		if err != nil {
			return contracts.Skill{}, fmt.Errorf("skills: parse Cursor frontmatter %q: %w", path, err)
		}
		body = b
		name = strings.TrimSpace(front.Name)
		description = strings.TrimSpace(front.Description)
	} else {
		// .cursorrules: treat entire file as body
		body = string(data)
	}

	// Fallback to filename or fixed name for .cursorrules
	if name == "" {
		base := filepath.Base(path)
		if strings.EqualFold(base, ".cursorrules") {
			name = "cursor-rules"
		} else {
			name = strings.TrimSuffix(base, filepath.Ext(base))
		}
	}
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == "/" {
		name = "cursor-rules"
	}

	skill := contracts.Skill{
		Name:        name,
		Tier:        tier,
		Description: description,
		Tags:        []string{"cursor"},
		Path:        path,
		Body:        strings.TrimSpace(body),
		UpdatedAt:   time.Time{},
	}
	return skill, nil
}

// splitCursorFrontmatter parses optional YAML frontmatter from .mdc files.
func splitCursorFrontmatter(data []byte) (cursorFrontmatter, string, error) {
	normalised := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(normalised, []byte("---\n")) && !bytes.Equal(bytes.TrimSpace(normalised), []byte("---")) {
		return cursorFrontmatter{}, string(normalised), nil
	}

	rest := normalised[len("---\n"):]
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return cursorFrontmatter{}, string(normalised), nil
	}
	header := rest[:end]
	body := rest[end+len("\n---"):]
	body = bytes.TrimPrefix(body, []byte("\n"))

	var front cursorFrontmatter
	if len(bytes.TrimSpace(header)) > 0 {
		if err := yaml.Unmarshal(header, &front); err != nil {
			return cursorFrontmatter{}, "", err
		}
	}
	return front, string(body), nil
}
