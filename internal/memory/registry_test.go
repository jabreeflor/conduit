package memory

import (
	"context"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestRegistryEnforcesSingleProvider(t *testing.T) {
	reg := &Registry{}

	if err := reg.Register(contracts.MemoryProviderKindFlatFile, NoOpProvider{}); err != nil {
		t.Fatalf("first Register returned error: %v", err)
	}
	if err := reg.Register(contracts.MemoryProviderKindSQLite, NoOpProvider{}); err == nil {
		t.Fatal("second Register should return ErrProviderAlreadyActive but did not")
	}
}

func TestRegistryReplaceSwapsProviders(t *testing.T) {
	reg := &Registry{}
	_ = reg.Register(contracts.MemoryProviderKindFlatFile, NoOpProvider{})

	if err := reg.Replace(context.Background(), contracts.MemoryProviderKindSQLite, NoOpProvider{}); err != nil {
		t.Fatalf("Replace returned error: %v", err)
	}

	_, kind := reg.Active()
	if kind != contracts.MemoryProviderKindSQLite {
		t.Fatalf("Active kind = %q, want %q", kind, contracts.MemoryProviderKindSQLite)
	}
}

func TestRegistryShutdownClearsProvider(t *testing.T) {
	reg := &Registry{}
	_ = reg.Register(contracts.MemoryProviderKindFlatFile, NoOpProvider{})

	if err := reg.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	p, _ := reg.Active()
	if p != nil {
		t.Fatal("Active() should return nil after Shutdown")
	}
}

func TestRegistryActiveReturnsNilWhenEmpty(t *testing.T) {
	reg := &Registry{}
	p, kind := reg.Active()
	if p != nil || kind != "" {
		t.Fatalf("expected (nil, \"\"), got (%v, %q)", p, kind)
	}
}
