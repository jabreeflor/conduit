package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestFlatFileProviderWriteAndPrefetch(t *testing.T) {
	dir := t.TempDir()
	p := newTestProvider(t, dir)

	entry := contracts.MemoryEntry{
		Kind:      contracts.MemoryKindLongTermEpisodic,
		Title:     "Router decision",
		Body:      "Use provider fallbacks.",
		CreatedAt: time.Date(2026, 4, 29, 14, 30, 0, 0, time.UTC),
	}

	if err := p.Write(context.Background(), entry); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	got, err := p.Prefetch(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Prefetch returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Prefetch returned %d entries, want 1", len(got))
	}
	if got[0].Title != "Router decision" {
		t.Fatalf("Title = %q, want %q", got[0].Title, "Router decision")
	}
	if !containsSubstr(got[0].Body, "Use provider fallbacks.") {
		t.Fatalf("Body = %q, want it to contain %q", got[0].Body, "Use provider fallbacks.")
	}
}

func TestFlatFileProviderRejectsShortTermWrites(t *testing.T) {
	dir := t.TempDir()
	p := newTestProvider(t, dir)

	entry := contracts.MemoryEntry{
		Kind:  contracts.MemoryKindShortTerm,
		Title: "Active task",
		Body:  "Implement memory layer.",
	}
	if err := p.Write(context.Background(), entry); err == nil {
		t.Fatal("Write(short-term) should return an error but did not")
	}
}

func TestFlatFileProviderSearchFiltersAndRanksNewestFirst(t *testing.T) {
	dir := t.TempDir()
	p := newTestProvider(t, dir)

	writeEntry(t, p, "Router decision", "Use fallbacks.", time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC))
	writeEntry(t, p, "Release checklist", "Run tests before PR.", time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC))
	writeEntry(t, p, "Skill: slugify", "Lowercase and dash-separate.", time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC))

	results, err := p.Search(context.Background(), "router", 10)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search returned %d results, want 1; got %#v", len(results), results)
	}
	if results[0].Title != "Router decision" {
		t.Fatalf("Title = %q", results[0].Title)
	}
}

func TestFlatFileProviderSearchLimitIsRespected(t *testing.T) {
	dir := t.TempDir()
	p := newTestProvider(t, dir)

	for i := 0; i < 5; i++ {
		writeEntry(t, p, "note", "content", time.Date(2026, 4, i+1, 0, 0, 0, 0, time.UTC))
	}

	results, err := p.Search(context.Background(), "note", 3)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("Search returned %d results, want 3", len(results))
	}
}

func TestFlatFileProviderSearchEmptyQueryReturnsAll(t *testing.T) {
	dir := t.TempDir()
	p := newTestProvider(t, dir)

	writeEntry(t, p, "Alpha", "a", time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC))
	writeEntry(t, p, "Beta", "b", time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC))

	results, err := p.Search(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Search returned %d results, want 2", len(results))
	}
}

func TestFlatFileProviderWriteCreatesMarkdownWithFrontMatter(t *testing.T) {
	dir := t.TempDir()
	p := newTestProvider(t, dir)

	entry := contracts.MemoryEntry{
		Kind:      contracts.MemoryKindSkill,
		Title:     "Release checklist",
		Body:      "Run tests before PR.",
		CreatedAt: time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC),
	}
	if err := p.Write(context.Background(), entry); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir returned error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	data, err := os.ReadFile(filepath.Join(dir, files[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	contents := string(data)
	for _, want := range []string{"---", "kind: skill", "title: Release checklist", "Run tests before PR."} {
		if !containsLine(contents, want) {
			t.Fatalf("file missing %q:\n%s", want, contents)
		}
	}
}

func newTestProvider(t *testing.T, dir string) *FlatFileProvider {
	t.Helper()
	p := NewFlatFileProvider(dir)
	if err := p.Initialize(context.Background(), contracts.MemoryConfig{}); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	return p
}

func writeEntry(t *testing.T, p *FlatFileProvider, title, body string, at time.Time) {
	t.Helper()
	if err := p.Write(context.Background(), contracts.MemoryEntry{
		Kind:      contracts.MemoryKindLongTermEpisodic,
		Title:     title,
		Body:      body,
		CreatedAt: at,
	}); err != nil {
		t.Fatalf("Write(%q) returned error: %v", title, err)
	}
}

func containsLine(contents, substr string) bool {
	for _, line := range splitLines(contents) {
		if line == substr || (len(line) >= len(substr) && containsSubstr(line, substr)) {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func containsSubstr(s, sub string) bool {
	return len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}
