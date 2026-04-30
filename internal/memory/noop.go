package memory

import (
	"context"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// NoOpProvider is a do-nothing MemoryProvider used in tests or when memory
// persistence is explicitly disabled.
type NoOpProvider struct{}

func (NoOpProvider) Initialize(_ context.Context, _ contracts.MemoryConfig) error { return nil }
func (NoOpProvider) Prefetch(_ context.Context, _ string) ([]contracts.MemoryEntry, error) {
	return nil, nil
}
func (NoOpProvider) Write(_ context.Context, _ contracts.MemoryEntry) error { return nil }
func (NoOpProvider) Search(_ context.Context, _ string, _ int) ([]contracts.MemoryEntry, error) {
	return nil, nil
}
func (NoOpProvider) Compress(_ context.Context) (*contracts.CompressedContext, error) {
	return nil, nil
}
func (NoOpProvider) Shutdown(_ context.Context) error { return nil }
