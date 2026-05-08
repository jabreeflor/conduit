// Package server exposes Conduit's coding agent over HTTP+WebSocket.
//
// The server is the backend for the Conduit GUI: a single binary that
// hosts a JSON-over-WebSocket agent endpoint plus a small REST surface
// for read-only views (info, sessions, memory). Wire protocol is
// documented in agent.go and views.go; the contract is locked because
// the frontend is built against the exact event shapes here.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jabreeflor/conduit/internal/coding"
	"github.com/jabreeflor/conduit/internal/tools"
)

// StreamerFactory produces a coding.Streamer wired to the given tool
// slice. It returns the streamer and the human-readable provider/model
// names so /api/info and the per-connection "session" frame can carry
// them. The factory is called once per WebSocket connection so each
// connection gets its own streamer (and thus its own conversation
// history).
type StreamerFactory func(codingTools []tools.Tool) (coding.Streamer, string, string)

// Server wires the coding agent factory and read-only data sources into
// HTTP routes. The zero value is not usable; construct via New.
type Server struct {
	factory     StreamerFactory
	baseTools   []tools.Tool
	provider    string
	model       string
	version     string
	homeDir     string
	sessionsDir string
}

// Config bundles the values the server needs to render /api/info and
// emit accurate "session" frames. Provider and Model describe the
// streamer the factory will build; SessionsDir/HomeDir let the REST
// handlers locate journals and memory files (override for tests).
type Config struct {
	Factory     StreamerFactory
	BaseTools   []tools.Tool
	Provider    string
	Model       string
	Version     string
	HomeDir     string
	SessionsDir string
}

// New returns a Server ready to serve the documented routes.
func New(cfg Config) *Server {
	home := cfg.HomeDir
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	sessions := cfg.SessionsDir
	if sessions == "" && home != "" {
		sessions = filepath.Join(home, ".conduit", "coding-sessions")
	}
	return &Server{
		factory:     cfg.Factory,
		baseTools:   cfg.BaseTools,
		provider:    cfg.Provider,
		model:       cfg.Model,
		version:     cfg.Version,
		homeDir:     home,
		sessionsDir: sessions,
	}
}

// Handler returns the HTTP mux with all routes wired up. Exposed
// separately so tests can mount it on httptest without binding a port.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent", s.handleAgent)
	mux.HandleFunc("/api/info", s.handleInfo)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/projects", s.handleProjects)
	mux.HandleFunc("/api/memory", s.handleMemory)
	return withCORS(mux)
}

// withCORS lets the local Tauri/vite dev frontend (different origin) call
// the REST endpoints. The server only ever binds to localhost so this is
// not a real-world cross-site exposure.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Serve binds to addr and runs the HTTP server until ctx is cancelled.
// Cancellation triggers a 5 s graceful shutdown; if the listener fails
// to bind, the error is returned immediately.
func (s *Server) Serve(ctx context.Context, addr string) error {
	hs := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("server: listen %s: %w", addr, err)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- hs.Serve(ln) }()
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hs.Shutdown(shutCtx)
		return nil
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

// writeJSON is the shared response helper for the REST views.
func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
