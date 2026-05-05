package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestSkillMDAdapterUsesParentDir(t *testing.T) {
	dir := t.TempDir()
	pdir := filepath.Join(dir, "code-review")
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "Some review skill content"
	path := filepath.Join(pdir, "SKILL.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	a := NewSkillMDAdapter()
	if !a.CanHandle(path) {
		t.Fatal("CanHandle should accept SKILL.md")
	}
	data, _ := os.ReadFile(path)
	skill, err := a.Parse(path, data, contracts.SkillTierImported)
	if err != nil {
		t.Fatal(err)
	}
	if skill.Name != "code-review" {
		t.Fatalf("want code-review, got %q", skill.Name)
	}
}

func TestSoulMDTagsOpenClaw(t *testing.T) {
	dir := t.TempDir()
	pdir := filepath.Join(dir, "rev")
	_ = os.MkdirAll(pdir, 0o755)
	path := filepath.Join(pdir, "SOUL.md")
	_ = os.WriteFile(path, []byte("body"), 0o644)
	a := NewSoulMDAdapter()
	data, _ := os.ReadFile(path)
	skill, err := a.Parse(path, data, contracts.SkillTierImported)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tag := range skill.Tags {
		if tag == "openclaw" {
			found = true
		}
	}
	if !found {
		t.Fatalf("openclaw tag missing: %+v", skill.Tags)
	}
}

func TestCursorRulesAdapter(t *testing.T) {
	dir := t.TempDir()
	a := NewCursorRulesAdapter()
	// .cursorrules at project root.
	pdir := filepath.Join(dir, "myproj")
	_ = os.MkdirAll(pdir, 0o755)
	rules := filepath.Join(pdir, ".cursorrules")
	_ = os.WriteFile(rules, []byte("be polite"), 0o644)
	if !a.CanHandle(rules) {
		t.Fatal("should handle .cursorrules")
	}
	// .cursor/rules/foo.md.
	rulesDir := filepath.Join(pdir, ".cursor", "rules")
	_ = os.MkdirAll(rulesDir, 0o755)
	rule := filepath.Join(rulesDir, "style.md")
	_ = os.WriteFile(rule, []byte("style rules"), 0o644)
	if !a.CanHandle(rule) {
		t.Fatal("should handle .cursor/rules/*.md")
	}
	data, _ := os.ReadFile(rule)
	skill, err := a.Parse(rule, data, contracts.SkillTierImported)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(skill.Name, "cursor:") {
		t.Fatalf("expected cursor: prefix, got %q", skill.Name)
	}
}

func TestHermesAdapter(t *testing.T) {
	dir := t.TempDir()
	pdir := filepath.Join(dir, "tool")
	_ = os.MkdirAll(pdir, 0o755)
	path := filepath.Join(pdir, "hermes.json")
	_ = os.WriteFile(path, []byte(`{"name":"deploy","description":"d","body":"do it"}`), 0o644)
	a := NewHermesAdapter()
	if !a.CanHandle(path) {
		t.Fatal("should handle hermes.json")
	}
	data, _ := os.ReadFile(path)
	skill, err := a.Parse(path, data, contracts.SkillTierImported)
	if err != nil {
		t.Fatal(err)
	}
	if skill.Name != "deploy" || skill.Body != "do it" {
		t.Fatalf("unexpected: %+v", skill)
	}
}

func TestImportDeduplicates(t *testing.T) {
	src := t.TempDir()
	tgt := t.TempDir()
	// Two SKILL.md files with the same parent name -> would clash; vary names.
	a := filepath.Join(src, "a")
	_ = os.MkdirAll(a, 0o755)
	_ = os.WriteFile(filepath.Join(a, "SKILL.md"), []byte("alpha"), 0o644)
	b := filepath.Join(src, "b")
	_ = os.MkdirAll(b, 0o755)
	_ = os.WriteFile(filepath.Join(b, "SKILL.md"), []byte("beta"), 0o644)

	res, err := Import(src, tgt, ImportSourceAuto)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Imported) != 2 {
		t.Fatalf("want 2 imports, got %d (%v)", len(res.Imported), res.Imported)
	}
	// Re-import: every file should be skipped as already present.
	res2, err := Import(src, tgt, ImportSourceAuto)
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Imported) != 0 {
		t.Fatalf("re-import should be no-op, imported %d", len(res2.Imported))
	}
	if len(res2.Skipped) != 2 {
		t.Fatalf("re-import should skip 2, got %d", len(res2.Skipped))
	}
}

func TestImportSourceFilter(t *testing.T) {
	src := t.TempDir()
	tgt := t.TempDir()
	pdir := filepath.Join(src, "x")
	_ = os.MkdirAll(pdir, 0o755)
	_ = os.WriteFile(filepath.Join(pdir, "SOUL.md"), []byte("soul"), 0o644)
	_ = os.WriteFile(filepath.Join(pdir, "SKILL.md"), []byte("skill"), 0o644)

	res, err := Import(src, tgt, ImportSourceOClaw)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Imported) != 1 || res.Imported[0].Tags == nil {
		t.Fatalf("openclaw filter wrong: %+v", res.Imported)
	}
}

func TestAdaptersForSourceUnknown(t *testing.T) {
	if _, err := AdaptersForSource("nope"); err == nil {
		t.Fatal("expected error for unknown source")
	}
}
