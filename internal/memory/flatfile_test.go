package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInitialize_createsDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mem")
	p := NewFlatFileProviderAt(dir)

	if err := p.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("directory not created: %v", err)
	}
}

func TestWrite_createsMarkdownFile(t *testing.T) {
	p := NewFlatFileProviderAt(t.TempDir())
	p.Initialize(context.Background()) //nolint:errcheck

	entry := Entry{
		Kind:  KindFact,
		Title: "Preferred model",
		Body:  "Always use claude-sonnet-4-6 for long-context tasks.",
		Tags:  []string{"model", "preference"},
	}
	if err := p.Write(context.Background(), entry); err != nil {
		t.Fatalf("Write: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(p.dir, "*.md"))
	if len(files) != 1 {
		t.Fatalf("expected 1 .md file, got %d", len(files))
	}

	data, _ := os.ReadFile(files[0])
	content := string(data)
	if !strings.Contains(content, "---") {
		t.Error("file missing YAML frontmatter delimiter")
	}
	if !strings.Contains(content, "kind: fact") {
		t.Error("file missing kind field")
	}
	if !strings.Contains(content, "# Preferred model") {
		t.Error("file missing markdown heading")
	}
	if !strings.Contains(content, "claude-sonnet-4-6") {
		t.Error("file missing body content")
	}
}

func TestWrite_updateReplacesFile(t *testing.T) {
	p := NewFlatFileProviderAt(t.TempDir())
	p.Initialize(context.Background()) //nolint:errcheck

	entry := Entry{
		ID:    "abc123def456abcd1234",
		Kind:  KindDecision,
		Title: "Original title",
		Body:  "Original body.",
	}
	p.Write(context.Background(), entry) //nolint:errcheck

	entry.Title = "Updated title"
	entry.Body = "Updated body."
	if err := p.Write(context.Background(), entry); err != nil {
		t.Fatalf("second Write: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(p.dir, "*.md"))
	if len(files) != 1 {
		t.Fatalf("expected 1 .md file after update, got %d", len(files))
	}

	data, _ := os.ReadFile(files[0])
	if !strings.Contains(string(data), "Updated body") {
		t.Error("updated body not written")
	}
	if strings.Contains(string(data), "Original") {
		t.Error("old content still present after update")
	}
}

func TestWrite_assignsIDWhenEmpty(t *testing.T) {
	p := NewFlatFileProviderAt(t.TempDir())
	p.Initialize(context.Background()) //nolint:errcheck

	entry := Entry{Kind: KindContext, Title: "Session context", Body: "Working on memory layer."}
	p.Write(context.Background(), entry) //nolint:errcheck

	results, _ := p.Search(context.Background(), "Session context")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID == "" {
		t.Error("ID should be auto-generated")
	}
}

func TestSearch_matchesTitleAndBody(t *testing.T) {
	p := NewFlatFileProviderAt(t.TempDir())
	p.Initialize(context.Background()) //nolint:errcheck

	p.Write(context.Background(), Entry{Kind: KindFact, Title: "Go version", Body: "Project uses Go 1.22."})         //nolint:errcheck
	p.Write(context.Background(), Entry{Kind: KindFact, Title: "Database", Body: "PostgreSQL on port 5432."})        //nolint:errcheck
	p.Write(context.Background(), Entry{Kind: KindPreference, Title: "Code style", Body: "No trailing whitespace."}) //nolint:errcheck

	got, err := p.Search(context.Background(), "go")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// "Go version" title and "Go 1.22" body should both match.
	if len(got) < 1 {
		t.Errorf("expected ≥1 result for 'go', got %d", len(got))
	}
}

func TestSearch_emptyQueryReturnsAll(t *testing.T) {
	p := NewFlatFileProviderAt(t.TempDir())
	p.Initialize(context.Background()) //nolint:errcheck

	for i := 0; i < 3; i++ {
		p.Write(context.Background(), Entry{ //nolint:errcheck
			Kind:  KindFact,
			Title: strings.Repeat("entry", i+1),
			Body:  "body",
		})
	}

	got, err := p.Search(context.Background(), "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 entries, got %d", len(got))
	}
}

func TestSearch_matchesTags(t *testing.T) {
	p := NewFlatFileProviderAt(t.TempDir())
	p.Initialize(context.Background()) //nolint:errcheck

	p.Write(context.Background(), Entry{Kind: KindPreference, Title: "Model choice", Body: "Use Haiku for cheap tasks.", Tags: []string{"model", "cost"}}) //nolint:errcheck
	p.Write(context.Background(), Entry{Kind: KindFact, Title: "Latency note", Body: "p99 is under 200ms.", Tags: []string{"performance"}})                //nolint:errcheck

	got, _ := p.Search(context.Background(), "cost")
	if len(got) != 1 {
		t.Errorf("expected 1 tag-matched entry, got %d", len(got))
	}
}

func TestPrefetch_delegatesToSearch(t *testing.T) {
	p := NewFlatFileProviderAt(t.TempDir())
	p.Initialize(context.Background())                                                                             //nolint:errcheck
	p.Write(context.Background(), Entry{Kind: KindFact, Title: "API key rotation", Body: "Rotate every 90 days."}) //nolint:errcheck

	got, err := p.Prefetch(context.Background(), "API")
	if err != nil {
		t.Fatalf("Prefetch: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 prefetch result, got %d", len(got))
	}
}

func TestWrite_preservesCreatedAt(t *testing.T) {
	p := NewFlatFileProviderAt(t.TempDir())
	p.Initialize(context.Background()) //nolint:errcheck

	original := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	entry := Entry{
		ID:        "fixed-id-123456789012",
		Kind:      KindDecision,
		Title:     "Architecture decision",
		Body:      "Chosen monorepo layout.",
		CreatedAt: original,
	}
	p.Write(context.Background(), entry) //nolint:errcheck

	got, _ := p.Search(context.Background(), "Architecture")
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if !got[0].CreatedAt.Equal(original) {
		t.Errorf("CreatedAt = %v, want %v", got[0].CreatedAt, original)
	}
}

func TestMarshalParseRoundtrip(t *testing.T) {
	entry := Entry{
		ID:        "roundtripid12345678",
		Kind:      KindDecision,
		Title:     "Round-trip test",
		Body:      "Body with\nmultiple lines.\n",
		Tags:      []string{"test", "roundtrip"},
		CreatedAt: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2025, 6, 2, 8, 30, 0, 0, time.UTC),
	}

	raw := marshalEntry(entry)
	got, ok := parseEntry(raw)
	if !ok {
		t.Fatalf("parseEntry returned !ok\n%s", raw)
	}
	if got.ID != entry.ID {
		t.Errorf("ID = %q, want %q", got.ID, entry.ID)
	}
	if got.Kind != entry.Kind {
		t.Errorf("Kind = %q, want %q", got.Kind, entry.Kind)
	}
	if got.Title != entry.Title {
		t.Errorf("Title = %q, want %q", got.Title, entry.Title)
	}
	if len(got.Tags) != len(entry.Tags) {
		t.Errorf("Tags = %v, want %v", got.Tags, entry.Tags)
	}
	if !got.CreatedAt.Equal(entry.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, entry.CreatedAt)
	}
}

func TestCompressAndShutdown_noops(t *testing.T) {
	p := NewFlatFileProviderAt(t.TempDir())
	if err := p.Compress(context.Background()); err != nil {
		t.Errorf("Compress: %v", err)
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}

// Ensure FlatFileProvider satisfies the Provider interface at compile time.
var _ Provider = (*FlatFileProvider)(nil)
