// Package capabilities implements PRD §6.8 — three modular, opt-in capability
// adapters (Shell, Browser, Desktop) that expose computer-use tools through a
// common, transport-agnostic interface.
//
// Each capability is independently togglable via Config. Browser and Desktop
// are thin proxies to externally-configured MCP servers (Chrome MCP for
// Browser; open-codex-computer-use for Desktop) so the underlying server can
// evolve without touching this package. Shell wraps a local exec primitive
// gated by the per-app approval hook.
//
// The package deliberately avoids hard-importing the MCP server packages so
// that callers can wire concrete implementations (or stubs) in via the
// MCPClientFactory hook. This keeps PR #40 independent of #37 and #39 while
// still exposing a clean integration seam.
package capabilities

import (
	"context"
	"errors"
	"fmt"
)

// Kind identifies one capability adapter.
type Kind string

const (
	KindShell   Kind = "shell"
	KindBrowser Kind = "browser"
	KindDesktop Kind = "desktop"
)

// Tool is a capability-provided tool descriptor. It mirrors the shape used by
// the MCP and tools packages but is deliberately decoupled — callers translate
// to their preferred dialect.
type Tool struct {
	// Capability is the originating adapter (shell, browser, desktop).
	Capability Kind
	// Name is the fully-qualified tool name (e.g. "shell.exec",
	// "browser.navigate", "desktop.click"). Names from MCP-backed adapters are
	// prefixed with the capability kind so they cannot collide with built-in
	// tools.
	Name string
	// Description is the short human-readable summary surfaced to models.
	Description string
	// Schema is the JSON Schema for the tool's input arguments.
	Schema map[string]any
}

// Result is the normalized output of a Dispatch call.
// Text is the human-readable result. Data carries optional structured payload
// (for tools that emit JSON). IsError signals a tool-level failure that the
// caller may surface back to the model without aborting the loop.
type Result struct {
	Text    string
	Data    map[string]any
	IsError bool
}

// Capability is the contract all adapters satisfy. Implementations must be
// safe to call from a single goroutine; callers serialize Dispatch per
// capability when concurrency is required.
type Capability interface {
	// Kind returns the adapter's identity.
	Kind() Kind

	// Init prepares the adapter (connects to MCP servers, validates config).
	// Init may report a non-fatal "unavailable" error — callers should
	// continue with the remaining capabilities and surface the message via
	// ListTools / Dispatch when the user invokes this capability.
	Init(ctx context.Context) error

	// ListTools returns the tools currently exposed by this capability.
	// Returning an empty slice with a nil error means the adapter is opted-in
	// but has no tools yet (e.g. MCP server not connected).
	ListTools(ctx context.Context) ([]Tool, error)

	// Dispatch executes one tool invocation. The adapter is responsible for
	// argument validation, approval-gate consultation, and translation to the
	// underlying transport.
	Dispatch(ctx context.Context, toolName string, args map[string]any) (Result, error)

	// Shutdown releases any resources held by the adapter (subprocess
	// connections, file handles). Idempotent.
	Shutdown(ctx context.Context) error
}

// Approval is the per-app approval hook from issue #39. Adapters call
// RequireApproval before performing a side-effecting action; the returned
// boolean indicates whether the user has approved this app.
//
// When #39 lands, callers wire in a concrete implementation backed by the
// persistent approval config. Until then, callers may pass the package-level
// AllowAllApproval (open-by-default) or DenyAllApproval as appropriate; tests
// also use these stubs.
type Approval interface {
	RequireApproval(ctx context.Context, app string, action string) (bool, error)
}

// ApprovalFunc adapts a function to the Approval interface for callers that
// want to inline a closure (e.g. tests, CLI prompts).
type ApprovalFunc func(ctx context.Context, app string, action string) (bool, error)

// RequireApproval implements Approval.
func (f ApprovalFunc) RequireApproval(ctx context.Context, app string, action string) (bool, error) {
	return f(ctx, app, action)
}

// AllowAllApproval is a default-open stub used when issue #39's per-app
// approval system has not been wired in yet. It records nothing and approves
// every request. TODO(#39): replace with the persistent per-app gate.
var AllowAllApproval Approval = ApprovalFunc(func(_ context.Context, _ string, _ string) (bool, error) {
	return true, nil
})

// DenyAllApproval is a safe-by-default stub for unit tests and dry-runs.
var DenyAllApproval Approval = ApprovalFunc(func(_ context.Context, _ string, _ string) (bool, error) {
	return false, nil
})

// ErrCapabilityUnavailable is returned by Dispatch / ListTools when a
// capability is opted-in but its underlying transport (MCP server, exec
// runtime) is not reachable. Callers should surface the wrapped message to
// the user instead of treating it as a hard failure.
var ErrCapabilityUnavailable = errors.New("capability unavailable")

// UnavailableError wraps ErrCapabilityUnavailable with adapter-specific
// context (kind + reason) so users see a useful message in TUI/CLI output.
type UnavailableError struct {
	Kind   Kind
	Reason string
}

func (e *UnavailableError) Error() string {
	return fmt.Sprintf("%s capability unavailable: %s", e.Kind, e.Reason)
}

func (e *UnavailableError) Unwrap() error { return ErrCapabilityUnavailable }

// newUnavailable builds an UnavailableError. Adapters use this from Dispatch
// and ListTools when the underlying server is missing.
func newUnavailable(kind Kind, reason string) error {
	return &UnavailableError{Kind: kind, Reason: reason}
}
