package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// tierPrecedence is the locked walk order. Workspace wins, bundled loses.
// Skills loaded earlier win against the same name loaded later.
var tierPrecedence = []contracts.SkillTier{
	contracts.SkillTierWorkspace,
	contracts.SkillTierPersonal,
	contracts.SkillTierImported,
	contracts.SkillTierBundled,
}

// Registry indexes user-authored skill files across the four-tier hierarchy.
// It is built once at startup (or on explicit reload) so per-task lookups stay
// in-memory; the filesystem layout is the source of truth.
type Registry struct {
	roots     map[contracts.SkillTier]string
	skills    map[string]contracts.Skill
	conflicts []contracts.SkillConflict
}

// NewRegistry constructs an empty registry pointed at the supplied per-tier
// roots. Callers compute defaults via DefaultRoots so tests and embeddings
// can override directories without env var hacks.
func NewRegistry(roots map[contracts.SkillTier]string) *Registry {
	cloned := make(map[contracts.SkillTier]string, len(roots))
	for tier, path := range roots {
		cloned[tier] = path
	}
	return &Registry{
		roots:  cloned,
		skills: map[string]contracts.Skill{},
	}
}

// Load walks each tier root in precedence order, asks the first matching
// adapter to parse each file, and resolves name collisions in place.
//
// Conflict policy: precedence by tier, first-loaded-wins within tier;
// collisions surface via Conflicts() instead of erroring so import sweeps
// don't fail.
func (r *Registry) Load(adapters []Adapter) error {
	r.skills = map[string]contracts.Skill{}
	r.conflicts = nil

	if len(adapters) == 0 {
		return errors.New("skills: at least one adapter required")
	}

	for _, tier := range tierPrecedence {
		root, ok := r.roots[tier]
		if !ok || root == "" {
			continue
		}
		paths, err := collectSkillPaths(root)
		if err != nil {
			return fmt.Errorf("skills: walk %s tier %q: %w", tier, root, err)
		}
		for _, path := range paths {
			if err := r.loadFile(tier, path, adapters); err != nil {
				// Per-file errors must not abort the sweep — surfaces will
				// later show a "broken skills" pane. For now we swallow and
				// keep going so a single bad markdown file can't hide the
				// rest of the user's library.
				continue
			}
		}
	}
	return nil
}

// Lookup returns the hierarchy-resolved skill for an exact name.
func (r *Registry) Lookup(name string) (contracts.Skill, bool) {
	skill, ok := r.skills[name]
	return skill, ok
}

// Search runs a case-insensitive substring scan over name, description, and
// tags. The first cut at "indexed semantic-ish" search is intentionally cheap;
// embeddings will plug in by exposing this as a strategy later.
func (r *Registry) Search(query string) []contracts.Skill {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return r.List()
	}

	matches := make([]contracts.Skill, 0, len(r.skills))
	for _, skill := range r.skills {
		if skillMatchesQuery(skill, query) {
			matches = append(matches, skill)
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Name < matches[j].Name })
	return matches
}

// Conflicts returns a copy of the collisions recorded during Load.
func (r *Registry) Conflicts() []contracts.SkillConflict {
	out := make([]contracts.SkillConflict, len(r.conflicts))
	copy(out, r.conflicts)
	return out
}

// List returns every loaded skill in alphabetical order. Stable ordering keeps
// the CLI/TUI output diffable across reloads.
func (r *Registry) List() []contracts.Skill {
	out := make([]contracts.Skill, 0, len(r.skills))
	for _, skill := range r.skills {
		out = append(out, skill)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *Registry) loadFile(tier contracts.SkillTier, path string, adapters []Adapter) error {
	adapter := pickAdapter(adapters, path)
	if adapter == nil {
		return fmt.Errorf("skills: no adapter handles %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("skills: read %q: %w", path, err)
	}
	skill, err := adapter.Parse(path, data, tier)
	if err != nil {
		return err
	}
	if info, statErr := os.Stat(path); statErr == nil {
		skill.UpdatedAt = info.ModTime()
	}

	existing, ok := r.skills[skill.Name]
	if !ok {
		r.skills[skill.Name] = skill
		return nil
	}
	r.recordConflict(existing, skill)
	return nil
}

// recordConflict appends the loser to an existing entry in Conflicts() or
// creates a new one. The winner is whichever skill was loaded first — by tier
// precedence across tiers, by sorted filename within a tier.
func (r *Registry) recordConflict(winner, loser contracts.Skill) {
	for i := range r.conflicts {
		if r.conflicts[i].Name == winner.Name && r.conflicts[i].Winner == winner.Path {
			r.conflicts[i].Losers = append(r.conflicts[i].Losers, loser.Path)
			return
		}
	}
	r.conflicts = append(r.conflicts, contracts.SkillConflict{
		Name:   winner.Name,
		Tier:   winner.Tier,
		Winner: winner.Path,
		Losers: []string{loser.Path},
	})
}

// collectSkillPaths walks a tier root and returns deterministic, sorted file
// paths. Missing directories are normal on first run and are not errors.
func collectSkillPaths(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	var paths []string
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Strings(paths)
	return paths, nil
}

func pickAdapter(adapters []Adapter, path string) Adapter {
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		if adapter.CanHandle(path) {
			return adapter
		}
	}
	return nil
}

func skillMatchesQuery(skill contracts.Skill, query string) bool {
	if strings.Contains(strings.ToLower(skill.Name), query) {
		return true
	}
	if strings.Contains(strings.ToLower(skill.Description), query) {
		return true
	}
	for _, tag := range skill.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}
