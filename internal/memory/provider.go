// Package memory defines the MemoryProvider interface and supporting types used
// by the Conduit engine to persist and retrieve agent knowledge across sessions.
package memory

import (
	"context"
	"time"
)

// Kind classifies a memory entry for filtering and display.
type Kind string

const (
	KindFact       Kind = "fact"
	KindDecision   Kind = "decision"
	KindPreference Kind = "preference"
	KindContext    Kind = "context"
)

// Entry is one unit of persistent agent memory.
type Entry struct {
	ID        string
	Kind      Kind
	Title     string
	Body      string
	Tags      []string
	CreatedAt time.Time
	UpdatedAt time.Time
	// Pinned marks an entry as protected from automatic pruning. The memory
	// inspector lets users toggle this manually (PRD §19 "Memory pruning /
	// manual override"); any future automatic compactor must skip pinned entries.
	Pinned bool
}

// Provider is the abstract memory layer. Only one external provider may be
// active at a time to prevent tool schema bloat (PRD §6.4).
type Provider interface {
	// Initialize prepares storage (creates dirs, opens connections).
	Initialize(ctx context.Context) error
	// Prefetch warms the in-memory cache with entries relevant to the query.
	Prefetch(ctx context.Context, query string) ([]Entry, error)
	// Write persists one entry; update semantics apply when ID matches an existing entry.
	Write(ctx context.Context, entry Entry) error
	// Search returns entries whose title, body, or tags contain the query.
	Search(ctx context.Context, query string) ([]Entry, error)
	// Delete removes the entry with the given ID. Returns nil if no entry
	// matches (idempotent). Pinned entries are still deleted — Delete is the
	// explicit user-driven path; pruners must check Pinned themselves.
	Delete(ctx context.Context, id string) error
	// Prune removes every entry matched by query (same matching rules as
	// Search) except those with Pinned=true. Returns the IDs that were removed.
	// An empty query prunes everything not pinned.
	Prune(ctx context.Context, query string) ([]string, error)
	// Compress merges or prunes stale entries to keep the store lean.
	Compress(ctx context.Context) error
	// Shutdown flushes any pending writes and releases resources.
	Shutdown(ctx context.Context) error
}

// Hooks are optional lifecycle callbacks attached to a provider. All fields are
// optional; nil functions are silently skipped.
type Hooks struct {
	OnTurnStart   func(ctx context.Context) error
	OnSessionEnd  func(ctx context.Context) error
	OnPreCompress func(ctx context.Context, entries []Entry) error
	OnMemoryWrite func(ctx context.Context, entry Entry) error
}

// HookProvider extends Provider with lifecycle hooks.
type HookProvider interface {
	Provider
	Hooks() Hooks
}
