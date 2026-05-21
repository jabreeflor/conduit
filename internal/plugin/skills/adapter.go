// Package skills owns Conduit's skill registry: filesystem-backed,
// hierarchically resolved, indexed for task-start lookup.
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

// Adapter converts a single source file on disk into a contracts.Skill.
// The interface is intentionally narrow so future adapters (Hermes, OpenClaw
// SOUL.md, Cursor rules, AGENTS.md, ...) can be plugged in without changes
// to the registry — they only need to recognize their own files.
type Adapter interface {
	// Name identifies the adapter for diagnostic logging.
	Name() string
	// CanHandle does a cheap shape sniff (extension, marker file, etc.) so the
	// registry can pick the first matching adapter without reading the file
	// twice.
	CanHandle(path string) bool
	// Parse converts file bytes into a Skill, attaching the caller-supplied
	// tier so the same adapter implementation works in every layer.
	Parse(path string, data []byte, tier contracts.SkillTier) (contracts.Skill, error)
}

// MarkdownAdapter parses plain markdown files with optional YAML frontmatter
// fenced by `---`. It is the baseline adapter every conduit install ships
// with; richer adapters layer on top of the same interface.
type MarkdownAdapter struct{}

// NewMarkdownAdapter returns the default markdown adapter. It is stateless,
// but the constructor matches the project's New* convention so future caching
// (compiled regex, mmap, ...) lands without touching call sites.
func NewMarkdownAdapter() MarkdownAdapter {
	return MarkdownAdapter{}
}

// Name implements Adapter.
func (MarkdownAdapter) Name() string { return "markdown" }

// CanHandle accepts any *.md file. Tier roots already constrain the search,
// so we keep the sniff inexpensive and rely on Parse for stricter validation.
func (MarkdownAdapter) CanHandle(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".md")
}

type markdownFrontmatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
}

// Parse implements Adapter. Files missing both frontmatter `name` and a
// meaningful filename are rejected so the caller can log and skip them rather
// than indexing nameless skills the user can never look up.
func (MarkdownAdapter) Parse(path string, data []byte, tier contracts.SkillTier) (contracts.Skill, error) {
	front, body, err := splitFrontmatter(data)
	if err != nil {
		return contracts.Skill{}, fmt.Errorf("skills: parse frontmatter %q: %w", path, err)
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

// splitFrontmatter returns the parsed YAML header (zero-value when absent) and
// the markdown body that follows it. We tolerate both LF and CRLF inputs
// because skill libraries are routinely shared via Windows-edited zips.
func splitFrontmatter(data []byte) (markdownFrontmatter, string, error) {
	normalised := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(normalised, []byte("---\n")) && !bytes.Equal(bytes.TrimSpace(normalised), []byte("---")) {
		return markdownFrontmatter{}, string(normalised), nil
	}

	rest := normalised[len("---\n"):]
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return markdownFrontmatter{}, string(normalised), nil
	}
	header := rest[:end]
	body := rest[end+len("\n---"):]
	body = bytes.TrimPrefix(body, []byte("\n"))

	var front markdownFrontmatter
	if len(bytes.TrimSpace(header)) > 0 {
		if err := yaml.Unmarshal(header, &front); err != nil {
			return markdownFrontmatter{}, "", err
		}
	}
	return front, string(body), nil
}

func normaliseTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		out = append(out, tag)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
