package skills

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func writeSkill(t *testing.T, dir, filename, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", dir, err)
	}
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
	return path
}

func newTestRegistry(t *testing.T, base string) (*Registry, map[contracts.SkillTier]string) {
	t.Helper()
	roots := map[contracts.SkillTier]string{
		contracts.SkillTierWorkspace: filepath.Join(base, "workspace"),
		contracts.SkillTierPersonal:  filepath.Join(base, "personal"),
		contracts.SkillTierImported:  filepath.Join(base, "imported"),
		contracts.SkillTierBundled:   filepath.Join(base, "bundled"),
	}
	return NewRegistry(roots), roots
}

func TestLoadCrossTierPrecedence(t *testing.T) {
	dir := t.TempDir()
	reg, roots := newTestRegistry(t, dir)

	wsPath := writeSkill(t, roots[contracts.SkillTierWorkspace], "lint.md", "---\nname: lint\ndescription: ws\n---\nworkspace body\n")
	personalPath := writeSkill(t, roots[contracts.SkillTierPersonal], "lint.md", "---\nname: lint\ndescription: personal\n---\npersonal body\n")
	importedPath := writeSkill(t, roots[contracts.SkillTierImported], "lint.md", "---\nname: lint\ndescription: imported\n---\nimported body\n")
	bundledPath := writeSkill(t, roots[contracts.SkillTierBundled], "lint.md", "---\nname: lint\ndescription: bundled\n---\nbundled body\n")

	if err := reg.Load([]Adapter{NewMarkdownAdapter()}); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, ok := reg.Lookup("lint")
	if !ok {
		t.Fatalf("expected lint to be loaded")
	}
	if got.Tier != contracts.SkillTierWorkspace {
		t.Errorf("expected workspace winner, got tier %q", got.Tier)
	}
	if got.Path != wsPath {
		t.Errorf("expected winner path %q, got %q", wsPath, got.Path)
	}

	conflicts := reg.Conflicts()
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	losers := conflicts[0].Losers
	sort.Strings(losers)
	wantLosers := []string{bundledPath, importedPath, personalPath}
	sort.Strings(wantLosers)
	if !reflect.DeepEqual(losers, wantLosers) {
		t.Errorf("losers mismatch: got %v want %v", losers, wantLosers)
	}
}

func TestLoadWithinTierFirstLoadedWins(t *testing.T) {
	dir := t.TempDir()
	reg, roots := newTestRegistry(t, dir)

	// Filenames sort: a.md before z.md, so a.md wins.
	winnerPath := writeSkill(t, roots[contracts.SkillTierPersonal], "a.md", "---\nname: rename\n---\nfirst\n")
	loserPath := writeSkill(t, roots[contracts.SkillTierPersonal], "z.md", "---\nname: rename\n---\nsecond\n")

	if err := reg.Load([]Adapter{NewMarkdownAdapter()}); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, ok := reg.Lookup("rename")
	if !ok {
		t.Fatalf("expected rename to load")
	}
	if got.Path != winnerPath {
		t.Errorf("first-loaded should win: got %q want %q", got.Path, winnerPath)
	}
	conflicts := reg.Conflicts()
	if len(conflicts) != 1 || conflicts[0].Winner != winnerPath {
		t.Fatalf("expected 1 same-tier conflict for winner %q, got %+v", winnerPath, conflicts)
	}
	if len(conflicts[0].Losers) != 1 || conflicts[0].Losers[0] != loserPath {
		t.Errorf("expected loser %q, got %v", loserPath, conflicts[0].Losers)
	}
}

func TestSearchCaseInsensitiveDeterministic(t *testing.T) {
	dir := t.TempDir()
	reg, roots := newTestRegistry(t, dir)

	writeSkill(t, roots[contracts.SkillTierPersonal], "alpha.md", "---\nname: Alpha\ndescription: Refactor TypeScript\ntags: [ts, refactor]\n---\nbody\n")
	writeSkill(t, roots[contracts.SkillTierPersonal], "beta.md", "---\nname: Beta\ndescription: Run benchmarks\ntags: [perf]\n---\nbody\n")
	writeSkill(t, roots[contracts.SkillTierPersonal], "gamma.md", "---\nname: Gamma\ndescription: TypeScript codemod\ntags: [ts]\n---\nbody\n")

	if err := reg.Load([]Adapter{NewMarkdownAdapter()}); err != nil {
		t.Fatalf("Load: %v", err)
	}

	results := reg.Search("typescript")
	if len(results) != 2 {
		t.Fatalf("expected 2 typescript matches, got %d (%v)", len(results), results)
	}
	if results[0].Name != "Alpha" || results[1].Name != "Gamma" {
		t.Errorf("expected alphabetical order [Alpha, Gamma], got [%s, %s]", results[0].Name, results[1].Name)
	}

	if got := reg.Search("PERF"); len(got) != 1 || got[0].Name != "Beta" {
		t.Errorf("expected case-insensitive tag match for PERF, got %v", got)
	}
}

func TestLoadToleratesMissingTierDirectories(t *testing.T) {
	dir := t.TempDir()
	roots := map[contracts.SkillTier]string{
		contracts.SkillTierWorkspace: filepath.Join(dir, "missing-workspace"),
		contracts.SkillTierPersonal:  filepath.Join(dir, "personal"),
	}
	reg := NewRegistry(roots)
	writeSkill(t, roots[contracts.SkillTierPersonal], "lone.md", "---\nname: lone\n---\nbody\n")

	if err := reg.Load([]Adapter{NewMarkdownAdapter()}); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := reg.Lookup("lone"); !ok {
		t.Fatalf("expected lone skill loaded despite missing workspace dir")
	}
}

func TestLookupMissReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	reg, _ := newTestRegistry(t, dir)
	if err := reg.Load([]Adapter{NewMarkdownAdapter()}); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if skill, ok := reg.Lookup("does-not-exist"); ok {
		t.Errorf("expected miss, got %+v", skill)
	}
}

func TestLoadRequiresAdapter(t *testing.T) {
	dir := t.TempDir()
	reg, _ := newTestRegistry(t, dir)
	if err := reg.Load(nil); err == nil {
		t.Fatalf("expected error when no adapters supplied")
	}
}

func TestListIsAlphabetical(t *testing.T) {
	dir := t.TempDir()
	reg, roots := newTestRegistry(t, dir)
	writeSkill(t, roots[contracts.SkillTierPersonal], "z.md", "---\nname: zoo\n---\nz\n")
	writeSkill(t, roots[contracts.SkillTierPersonal], "a.md", "---\nname: ant\n---\na\n")
	writeSkill(t, roots[contracts.SkillTierPersonal], "m.md", "---\nname: mole\n---\nm\n")

	if err := reg.Load([]Adapter{NewMarkdownAdapter()}); err != nil {
		t.Fatalf("Load: %v", err)
	}
	listed := reg.List()
	if len(listed) != 3 {
		t.Fatalf("expected 3 skills, got %d", len(listed))
	}
	want := []string{"ant", "mole", "zoo"}
	for i, name := range want {
		if listed[i].Name != name {
			t.Errorf("position %d: got %q want %q", i, listed[i].Name, name)
		}
	}
}
