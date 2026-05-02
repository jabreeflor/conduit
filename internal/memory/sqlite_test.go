package memory

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestSQLite(t *testing.T) *SQLiteProvider {
	t.Helper()
	p := NewSQLiteProviderAt(filepath.Join(t.TempDir(), "memory.db"))
	if err := p.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })
	return p
}

func TestSQLite_InitializeIsIdempotent(t *testing.T) {
	p := NewSQLiteProviderAt(filepath.Join(t.TempDir(), "memory.db"))
	if err := p.Initialize(context.Background()); err != nil {
		t.Fatalf("first Initialize: %v", err)
	}
	if err := p.Initialize(context.Background()); err != nil {
		t.Fatalf("second Initialize: %v", err)
	}
	_ = p.Shutdown(context.Background())
}

func TestSQLite_WriteAndSearch(t *testing.T) {
	p := newTestSQLite(t)
	ctx := context.Background()

	entries := []Entry{
		{Kind: KindFact, Title: "Go version", Body: "Project uses Go 1.22.", Tags: []string{"build"}},
		{Kind: KindFact, Title: "Database", Body: "PostgreSQL on port 5432.", Tags: []string{"infra"}},
		{Kind: KindPreference, Title: "Code style", Body: "No trailing whitespace."},
	}
	for _, e := range entries {
		if err := p.Write(ctx, e); err != nil {
			t.Fatalf("Write %q: %v", e.Title, err)
		}
	}

	got, err := p.Search(ctx, "postgres")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Database" {
		t.Fatalf("Search(postgres) = %+v, want one Database hit", got)
	}
}

func TestSQLite_SearchEmptyReturnsAll(t *testing.T) {
	p := newTestSQLite(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := p.Write(ctx, Entry{Kind: KindFact, Title: "entry", Body: strings.Repeat("body ", i+1)}); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	got, err := p.Search(ctx, "")
	if err != nil {
		t.Fatalf("Search empty: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("Search empty = %d, want 3", len(got))
	}
}

func TestSQLite_SearchTokenizesAcrossFields(t *testing.T) {
	p := newTestSQLite(t)
	ctx := context.Background()

	_ = p.Write(ctx, Entry{Title: "rotate api keys", Body: "every 90 days", Tags: []string{"security"}})
	_ = p.Write(ctx, Entry{Title: "build cache", Body: "invalidate on lockfile change"})

	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"prefix match", "rota", "rotate api keys"},
		{"phrase across body", "90 days", "rotate api keys"},
		{"tag match", "security", "rotate api keys"},
		{"unrelated", "lockfile", "build cache"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := p.Search(ctx, tc.query)
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if len(got) != 1 || got[0].Title != tc.want {
				t.Fatalf("Search(%q) = %+v, want one %q", tc.query, got, tc.want)
			}
		})
	}
}

func TestSQLite_WriteUpsert(t *testing.T) {
	p := newTestSQLite(t)
	ctx := context.Background()

	entry := Entry{ID: "fixedid000000000000a", Kind: KindDecision, Title: "Original", Body: "v1"}
	if err := p.Write(ctx, entry); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	entry.Title = "Updated"
	entry.Body = "v2"
	if err := p.Write(ctx, entry); err != nil {
		t.Fatalf("second Write: %v", err)
	}

	got, _ := p.Search(ctx, "")
	if len(got) != 1 {
		t.Fatalf("expected 1 entry after upsert, got %d", len(got))
	}
	if got[0].Title != "Updated" || got[0].Body != "v2" {
		t.Fatalf("upsert did not replace fields: %+v", got[0])
	}

	// FTS index should reflect the new content, not the old.
	stale, _ := p.Search(ctx, "Original")
	if len(stale) != 0 {
		t.Fatalf("FTS still matches stale title: %+v", stale)
	}
	fresh, _ := p.Search(ctx, "Updated")
	if len(fresh) != 1 {
		t.Fatalf("FTS missed updated title")
	}
}

