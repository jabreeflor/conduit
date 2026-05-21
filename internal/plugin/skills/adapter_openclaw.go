package skills

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// OpenClawAdapter parses OpenClaw SOUL.md files that contain agent persona
// and instruction sections.
type OpenClawAdapter struct{}

// NewOpenClawAdapter returns an OpenClaw skill adapter.
func NewOpenClawAdapter() OpenClawAdapter {
	return OpenClawAdapter{}
}

// Name implements Adapter.
func (OpenClawAdapter) Name() string { return "openclaw" }

// CanHandle accepts files named SOUL.md (case-insensitive).
func (OpenClawAdapter) CanHandle(path string) bool {
	base := filepath.Base(path)
	return strings.EqualFold(base, "SOUL.md")
}

// Parse implements Adapter. It extracts persona/role sections from markdown
// and maps them to a Skill. If no structured sections are found, it treats
// the entire body as instructions.
func (OpenClawAdapter) Parse(path string, data []byte, tier contracts.SkillTier) (contracts.Skill, error) {
	body := string(data)

	// Extract name from parent directory
	dir := filepath.Dir(path)
	name := filepath.Base(dir)
	if name == "" || name == "." {
		name = "openclaw-soul"
	}
	name = strings.TrimSpace(name)

	// Extract description and role from markdown sections
	description := extractSection(body, "description", "Description")
	if description == "" {
		description = extractSection(body, "role", "Role")
	}
	if description == "" {
		description = extractSection(body, "persona", "Persona")
	}

	// Extract instructions section
	instructions := extractSection(body, "instructions", "Instructions")
	if instructions == "" {
		// Fallback: use full body if no instructions section found
		instructions = body
	}

	skill := contracts.Skill{
		Name:        name,
		Tier:        tier,
		Description: description,
		Tags:        []string{"openclaw"},
		Path:        path,
		Body:        strings.TrimSpace(instructions),
		UpdatedAt:   time.Time{},
	}
	return skill, nil
}

// extractSection finds a markdown section (## Header or ### Header) and returns
// its content until the next header or EOF.
func extractSection(content string, sectionNames ...string) string {
	lines := strings.Split(content, "\n")
	var capturing bool
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isMarkdownHeader(trimmed) {
			if capturing {
				break // next header ends the section
			}
			headerText := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			for _, name := range sectionNames {
				if strings.EqualFold(headerText, name) {
					capturing = true
					break
				}
			}
			continue
		}
		if capturing {
			result = append(result, line)
		}
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}

func isMarkdownHeader(line string) bool {
	return strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ") ||
		strings.HasPrefix(line, "### ") || strings.HasPrefix(line, "#### ")
}
