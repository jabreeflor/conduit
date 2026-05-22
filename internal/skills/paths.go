package skills

import (
	"path/filepath"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// DefaultRoots returns the canonical filesystem layout for the four skill
// tiers. Workspace lives next to the project so a `git clone` carries its
// skills along; personal/imported/bundled live under the user's home so they
// follow the user across projects. The bundled root is a placeholder for
// shipped-with-binary skills the installer will populate later.
func DefaultRoots(home, workspace string) map[contracts.SkillTier]string {
	roots := map[contracts.SkillTier]string{}
	if workspace != "" {
		roots[contracts.SkillTierWorkspace] = filepath.Join(workspace, ".conduit", "skills")
	}
	if home != "" {
		roots[contracts.SkillTierPersonal] = filepath.Join(home, ".conduit", "skills", "personal")
		roots[contracts.SkillTierImported] = filepath.Join(home, ".conduit", "skills", "imported")
		roots[contracts.SkillTierBundled] = filepath.Join(home, ".conduit", "skills", "bundled")
	}
	return roots
}
