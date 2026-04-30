package memory

import "context"

// NoOpProvider is a do-nothing Provider used in tests or when memory
// persistence is explicitly disabled.
type NoOpProvider struct{}

func (NoOpProvider) Initialize(_ context.Context) error                    { return nil }
func (NoOpProvider) Prefetch(_ context.Context, _ string) ([]Entry, error) { return nil, nil }
func (NoOpProvider) Write(_ context.Context, _ Entry) error                { return nil }
func (NoOpProvider) Search(_ context.Context, _ string) ([]Entry, error)   { return nil, nil }
func (NoOpProvider) Compress(_ context.Context) error                      { return nil }
func (NoOpProvider) Shutdown(_ context.Context) error                      { return nil }
