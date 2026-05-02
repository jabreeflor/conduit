package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDelete_removesEntry verifies single-entry deletion is idempotent and
// actually removes the underlying markdown file.
func TestDelete_removesEntry(t *testing.T) {
	p := NewFlatFileProviderAt(t.TempDir())
	if err := p.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	entry := Entry{ID: "abc123def456ffaa1234", Kind: KindFact, Title: "doomed", Body: "x"}
	if err := p.Write(context.Background(), entry); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := p.Delete(context.Background(), entry.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(p.dir, "*.md"))
	if len(files) != 0 {
		t.Fatalf("expected 0 files after delete, got %d", len(files))
	}

	// Idempotent — second delete must succeed.
	if err := p.Delete(context.Background(), entry.ID); err != nil {
		t.Fatalf("second Delete should be idempotent, got %v", err)
	}
}

// TestPrune_skipsPinned proves manual override semantics: pinned entries
// survive a prune even when they match the filter.
func TestPrune_skipsPinned(t *testing.T) {
	p := NewFlatFileProviderAt(t.TempDir())
	p.Initialize(context.Background()) //nolint:errcheck

	pinned := Entry{ID: "111aa222bb33cc44dd55", Title: "pinned note", Body: "keep me", Pinned: true}
	loose := Entry{ID: "999aa888bb77cc66dd55", Title: "loose note", Body: "drop me"}
	p.Write(context.Background(), pinned) //nolint:errcheck
	p.Write(context.Background(), loose)  //nolint:errcheck

	removed, err := p.Prune(context.Background(), "")
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if len(removed) != 1 || removed[0] != loose.ID {
		t.Fatalf("expected to prune only the loose entry, got %v", removed)
	}

	all, _ := p.Search(context.Background(), "")
	if len(all) != 1 || all[0].ID != pinned.ID {
		t.Fatalf("pinned entry must survive prune, got %+v", all)
	}
}

// TestPrune_byQuery prunes only entries matching the query.
func TestPrune_byQuery(t *testing.T) {
	p := NewFlatFileProviderAt(t.TempDir())
	p.Initialize(context.Background()) //nolint:errcheck

	p.Write(context.Background(), Entry{Title: "alpha thing", Body: "foo"})  //nolint:errcheck
	p.Write(context.Background(), Entry{Title: "beta thing", Body: "alpha"}) //nolint:errcheck
	p.Write(context.Background(), Entry{Title: "gamma", Body: "unrelated"})  //nolint:errcheck

	removed, err := p.Prune(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if len(removed) != 2 {
		t.Fatalf("expected 2 removals (title and body matches), got %d (%v)", len(removed), removed)
	}

	remaining, _ := p.Search(context.Background(), "")
	if len(remaining) != 1 || remaining[0].Title != "gamma" {
		t.Fatalf("expected only gamma to remain, got %+v", remaining)
	}
}

// TestPinnedRoundTripsThroughFile ensures the pinned flag survives a
// write→read cycle via the YAML frontmatter.
func TestPinnedRoundTripsThroughFile(t *testing.T) {
	p := NewFlatFileProviderAt(t.TempDir())
	p.Initialize(context.Background()) //nolint:errcheck

	entry := Entry{Title: "Constitution", Body: "do not delete", Pinned: true}
	if err := p.Write(context.Background(), entry); err != nil {
		t.Fatalf("Write: %v", err)
	}
	results, err := p.Search(context.Background(), "Constitution")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search returned %d, want 1", len(results))
	}
	if !results[0].Pinned {
		t.Fatalf("Pinned flag did not round-trip through frontmatter")
	}

	// Default (false) entries must not emit a pinned line — keeps existing
	// fixtures byte-stable.
	plain := Entry{Title: "Ordinary"}
	p.Write(context.Background(), plain) //nolint:errcheck
	files, _ := filepath.Glob(filepath.Join(p.dir, "ordinary-*.md"))
	if len(files) != 1 {
		t.Fatalf("expected 1 ordinary file, got %d", len(files))
	}
	raw, _ := os.ReadFile(files[0])
	data := string(raw)
	if strings.Contains(data, "pinned:") {
		t.Fatalf("non-pinned entry should not emit pinned key, got:\n%s", data)
	}
}
