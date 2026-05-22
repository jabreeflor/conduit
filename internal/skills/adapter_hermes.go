package skills

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
	"gopkg.in/yaml.v3"
)

// HermesAdapter parses YAML files with Hermes-specific frontmatter.
// Hermes is an agent framework by NousResearch that includes a `platforms:` field
// in its skill metadata.
type HermesAdapter struct{}

// NewHermesAdapter returns a Hermes skill adapter.
func NewHermesAdapter() HermesAdapter {
	return HermesAdapter{}
}

// Name implements Adapter.
func (HermesAdapter) Name() string { return "hermes" }

// CanHandle accepts hermes.json files anywhere, and .md files in paths
// that suggest Hermes origin (e.g., containing "hermes" in the path). This
// prevents conflicts with the broader MarkdownAdapter by narrowing the scope.
func (HermesAdapter) CanHandle(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if base == "hermes.json" {
		return true
	}
	ext := filepath.Ext(path)
	if !strings.EqualFold(ext, ".md") {
		return false
	}
	// Accept files in hermes-related directories or with hermes in the name
	return strings.Contains(strings.ToLower(path), "hermes") ||
		strings.Contains(base, "hermes")
}

type hermesFrontmatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
	Platforms   []string `yaml:"platforms"`
	ToolsFilter []string `yaml:"tools_filter"`
	Conditions  []string `yaml:"conditions"`
}

// Parse implements Adapter. It checks for the Hermes-specific `platforms:` field
// to confirm this is a Hermes skill before parsing. As a special case, files
// named hermes.json are parsed as JSON skill descriptors.
func (HermesAdapter) Parse(path string, data []byte, tier contracts.SkillTier) (contracts.Skill, error) {
	if strings.EqualFold(filepath.Base(path), "hermes.json") {
		var jf struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Body        string   `json:"body"`
			Tags        []string `json:"tags"`
		}
		if err := json.Unmarshal(data, &jf); err != nil {
			return contracts.Skill{}, fmt.Errorf("skills: parse Hermes JSON %q: %w", path, err)
		}
		name := strings.TrimSpace(jf.Name)
		if name == "" {
			base := filepath.Base(path)
			name = strings.TrimSuffix(base, filepath.Ext(base))
		}
		if name == "" {
			return contracts.Skill{}, fmt.Errorf("skills: %q has no usable name", path)
		}
		return contracts.Skill{
			Name:        name,
			Tier:        tier,
			Description: strings.TrimSpace(jf.Description),
			Tags:        normaliseTags(jf.Tags),
			Path:        path,
			Body:        strings.TrimSpace(jf.Body),
			UpdatedAt:   time.Time{},
		}, nil
	}

	front, body, err := splitHermesFrontmatter(data)
	if err != nil {
		return contracts.Skill{}, fmt.Errorf("skills: parse Hermes frontmatter %q: %w", path, err)
	}

	// Verify this is actually a Hermes skill by checking for platforms field
	if len(front.Platforms) == 0 {
		return contracts.Skill{}, fmt.Errorf("skills: %q missing required 'platforms' field for Hermes", path)
	}

	name := strings.TrimSpace(front.Name)
	if name == "" {
		base := filepath.Base(path)
		name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return contracts.Skill{}, fmt.Errorf("skills: %q has no usable name", path)
	}

	skill := contracts.Skill{
		Name:        name,
		Tier:        tier,
		Description: strings.TrimSpace(front.Description),
		Tags:        normaliseTags(front.Tags),
		Path:        path,
		Body:        strings.TrimSpace(body),
		UpdatedAt:   time.Time{},
	}
	return skill, nil
}

// splitHermesFrontmatter returns the parsed YAML header and the body that follows.
// It handles both LF and CRLF line endings.
func splitHermesFrontmatter(data []byte) (hermesFrontmatter, string, error) {
	normalised := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(normalised, []byte("---\n")) && !bytes.Equal(bytes.TrimSpace(normalised), []byte("---")) {
		return hermesFrontmatter{}, string(normalised), nil
	}

	rest := normalised[len("---\n"):]
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return hermesFrontmatter{}, string(normalised), nil
	}
	header := rest[:end]
	body := rest[end+len("\n---"):]
	body = bytes.TrimPrefix(body, []byte("\n"))

	var front hermesFrontmatter
	if len(bytes.TrimSpace(header)) > 0 {
		if err := yaml.Unmarshal(header, &front); err != nil {
			return hermesFrontmatter{}, "", err
		}
	}
	return front, string(body), nil
}
