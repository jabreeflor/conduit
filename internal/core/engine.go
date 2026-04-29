// Package core contains the shared Conduit engine used by all surfaces.
package core

import (
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// Engine owns the long-lived runtime state for Conduit.
type Engine struct {
	name      string
	version   string
	startedAt time.Time
	surfaces  []contracts.Surface
}

// New creates a core engine instance with the surfaces planned for the
// monorepo scaffold.
func New(version string) *Engine {
	return &Engine{
		name:      "Conduit",
		version:   version,
		startedAt: time.Now().UTC(),
		surfaces: []contracts.Surface{
			contracts.SurfaceTUI,
			contracts.SurfaceGUI,
			contracts.SurfaceSpotlight,
		},
	}
}

// Info returns a stable summary that frontends can use during startup.
func (e *Engine) Info() contracts.EngineInfo {
	return contracts.EngineInfo{
		Name:      e.name,
		Version:   e.version,
		Surfaces:  append([]contracts.Surface(nil), e.surfaces...),
		StartedAt: e.startedAt,
	}
}
