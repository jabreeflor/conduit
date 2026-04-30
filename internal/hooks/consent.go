// Package hooks implements the consent gate for user-defined shell-subprocess
// hook scripts. It enforces first-use confirmation (PRD §6.6) and supports
// the seven Codex-aligned lifecycle events (PRD §6.25.17).
package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// ConsentConfig controls how the gate handles unapproved hook registrations.
type ConsentConfig struct {
	// StatePath is the JSON file that persists approved fingerprints.
	// Defaults to ~/.conduit/hook_consent.json.
	StatePath string
	// Interactive enables prompting the user for confirmation on first use.
	// When false, AcceptHooks must be true or registration is blocked.
	Interactive bool
	// AcceptHooks auto-approves hooks without prompting. Required for
	// non-interactive callers such as API servers and cron jobs (PRD §6.6).
	AcceptHooks bool
}

// ConfirmFunc is called in interactive mode when a hook needs first-use
// approval. Return true to approve, false to deny.
type ConfirmFunc func(event contracts.HookEvent, command string) (bool, error)

// ConsentGate enforces first-use confirmation for hook registrations.
// All methods are safe for concurrent use.
type ConsentGate struct {
	mu          sync.Mutex
	statePath   string
	interactive bool
	acceptHooks bool
	approved    map[string]struct{}
	confirm     ConfirmFunc
}

// consentState is the on-disk representation of approved fingerprints.
type consentState struct {
	Approved []string `json:"approved"`
}

const defaultStateFile = ".conduit/hook_consent.json"

// New loads persisted consent state and returns a gate ready to evaluate
// hook registrations. confirm is called for interactive callers on first use;
// it may be nil when Interactive is false.
func New(config ConsentConfig, confirm ConfirmFunc) (*ConsentGate, error) {
	statePath := config.StatePath
	if statePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("hooks: resolve home dir: %w", err)
		}
		statePath = filepath.Join(home, defaultStateFile)
	}

	approved := make(map[string]struct{})
	if data, err := os.ReadFile(statePath); err == nil {
		var state consentState
		if json.Unmarshal(data, &state) == nil {
			for _, fp := range state.Approved {
				approved[fp] = struct{}{}
			}
		}
	}

	return &ConsentGate{
		statePath:   statePath,
		interactive: config.Interactive,
		acceptHooks: config.AcceptHooks,
		approved:    approved,
		confirm:     confirm,
	}, nil
}

// RequireConsent ensures the caller has approved this hook before it runs.
//
//   - Already-approved registrations pass through immediately.
//   - Non-interactive callers without AcceptHooks receive an error; they must
//     set accept_hooks: true in config.
//   - Non-interactive callers with AcceptHooks auto-approve and persist.
//   - Interactive callers are prompted via ConfirmFunc on first use.
func (g *ConsentGate) RequireConsent(reg contracts.HookRegistration) error {
	fp := fingerprint(reg)

	g.mu.Lock()
	already := g.isApproved(fp)
	g.mu.Unlock()

	if already {
		return nil
	}

	if !g.interactive {
		if !g.acceptHooks {
			return fmt.Errorf(
				"hooks: %s hook %q requires user consent; set accept_hooks: true in config for non-interactive use",
				reg.Event, reg.Command,
			)
		}
		return g.recordApproval(fp)
	}

	ok, err := g.confirm(reg.Event, reg.Command)
	if err != nil {
		return fmt.Errorf("hooks: confirmation prompt: %w", err)
	}
	if !ok {
		return fmt.Errorf("hooks: %s hook %q was denied by the user", reg.Event, reg.Command)
	}
	return g.recordApproval(fp)
}

// Approved reports whether a registration has already been consented to.
func (g *ConsentGate) Approved(reg contracts.HookRegistration) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.isApproved(fingerprint(reg))
}

// RevokeAll clears all persisted consent records.
func (g *ConsentGate) RevokeAll() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.approved = make(map[string]struct{})
	return g.persist()
}

// isApproved checks the in-memory set. Caller must hold mu.
func (g *ConsentGate) isApproved(fp string) bool {
	_, ok := g.approved[fp]
	return ok
}

// recordApproval stores approval in memory and flushes to disk.
func (g *ConsentGate) recordApproval(fp string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.approved[fp] = struct{}{}
	return g.persist()
}

// persist writes the approved set to disk. Caller must hold mu.
func (g *ConsentGate) persist() error {
	fps := make([]string, 0, len(g.approved))
	for fp := range g.approved {
		fps = append(fps, fp)
	}
	state := consentState{Approved: fps}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("hooks: marshal consent state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(g.statePath), 0o755); err != nil {
		return fmt.Errorf("hooks: create consent dir: %w", err)
	}
	return os.WriteFile(g.statePath, append(data, '\n'), 0o600)
}

// fingerprint returns a stable, human-readable key for a registration.
func fingerprint(reg contracts.HookRegistration) string {
	return string(reg.Event) + ":" + reg.Command
}
