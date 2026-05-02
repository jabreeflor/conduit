package capabilities

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Manager owns the set of opt-in capability adapters and routes tool calls to
// the right one. It is the entry point most callers use:
//
//	mgr := capabilities.NewManager(cfg, approval, factory)
//	if err := mgr.Init(ctx); err != nil { ... }
//	defer mgr.Shutdown(ctx)
//	tools, _ := mgr.ListTools(ctx)
//	res, err := mgr.Dispatch(ctx, "shell.exec", map[string]any{...})
type Manager struct {
	caps []Capability
}

// NewManager constructs a Manager with the standard set of adapters,
// instantiating only those enabled in cfg. approval and factory may be nil:
//
//   - approval == nil  ⇒ AllowAllApproval (safe for tests; production callers
//     should wire in the real per-app gate from #39).
//   - factory == nil   ⇒ Browser and Desktop will report unavailable.
func NewManager(cfg Config, approval Approval, factory MCPClientFactory) *Manager {
	if approval == nil {
		approval = AllowAllApproval
	}

	var caps []Capability
	if cfg.Shell {
		caps = append(caps, NewShellCapability(cfg, approval))
	}
	if cfg.Browser {
		caps = append(caps, NewBrowserCapability(cfg, approval, factory))
	}
	if cfg.Desktop {
		caps = append(caps, NewDesktopCapability(cfg, approval, factory))
	}
	return &Manager{caps: caps}
}

// NewManagerFromCapabilities builds a Manager from an explicit slice. Useful
// for tests that want to inject fakes.
func NewManagerFromCapabilities(caps ...Capability) *Manager {
	return &Manager{caps: append([]Capability(nil), caps...)}
}

// Capabilities returns the active capability kinds in stable order.
func (m *Manager) Capabilities() []Kind {
	kinds := make([]Kind, 0, len(m.caps))
	for _, c := range m.caps {
		kinds = append(kinds, c.Kind())
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })
	return kinds
}

// Init initializes every active capability. Errors from individual adapters
// are aggregated; an adapter that fails Init still stays in the manager so
// that ListTools and Dispatch report the unavailable reason consistently.
func (m *Manager) Init(ctx context.Context) error {
	var errs []error
	for _, c := range m.caps {
		if err := c.Init(ctx); err != nil {
			errs = append(errs, fmt.Errorf("init %s: %w", c.Kind(), err))
		}
	}
	return errors.Join(errs...)
}

// ListTools returns the union of tools advertised by every active capability.
// Capabilities that report unavailable are silently skipped — ListTools is
// for prompt construction, where missing tools should not appear, while
// Dispatch is where the user-visible "unavailable" message is surfaced.
func (m *Manager) ListTools(ctx context.Context) ([]Tool, error) {
	var out []Tool
	for _, c := range m.caps {
		tools, err := c.ListTools(ctx)
		if err != nil {
			if errors.Is(err, ErrCapabilityUnavailable) {
				continue
			}
			return nil, fmt.Errorf("list %s tools: %w", c.Kind(), err)
		}
		out = append(out, tools...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Dispatch routes a tool call to the capability that owns it. Tool ownership
// is determined by the prefix before the first "." in the tool name (e.g.
// "shell.exec" → KindShell). Unknown prefixes return an error.
func (m *Manager) Dispatch(ctx context.Context, name string, args map[string]any) (Result, error) {
	kind, ok := capabilityForTool(name)
	if !ok {
		return Result{}, fmt.Errorf("capabilities: tool %q has no capability prefix (expected e.g. shell.exec)", name)
	}
	for _, c := range m.caps {
		if c.Kind() == kind {
			return c.Dispatch(ctx, name, args)
		}
	}
	return Result{}, fmt.Errorf("capabilities: %s is not enabled or registered", kind)
}

// Shutdown closes every capability. Errors are aggregated.
func (m *Manager) Shutdown(ctx context.Context) error {
	var errs []error
	for _, c := range m.caps {
		if err := c.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutdown %s: %w", c.Kind(), err))
		}
	}
	return errors.Join(errs...)
}

// capabilityForTool returns the capability kind responsible for a tool by
// inspecting the prefix before the first ".".
func capabilityForTool(toolName string) (Kind, bool) {
	idx := strings.Index(toolName, ".")
	if idx <= 0 {
		return "", false
	}
	switch Kind(toolName[:idx]) {
	case KindShell:
		return KindShell, true
	case KindBrowser:
		return KindBrowser, true
	case KindDesktop:
		return KindDesktop, true
	default:
		return "", false
	}
}
