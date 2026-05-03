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

// AgentsMDAdapter handles AGENTS.md instruction files. Tagged so surfaces
// can group these separately from prompt-style skills.
type AgentsMDAdapter struct{ md MarkdownAdapter }

// NewAgentsMDAdapter returns the AGENTS.md adapter.
func NewAgentsMDAdapter() AgentsMDAdapter { return AgentsMDAdapter{} }

// Name implements Adapter.
func (AgentsMDAdapter) Name() string { return "agents.md" }

// CanHandle implements Adapter.
func (AgentsMDAdapter) CanHandle(path string) bool {
	return strings.EqualFold(filepath.Base(path), "AGENTS.md")
}

// Parse implements Adapter.
func (a AgentsMDAdapter) Parse(path string, data []byte, tier contracts.SkillTier) (contracts.Skill, error) {
	skill, err := a.md.Parse(path, data, tier)
	if err != nil {
		return contracts.Skill{}, err
	}
	if name := strings.TrimSpace(filepath.Base(filepath.Dir(path))); name != "" {
		skill.Name = "agents:" + name
	} else {
		skill.Name = "agents"
	}
	skill.Tags = appendUnique(skill.Tags, "agents")
	return skill, nil
}

// CursorRulesAdapter handles Cursor's .cursorrules and .cursor/rules/*.md
// files. These are typically frontmatter-less; we wrap the body and derive
// the name from the file path.
type CursorRulesAdapter struct{ md MarkdownAdapter }

// NewCursorRulesAdapter returns the Cursor rules adapter.
func NewCursorRulesAdapter() CursorRulesAdapter { return CursorRulesAdapter{} }

// Name implements Adapter.
func (CursorRulesAdapter) Name() string { return "cursor.rules" }

// CanHandle implements Adapter.
func (CursorRulesAdapter) CanHandle(path string) bool {
	base := filepath.Base(path)
	if strings.EqualFold(base, ".cursorrules") {
		return true
	}
	parent := filepath.Base(filepath.Dir(path))
	return strings.EqualFold(parent, "rules") &&
		strings.EqualFold(filepath.Base(filepath.Dir(filepath.Dir(path))), ".cursor") &&
		strings.EqualFold(filepath.Ext(base), ".md")
}

// Parse implements Adapter.
func (a CursorRulesAdapter) Parse(path string, data []byte, tier contracts.SkillTier) (contracts.Skill, error) {
	skill, err := a.md.Parse(path, data, tier)
	if err != nil {
		return contracts.Skill{}, err
	}
	if strings.EqualFold(filepath.Base(path), ".cursorrules") {
		skill.Name = "cursor:" + filepath.Base(filepath.Dir(path))
	} else {
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		skill.Name = "cursor:" + base
	}
	skill.Tags = appendUnique(skill.Tags, "cursor")
	return skill, nil
}

// HermesAdapter handles Hermes-format skills (hermes.json + body.md inside a
// directory). The minimal recognition criterion is a hermes.json sibling; the
// actual body text comes from body.md if present, otherwise the JSON's
// description.
type HermesAdapter struct{ md MarkdownAdapter }

// NewHermesAdapter returns the Hermes adapter.
func NewHermesAdapter() HermesAdapter { return HermesAdapter{} }

// Name implements Adapter.
func (HermesAdapter) Name() string { return "hermes" }

// CanHandle implements Adapter.
func (HermesAdapter) CanHandle(path string) bool {
	return strings.EqualFold(filepath.Base(path), "hermes.json")
}

// Parse implements Adapter. It does not invoke the markdown adapter because
// the source is JSON; instead it constructs the skill directly from the
// declared fields.
func (HermesAdapter) Parse(path string, data []byte, tier contracts.SkillTier) (contracts.Skill, error) {
	var spec struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
		Body        string   `json:"body"`
	}
	if err := jsonUnmarshal(data, &spec); err != nil {
		return contracts.Skill{}, err
	}
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		name = filepath.Base(filepath.Dir(path))
	}
	skill := contracts.Skill{
		Name:        name,
		Tier:        tier,
		Description: strings.TrimSpace(spec.Description),
		Tags:        appendUnique(normaliseTags(spec.Tags), "hermes"),
		Path:        path,
		Body:        strings.TrimSpace(spec.Body),
	}
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