func TestSQLite_WriteAssignsID(t *testing.T) {
	p := newTestSQLite(t)
	ctx := context.Background()

	if err := p.Write(ctx, Entry{Title: "auto id", Body: "x"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, _ := p.Search(ctx, "auto")
	if len(got) != 1 || got[0].ID == "" {
		t.Fatalf("expected one entry with auto-assigned ID, got %+v", got)
	}
}

func TestSQLite_WritePreservesCreatedAt(t *testing.T) {
	p := newTestSQLite(t)
	ctx := context.Background()

	original := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	entry := Entry{
		ID:        "fixedidcreated000aaa",
		Kind:      KindDecision,
		Title:     "Architecture decision",
		Body:      "Chosen monorepo layout.",
		CreatedAt: original,
	}
	if err := p.Write(ctx, entry); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, _ := p.Search(ctx, "architecture")
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if !got[0].CreatedAt.Equal(original) {
		t.Errorf("CreatedAt = %v, want %v", got[0].CreatedAt, original)
	}
}

func TestSQLite_DeleteIsIdempotent(t *testing.T) {
	p := newTestSQLite(t)
	ctx := context.Background()

	entry := Entry{ID: "doomedid000000000aaa", Title: "doomed", Body: "x"}
	_ = p.Write(ctx, entry)

	if err := p.Delete(ctx, entry.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := p.Delete(ctx, entry.ID); err != nil {
		t.Fatalf("second Delete should be idempotent: %v", err)
	}
	got, _ := p.Search(ctx, "")
	if len(got) != 0 {
		t.Fatalf("expected empty store, got %d", len(got))
	}
}

func TestSQLite_PruneSkipsPinned(t *testing.T) {
	p := newTestSQLite(t)
	ctx := context.Background()

	pinned := Entry{ID: "pinnedid0000000000aa", Title: "pinned note", Body: "keep me", Pinned: true}
	loose := Entry{ID: "looseid00000000000aa", Title: "loose note", Body: "drop me"}
	_ = p.Write(ctx, pinned)
	_ = p.Write(ctx, loose)

	removed, err := p.Prune(ctx, "")
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if len(removed) != 1 || removed[0] != loose.ID {
		t.Fatalf("expected only loose pruned, got %v", removed)
	}

	all, _ := p.Search(ctx, "")
	if len(all) != 1 || all[0].ID != pinned.ID {
		t.Fatalf("pinned entry must survive prune, got %+v", all)
	}
}

func TestSQLite_PruneByQuery(t *testing.T) {
	p := newTestSQLite(t)
	ctx := context.Background()

	_ = p.Write(ctx, Entry{ID: "alpha000000000000aaa", Title: "alpha thing", Body: "foo"})
	_ = p.Write(ctx, Entry{ID: "beta0000000000000aaa", Title: "beta thing", Body: "alpha context"})
	_ = p.Write(ctx, Entry{ID: "gamma000000000000aaa", Title: "gamma", Body: "unrelated"})

	removed, err := p.Prune(ctx, "alpha")
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if len(removed) != 2 {
		t.Fatalf("expected 2 removals, got %d (%v)", len(removed), removed)
	}

	remaining, _ := p.Search(ctx, "")
	if len(remaining) != 1 || remaining[0].Title != "gamma" {
		t.Fatalf("expected only gamma to remain, got %+v", remaining)
	}
}

func TestSQLite_PinnedRoundTrip(t *testing.T) {
	p := newTestSQLite(t)
	ctx := context.Background()

	if err := p.Write(ctx, Entry{Title: "Constitution", Body: "do not delete", Pinned: true}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, _ := p.Search(ctx, "constitution")
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if !got[0].Pinned {
		t.Fatalf("pinned flag did not round-trip")
	}
}

func TestSQLite_TagsRoundTrip(t *testing.T) {
	p := newTestSQLite(t)
	ctx := context.Background()

	if err := p.Write(ctx, Entry{Title: "model", Body: "haiku is cheap", Tags: []string{"model", "cost"}}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, _ := p.Search(ctx, "cost")
	if len(got) != 1 {
		t.Fatalf("expected 1 tag-matched entry, got %d", len(got))
	}
	if len(got[0].Tags) != 2 {
		t.Fatalf("expected 2 tags, got %v", got[0].Tags)
	}
}

func TestSQLite_PrefetchDelegatesToSearch(t *testing.T) {
	p := newTestSQLite(t)
	ctx := context.Background()

	_ = p.Write(ctx, Entry{Title: "API key rotation", Body: "Rotate every 90 days."})

	got, err := p.Prefetch(ctx, "rotation")
	if err != nil {
		t.Fatalf("Prefetch: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
}

func TestSQLite_OperationsBeforeInitFail(t *testing.T) {
	p := NewSQLiteProviderAt(filepath.Join(t.TempDir(), "memory.db"))
	ctx := context.Background()

	if err := p.Write(ctx, Entry{Title: "x", Body: "y"}); err == nil {
		t.Fatal("Write before Initialize should fail")
	}
	if _, err := p.Search(ctx, "x"); err == nil {
		t.Fatal("Search before Initialize should fail")
	}
}

func TestSQLite_SearchSanitisesGarbageQuery(t *testing.T) {
	p := newTestSQLite(t)
	ctx := context.Background()

	_ = p.Write(ctx, Entry{Title: "valid entry", Body: "real content"})

	// FTS5 special characters in user input must not produce a syntax error.
	got, err := p.Search(ctx, `"!!!*-`)
	if err != nil {
		t.Fatalf("Search with garbage chars: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("garbage query should match nothing, got %d", len(got))
	}
}

func TestSQLite_BuildFTSMatch(t *testing.T) {
	cases := map[string]string{
		"":              "",
		"   ":           "",
		"foo":           `"foo"*`,
		"Foo Bar":       `"foo"* AND "bar"*`,
		"hello, world!": `"hello"* AND "world"*`,
		"---":           "",
	}
	for in, want := range cases {
		if got := buildFTSMatch(in); got != want {
			t.Errorf("buildFTSMatch(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSQLite_ShutdownIsIdempotent(t *testing.T) {
	p := NewSQLiteProviderAt(filepath.Join(t.TempDir(), "memory.db"))
	if err := p.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
}
