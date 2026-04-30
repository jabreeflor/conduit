package hooks

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func newGate(t *testing.T, config ConsentConfig, confirm ConfirmFunc) *ConsentGate {
	t.Helper()
	if config.StatePath == "" {
		config.StatePath = filepath.Join(t.TempDir(), "hook_consent.json")
	}
	g, err := NewConsentGate(config, confirm)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g
}

var echoReg = contracts.HookRegistration{
	Event:   contracts.HookEventPreToolUse,
	Command: "echo hello",
}

func TestRequireConsent_nonInteractiveWithoutAcceptHooks_blocks(t *testing.T) {
	g := newGate(t, ConsentConfig{Interactive: false, AcceptHooks: false}, nil)
	err := g.RequireConsent(echoReg)
	if err == nil {
		t.Fatal("expected error for non-interactive caller without accept_hooks")
	}
}

func TestRequireConsent_nonInteractiveWithAcceptHooks_autoApproves(t *testing.T) {
	g := newGate(t, ConsentConfig{Interactive: false, AcceptHooks: true}, nil)
	if err := g.RequireConsent(echoReg); err != nil {
		t.Fatalf("RequireConsent: %v", err)
	}
	if !g.Approved(echoReg) {
		t.Error("hook should be marked approved after auto-accept")
	}
}

func TestRequireConsent_interactiveApproved_passes(t *testing.T) {
	confirm := func(_ contracts.HookEvent, _ string) (bool, error) { return true, nil }
	g := newGate(t, ConsentConfig{Interactive: true}, confirm)

	if err := g.RequireConsent(echoReg); err != nil {
		t.Fatalf("RequireConsent: %v", err)
	}
	if !g.Approved(echoReg) {
		t.Error("hook should be approved after user confirmed")
	}
}

func TestRequireConsent_interactiveDenied_errors(t *testing.T) {
	confirm := func(_ contracts.HookEvent, _ string) (bool, error) { return false, nil }
	g := newGate(t, ConsentConfig{Interactive: true}, confirm)

	err := g.RequireConsent(echoReg)
	if err == nil {
		t.Fatal("expected error when user denies the hook")
	}
	if g.Approved(echoReg) {
		t.Error("hook should not be approved after user denied")
	}
}

func TestRequireConsent_confirmError_propagates(t *testing.T) {
	sentinel := errors.New("terminal closed")
	confirm := func(_ contracts.HookEvent, _ string) (bool, error) { return false, sentinel }
	g := newGate(t, ConsentConfig{Interactive: true}, confirm)

	err := g.RequireConsent(echoReg)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestRequireConsent_alreadyApproved_skipsConfirm(t *testing.T) {
	calls := 0
	confirm := func(_ contracts.HookEvent, _ string) (bool, error) {
		calls++
		return true, nil
	}
	g := newGate(t, ConsentConfig{Interactive: true}, confirm)

	g.RequireConsent(echoReg) //nolint:errcheck — first use
	g.RequireConsent(echoReg) //nolint:errcheck — second use

	if calls != 1 {
		t.Errorf("confirm called %d times, want 1", calls)
	}
}

func TestConsentPersists_acrossGateInstances(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "hook_consent.json")
	confirm := func(_ contracts.HookEvent, _ string) (bool, error) { return true, nil }

	g1, _ := NewConsentGate(ConsentConfig{Interactive: true, StatePath: statePath}, confirm)
	if err := g1.RequireConsent(echoReg); err != nil {
		t.Fatalf("first gate: %v", err)
	}

	// A second gate instance loading from the same path should see the consent.
	g2, _ := NewConsentGate(ConsentConfig{Interactive: true, StatePath: statePath}, nil)
	if !g2.Approved(echoReg) {
		t.Error("second gate should load persisted consent from disk")
	}
}

func TestConsentStatePath_filePermissions(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "hook_consent.json")
	confirm := func(_ contracts.HookEvent, _ string) (bool, error) { return true, nil }

	g, _ := NewConsentGate(ConsentConfig{Interactive: true, StatePath: statePath}, confirm)
	g.RequireConsent(echoReg) //nolint:errcheck

	info, err := os.Stat(statePath)
	if err != nil {
		t.Fatalf("stat state file: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("state file mode = %o, want 0600", mode)
	}
}

func TestConsentStateJSON_isHumanReadable(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "hook_consent.json")
	confirm := func(_ contracts.HookEvent, _ string) (bool, error) { return true, nil }

	g, _ := NewConsentGate(ConsentConfig{Interactive: true, StatePath: statePath}, confirm)
	g.RequireConsent(echoReg) //nolint:errcheck

	data, _ := os.ReadFile(statePath)
	var state consentState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	if len(state.Approved) != 1 {
		t.Fatalf("approved count = %d, want 1", len(state.Approved))
	}
	want := "PreToolUse:echo hello"
	if state.Approved[0] != want {
		t.Errorf("fingerprint = %q, want %q", state.Approved[0], want)
	}
}

func TestRevokeAll_clearsApprovedSet(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "hook_consent.json")
	confirm := func(_ contracts.HookEvent, _ string) (bool, error) { return true, nil }

	g, _ := NewConsentGate(ConsentConfig{Interactive: true, StatePath: statePath}, confirm)
	g.RequireConsent(echoReg) //nolint:errcheck

	if err := g.RevokeAll(); err != nil {
		t.Fatalf("RevokeAll: %v", err)
	}
	if g.Approved(echoReg) {
		t.Error("hook should not be approved after RevokeAll")
	}

	// Reload from disk — persisted state should also be empty.
	g2, _ := NewConsentGate(ConsentConfig{StatePath: statePath}, nil)
	if g2.Approved(echoReg) {
		t.Error("reloaded gate should not show revoked consent")
	}
}

func TestAllSevenHookEvents_supported(t *testing.T) {
	events := []contracts.HookEvent{
		contracts.HookEventPreToolUse,
		contracts.HookEventPostToolUse,
		contracts.HookEventPermissionRequest,
		contracts.HookEventUserPromptSubmit,
		contracts.HookEventStop,
		contracts.HookEventAgentTurnComplete,
		contracts.HookEventApprovalRequested,
	}
	if len(events) != 7 {
		t.Fatalf("expected 7 hook events, got %d", len(events))
	}
	confirm := func(_ contracts.HookEvent, _ string) (bool, error) { return true, nil }
	g := newGate(t, ConsentConfig{Interactive: true}, confirm)

	for _, event := range events {
		reg := contracts.HookRegistration{Event: event, Command: "true"}
		if err := g.RequireConsent(reg); err != nil {
			t.Errorf("RequireConsent(%s): %v", event, err)
		}
	}
}
