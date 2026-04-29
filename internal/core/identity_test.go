package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIdentitySnapshotSystemPromptInjectsSoulLast(t *testing.T) {
	snapshot := IdentitySnapshot{
		Soul: "Prefer clarity over cleverness.",
		User: "User prefers concise responses.",
		ShortTerm: []MemoryEntry{{
			ID:    "current-task",
			Kind:  MemoryKindShortTerm,
			Title: "Current task",
			Body:  "Implement the identity stack.",
		}},
	}

	prompt := snapshot.SystemPrompt()

	memoryIndex := strings.Index(prompt, "## Memory")
	userIndex := strings.Index(prompt, "## USER.md")
	soulIndex := strings.Index(prompt, "## SOUL.md")
	if memoryIndex == -1 || userIndex == -1 || soulIndex == -1 {
		t.Fatalf("prompt missing expected layers:\n%s", prompt)
	}
	if !(memoryIndex < userIndex && userIndex < soulIndex) {
		t.Fatalf("identity layers are not ordered memory -> USER.md -> SOUL.md:\n%s", prompt)
	}
}

func TestIdentityManagerLoadsStaticFilesAndFlatMemory(t *testing.T) {
	manager := newTestIdentityManager(t)
	manager.now = fixedClock("2026-04-29T14:30:00Z")
	writeFile(t, manager.config.SoulPath, "# Soul\n\nAsk before publishing.")
	writeFile(t, manager.config.UserPath, "# User\n\nKeep responses tight.")

	longTerm, err := manager.WriteMemory(MemoryKindLongTermEpisodic, "Router decision", "Use provider fallbacks.")
	if err != nil {
		t.Fatalf("WriteMemory(long-term) returned error: %v", err)
	}
	manager.now = fixedClock("2026-04-29T14:31:00Z")
	skill, err := manager.WriteMemory(MemoryKindSkill, "Release checklist", "Run tests before PR.")
	if err != nil {
		t.Fatalf("WriteMemory(skill) returned error: %v", err)
	}

	snapshot, err := manager.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if snapshot.Soul != "# Soul\n\nAsk before publishing." {
		t.Fatalf("Soul = %q", snapshot.Soul)
	}
	if snapshot.User != "# User\n\nKeep responses tight." {
		t.Fatalf("User = %q", snapshot.User)
	}
	if len(snapshot.LongTerm) != 1 || snapshot.LongTerm[0].Title != "Router decision" {
		t.Fatalf("LongTerm = %#v", snapshot.LongTerm)
	}
	if len(snapshot.Skill) != 1 || snapshot.Skill[0].Title != "Release checklist" {
		t.Fatalf("Skill = %#v", snapshot.Skill)
	}
	if filepath.Dir(longTerm.Path) != manager.config.MemoryDir || filepath.Dir(skill.Path) != manager.config.MemoryDir {
		t.Fatalf("memory files were not written flat under %s", manager.config.MemoryDir)
	}
}

func TestIdentityManagerKeepsShortTermMemoryInSessionOnly(t *testing.T) {
	manager := newTestIdentityManager(t)

	manager.RememberShortTerm("Active branch", "codex/identity")
	snapshot, err := manager.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(snapshot.ShortTerm) != 1 {
		t.Fatalf("len(ShortTerm) = %d, want 1", len(snapshot.ShortTerm))
	}

	files, err := os.ReadDir(manager.config.MemoryDir)
	if err != nil {
		t.Fatalf("ReadDir returned error: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("short-term memory was written to disk: %#v", files)
	}

	manager.ClearShortTerm()
	snapshot, err = manager.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(snapshot.ShortTerm) != 0 {
		t.Fatalf("len(ShortTerm) after clear = %d, want 0", len(snapshot.ShortTerm))
	}
}

func TestEngineOwnsIdentityManager(t *testing.T) {
	engine := New("test")

	if engine.Identity() == nil {
		t.Fatal("Identity() returned nil")
	}
	if engine.Identity().Config().SoulPath == "" {
		t.Fatal("identity config did not resolve SOUL.md path")
	}
}

func newTestIdentityManager(t *testing.T) *IdentityManager {
	t.Helper()

	home := t.TempDir()
	manager := NewIdentityManager(IdentityConfig{HomeDir: home})
	return manager
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}

func fixedClock(value string) func() time.Time {
	return func() time.Time {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			panic(err)
		}
		return parsed
	}
}
