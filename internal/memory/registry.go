package memory

import (
	"context"
	"errors"
	"sync"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// ErrProviderAlreadyActive is returned by Register when a provider is already
// registered. Call Replace to swap providers with a graceful shutdown.
var ErrProviderAlreadyActive = errors.New("memory: an external provider is already active; call Replace to swap providers")

// Registry holds the active Provider and enforces the single-provider
// constraint from PRD §6.4 ("only one external provider may be active at a time").
type Registry struct {
	mu     sync.RWMutex
	active Provider
	kind   contracts.MemoryProviderKind
}

// Register activates provider as the sole external provider.
// Returns ErrProviderAlreadyActive if one is already registered.
func (r *Registry) Register(kind contracts.MemoryProviderKind, provider Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active != nil {
		return ErrProviderAlreadyActive
	}
	r.active = provider
	r.kind = kind
	return nil
}

// Replace shuts down the current provider (if any) and activates the new one.
func (r *Registry) Replace(ctx context.Context, kind contracts.MemoryProviderKind, provider Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active != nil {
		if err := r.active.Shutdown(ctx); err != nil {
			return err
		}
	}
	r.active = provider
	r.kind = kind
	return nil
}

// Active returns the current provider and its kind. Provider is nil if none
// has been registered.
func (r *Registry) Active() (Provider, contracts.MemoryProviderKind) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active, r.kind
}

// Shutdown shuts down the active provider and clears the registry.
func (r *Registry) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active == nil {
		return nil
	}
	err := r.active.Shutdown(ctx)
	r.active = nil
	r.kind = ""
	return err
}
