package skills

import (
	"path/filepath"
	"strings"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// Universal-source adapters (PRD §6.17). Each adapter recognises a source
// convention (Claude SKILL.md, Hermes, OpenClaw SOUL.md, Cursor rules,
// AGENTS.md) and normalises it into contracts.Skill. They all reuse the
// markdown frontmatter splitter so YAML headers are honoured when present;
// the only behavioural differences are name derivation and CanHandle.

// SkillMDAdapter handles Claude-style SKILL.md files. The skill name comes
// from the parent directory so a layout like skills/code-review/SKILL.md
// surfaces as "code-review".
type SkillMDAdapter struct{ md MarkdownAdapter }

// NewSkillMDAdapter returns the SKILL.md adapter.
func NewSkillMDAdapter() SkillMDAdapter { return SkillMDAdapter{} }

// Name implements Adapter.
func (SkillMDAdapter) Name() string { return "skill.md" }

// CanHandle implements Adapter.
func (SkillMDAdapter) CanHandle(path string) bool {
	return strings.EqualFold(filepath.Base(path), "SKILL.md")
}

// Parse implements Adapter.
func (a SkillMDAdapter) Parse(path string, data []byte, tier contracts.SkillTier) (contracts.Skill, error) {
	skill, err := a.md.Parse(path, data, tier)
	if err != nil {
		return contracts.Skill{}, err
	}
	if name := strings.TrimSpace(filepath.Base(filepath.Dir(path))); name != "" && skill.Name == strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) {
		skill.Name = name
	}
	return skill, nil
}

// SoulMDAdapter handles OpenClaw SOUL.md files. Same shape as SKILL.md;
// the convention difference is purely the filename, but we keep them as
// distinct adapters so import diagnostics can attribute provenance.
type SoulMDAdapter struct{ md MarkdownAdapter }

// NewSoulMDAdapter returns the SOUL.md adapter.
func NewSoulMDAdapter() SoulMDAdapter { return SoulMDAdapter{} }

// Name implements Adapter.
func (SoulMDAdapter) Name() string { return "soul.md" }

// CanHandle implements Adapter.
func (SoulMDAdapter) CanHandle(path string) bool {
	return strings.EqualFold(filepath.Base(path), "SOUL.md")
}

// Parse implements Adapter.
func (a SoulMDAdapter) Parse(path string, data []byte, tier contracts.SkillTier) (contracts.Skill, error) {
	skill, err := a.md.Parse(path, data, tier)
	if err != nil {
		return contracts.Skill{}, err
	}
	if name := strings.TrimSpace(filepath.Base(filepath.Dir(path))); name != "" {
		skill.Name = name
	}
	skill.Tags = appendUnique(skill.Tags, "openclaw")
	return skill, nil
}

// AllUniversalAdapters is the recommended adapter list for `conduit skills
// import` runs. Order matters: more specific filename matchers first so the
// generic markdown adapter doesn't claim SKILL.md before SkillMDAdapter does.
func AllUniversalAdapters() []Adapter {
	return []Adapter{
		NewHermesAdapter(),
		NewSkillMDAdapter(),
		NewSoulMDAdapter(),
		NewAgentsMDAdapter(),
		NewCursorRulesAdapter(),
		NewMarkdownAdapter(),
	}
}

func appendUnique(tags []string, extra string) []string {
	for _, t := range tags {
		if strings.EqualFold(t, extra) {
			return tags
		}
	}
	return append(tags, extra)
}
