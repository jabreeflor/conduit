package core

import (
	"context"
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
	"github.com/jabreeflor/conduit/internal/memory"
)

// TestEngineDeleteMemory exercises the inspector's delete path end-to-end
// against the real FlatFileProvider.
func TestEngineDeleteMemory(t *testing.T) {
	engine := New("test")
	provider := memory.NewFlatFileProviderAt(t.TempDir())
	if err := provider.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := engine.memRegistry.Replace(context.Background(), contracts.MemoryProviderKindFlatFile, provider); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	entry := memory.Entry{Kind: memory.KindFact, Title: "doomed", Body: "x"}
	if err := engine.WriteMemory(context.Background(), entry); err != nil {
		t.Fatalf("WriteMemory: %v", err)
	}
	stored, _ := engine.SearchMemory(context.Background(), "")
	if len(stored) != 1 {
		t.Fatalf("expected 1 entry pre-delete, got %d", len(stored))
	}

	if err := engine.DeleteMemory(context.Background(), stored[0].ID); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}
	left, _ := engine.SearchMemory(context.Background(), "")
	if len(left) != 0 {
		t.Fatalf("expected 0 entries post-delete, got %d", len(left))
	}

	found := false
	for _, ev := range engine.SessionLog() {
		if strings.Contains(ev.Message, "memory delete:") {
			found = true
		}
	}
	if !found {
		t.Fatal("session log missing memory delete event")
	}
}

// TestEnginePruneMemorySkipsPinned proves the engine wires Provider.Prune
// honoring the pinned/manual-override semantics.
func TestEnginePruneMemorySkipsPinned(t *testing.T) {
	engine := New("test")
	provider := memory.NewFlatFileProviderAt(t.TempDir())
	provider.Initialize(context.Background())                                                        //nolint:errcheck
	engine.memRegistry.Replace(context.Background(), contracts.MemoryProviderKindFlatFile, provider) //nolint:errcheck

	pinned := memory.Entry{Title: "keeper", Body: "load-bearing", Pinned: true}
	loose := memory.Entry{Title: "loose", Body: "go away"}
	engine.WriteMemory(context.Background(), pinned) //nolint:errcheck
	engine.WriteMemory(context.Background(), loose)  //nolint:errcheck

	removed, err := engine.PruneMemory(context.Background(), "")
	if err != nil {
		t.Fatalf("PruneMemory: %v", err)
	}
	if len(removed) != 1 {
		t.Fatalf("expected 1 removed (pinned skipped), got %d", len(removed))
	}

	all, _ := engine.SearchMemory(context.Background(), "")
	if len(all) != 1 || !all[0].Pinned {
		t.Fatalf("pinned entry should survive prune, got %+v", all)
	}
}

// TestEngineSetMemoryPinnedToggles checks both directions of the pin toggle.
func TestEngineSetMemoryPinnedToggles(t *testing.T) {
	engine := New("test")
	provider := memory.NewFlatFileProviderAt(t.TempDir())
	provider.Initialize(context.Background())                                                        //nolint:errcheck
	engine.memRegistry.Replace(context.Background(), contracts.MemoryProviderKindFlatFile, provider) //nolint:errcheck

	engine.WriteMemory(context.Background(), memory.Entry{Title: "candidate", Body: "y"}) //nolint:errcheck
	stored, _ := engine.SearchMemory(context.Background(), "")
	id := stored[0].ID

	if err := engine.SetMemoryPinned(context.Background(), id, true); err != nil {
		t.Fatalf("SetMemoryPinned(true): %v", err)
	}
	got, _ := engine.SearchMemory(context.Background(), "")
	if !got[0].Pinned {
		t.Fatal("entry not pinned after SetMemoryPinned(true)")
	}

	if err := engine.SetMemoryPinned(context.Background(), id, false); err != nil {
		t.Fatalf("SetMemoryPinned(false): %v", err)
	}
	got, _ = engine.SearchMemory(context.Background(), "")
	if got[0].Pinned {
		t.Fatal("entry still pinned after SetMemoryPinned(false)")
	}
}
