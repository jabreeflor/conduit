package core

import (
	"errors"

	"github.com/jabreeflor/conduit/internal/contracts"
)

var ErrNoRuntimeAvailable = errors.New("no sandbox runtime is available on this host")

// RuntimeProbe detects whether a backend is present and usable on this host.
type RuntimeProbe interface {
	Probe() contracts.SandboxRuntimeCapabilities
}

// RuntimeSelector picks the best available backend in preference order.
// Call Select to get capabilities for the first available backend.
type RuntimeSelector struct {
	preferences []contracts.SandboxBackend
	probes      map[contracts.SandboxBackend]RuntimeProbe
}

// DefaultRuntimePreferences returns the PRD §15.9 backend priority order:
// Apple Virtualization.framework first (no Docker dependency), then OCI
// containers as a fallback for users who already have Docker installed.
func DefaultRuntimePreferences() []contracts.SandboxBackend {
	return []contracts.SandboxBackend{
		contracts.SandboxBackendAppleVirtualization,
		contracts.SandboxBackendOCIContainer,
	}
}

// NewRuntimeSelector creates a selector with explicit probes for each backend.
// preferences controls the order in which backends are tried.
func NewRuntimeSelector(preferences []contracts.SandboxBackend, probes map[contracts.SandboxBackend]RuntimeProbe) *RuntimeSelector {
	return &RuntimeSelector{
		preferences: preferences,
		probes:      probes,
	}
}

// Select probes each backend in preference order and returns the capabilities
// of the first available runtime, or ErrNoRuntimeAvailable if none succeed.
// Backends with no registered probe are skipped silently — absence of a probe
// means the backend was never installed, not that it failed a health check.
func (s *RuntimeSelector) Select() (contracts.SandboxRuntimeCapabilities, error) {
	for _, backend := range s.preferences {
		probe, ok := s.probes[backend]
		if !ok {
			continue
		}
		if caps := probe.Probe(); caps.Available {
			return caps, nil
		}
	}
	return contracts.SandboxRuntimeCapabilities{}, ErrNoRuntimeAvailable
}
