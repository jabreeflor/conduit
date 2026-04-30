// Package memory defines the pluggable MemoryProvider interface and ships the
// bundled FlatFileProvider (default), NoOpProvider (tests), and a Registry that
// enforces the single-active-provider constraint described in PRD §6.4.
package memory

import (
	"context"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// MemoryProvider is the interface the engine uses to store and retrieve agent
// memory. All implementations must be safe for concurrent use.
type MemoryProvider interface {
	// Initialize prepares the provider for use. Called once at engine startup.
	Initialize(ctx context.Context, cfg contracts.MemoryConfig) error

	// Prefetch warms the provider's cache with entries relevant to sessionID.
	Prefetch(ctx context.Context, sessionID string) ([]contracts.MemoryEntry, error)

	// Write persists a new memory entry.
	Write(ctx context.Context, entry contracts.MemoryEntry) error

	// Search returns entries matching query, capped at limit results.
	// A limit of 0 applies the provider's own default.
	Search(ctx context.Context, query string, limit int) ([]contracts.MemoryEntry, error)

	// Compress condenses the in-flight context window into a summary and
	// archives the source entries. Returns nil when there is nothing to compress.
	Compress(ctx context.Context) (*contracts.CompressedContext, error)

	// Shutdown flushes pending writes and releases resources.
	Shutdown(ctx context.Context) error
}

// MemoryHooks is an optional lifecycle extension a provider may implement.
// The engine checks for this interface after Initialize and calls hooks when present.
type MemoryHooks interface {
	OnTurnStart(ctx context.Context, turnID string) error
	OnSessionEnd(ctx context.Context, sessionID string) error
	OnPreCompress(ctx context.Context) error
	OnMemoryWrite(ctx context.Context, entry contracts.MemoryEntry) error
}
