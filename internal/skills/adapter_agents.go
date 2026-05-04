package skills

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// AgentsMDAdapter parses AGENTS.md files which serve as root-level agent
// instruction files in repositories.
type AgentsMDAdapter struct{}

// NewAgentsMDAdapter returns an AGENTS.md adapter.
func NewAgentsMDAdapter() AgentsMDAdapter {
	return AgentsMDAdapter{}
}

// Name implements Adapter.
func (AgentsMDAdapter) Name() string { return "agents-md" }

// CanHandle accepts files named AGENTS.md (case-insensitive).
func (AgentsMDAdapter) CanHandle(path string) bool {
	base := filepath.Base(path)
	return strings.EqualFold(base, "AGENTS.md")
}

// Parse implements Adapter. It treats the entire file as instructions and
// derives the skill name from the parent directory.
func (AgentsMDAdapter) Parse(path string, data []byte, tier contracts.SkillTier) (contracts.Skill, error) {
	// Extract name from parent directory
	dir := filepath.Dir(path)
	name := filepath.Base(dir)
	if name == "" || name == "." || name == "/" {
		name = "agents"
	}
	name = strings.TrimSpace(name)

	body := string(data)

	skill := contracts.Skill{
		Name:        name,
		Tier:        tier,
		Description: "",
		Tags:        []string{"agents"},
		Path:        path,
		Body:        strings.TrimSpace(body),
		UpdatedAt:   time.Time{},
	}
	return skill, nil
}
