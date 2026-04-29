package core

import (
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestEngineInfoIncludesSurfaceContracts(t *testing.T) {
	engine := New("test")

	info := engine.Info()

	if info.Name != "Conduit" {
		t.Fatalf("Name = %q, want Conduit", info.Name)
	}
	if info.Version != "test" {
		t.Fatalf("Version = %q, want test", info.Version)
	}
	if len(info.Surfaces) != 3 {
		t.Fatalf("len(Surfaces) = %d, want 3", len(info.Surfaces))
	}
	if info.Surfaces[0] != contracts.SurfaceTUI {
		t.Fatalf("first surface = %q, want %q", info.Surfaces[0], contracts.SurfaceTUI)
	}
	if info.StartedAt.IsZero() {
		t.Fatal("StartedAt is zero")
	}
}

func TestEngineInfoReturnsSurfaceCopy(t *testing.T) {
	engine := New("test")

	info := engine.Info()
	info.Surfaces[0] = contracts.SurfaceGUI

	next := engine.Info()
	if next.Surfaces[0] != contracts.SurfaceTUI {
		t.Fatalf("surface slice was mutated through Info")
	}
}
