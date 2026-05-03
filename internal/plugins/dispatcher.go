package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// HookContext is the payload passed into every lifecycle handler. The exact
// fields populated depend on the event:
//
//   - beforePrompt / afterTurn / onResume / beforePersist:
//     SessionID, CWD, Prompt (beforePrompt only), TurnOutput (afterTurn only)
//   - beforeDelegate / afterDelegate: SessionID, ToolName, ToolInput,
//     ToolOutput (afterDelegate only)
//
// Plugins read what they need and ignore the rest. State is the
// session-scoped key/value bag persisted across turns when the manifest
// declares a state binding; mutations made by the handler are flushed by the
// dispatcher when Dispatch returns.
type HookContext struct {
	Event      LifecycleHook
	SessionID  string
	CWD        string
	Prompt     string
	TurnOutput string
	ToolName   string
	ToolInput  map[string]any
	ToolOutput string
	State      map[string]any
}

// HookDecision is what a handler returns to influence runtime behaviour.
// All fields are optional. Block short-circuits the surrounding action.
// Inject contributes additional context the agent should see (the surface
// decides where — typically appended to the system prompt for beforePrompt).
type HookDecision struct {
	Block        bool
	BlockReason  string
	Inject       string
	StateUpdates map[string]any
}

// Handler is the in-process callback invoked when a lifecycle event fires.
// Implementations should treat the context as read-only except for State,
// which is the dispatcher-managed persisted bag.
type Handler func(ctx context.Context, hc HookContext) (HookDecision, error)

// Runtime is the live plugin host. It owns the manifest registry, the
// in-process handler table, and per-plugin per-session state files.
type Runtime struct {
	mu        sync.RWMutex
	manifests []Manifest
	handlers  map[string]Handler
	stateRoot string
}

// NewRuntime constructs a runtime backed by stateRoot for state persistence.
// stateRoot may be empty to disable state I/O (in-memory only).
func NewRuntime(stateRoot string) *Runtime {
	return &Runtime{handlers: map[string]Handler{}, stateRoot: stateRoot}
}

// Load installs the supplied manifests as the active plugin set.
func (r *Runtime) Load(manifests []Manifest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.manifests = append(r.manifests[:0:0], manifests...)
}

// RegisterHandler binds an in-process function to the global name a manifest
// references via HookBinding.Handler. Handlers are shared across plugins; a
// single function can serve multiple manifests.
func (r *Runtime) RegisterHandler(name string, h Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[name] = h
}

// Manifests returns a copy of the loaded manifest list, useful for
// surfaces that enumerate plugins.
func (r *Runtime) Manifests() []Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Manifest, len(r.manifests))
	copy(out, r.manifests)
	return out
}

// Dispatch fires every registered handler for the named event in manifest
// order. The first handler to return Block stops the chain; the cumulative
// Inject text is concatenated and returned in the final decision.
//
// Handler errors do NOT abort the chain — they are joined into the returned
// error so a single broken plugin cannot silently disable the rest.
func (r *Runtime) Dispatch(ctx context.Context, event LifecycleHook, hc HookContext) (HookDecision, error) {
	r.mu.RLock()
	manifests := append([]Manifest(nil), r.manifests...)
	handlers := make(map[string]Handler, len(r.handlers))
	for k, v := range r.handlers {
		handlers[k] = v
	}
	r.mu.RUnlock()

	final := HookDecision{}
	var injects []string
	var errs []error

	for _, m := range manifests {
		state, _ := r.loadState(m, hc.SessionID)
		hc.State = state
		fired := false
		for _, binding := range m.Hooks {
			if binding.Event != event {
				continue
			}
			if binding.Matcher != "" && !matches(binding.Matcher, hc) {
				continue
			}
			h, ok := handlers[binding.Handler]
			if !ok {
				errs = append(errs, fmt.Errorf("plugins: %s: handler %q not registered", m.Name, binding.Handler))
				continue
			}
			dec, err := h(ctx, hc)
			if err != nil {
				errs = append(errs, fmt.Errorf("plugins: %s/%s: %w", m.Name, binding.Handler, err))
				continue
			}
			fired = true
			if dec.Inject != "" {
				injects = append(injects, dec.Inject)
			}
			for k, v := range dec.StateUpdates {
				if hc.State == nil {
					hc.State = map[string]any{}
				}
				hc.State[k] = v
			}
			if dec.Block {
				final.Block = true
				final.BlockReason = dec.BlockReason
				_ = r.saveState(m, hc.SessionID, hc.State)
				final.Inject = strings.Join(injects, "\n")
				if len(errs) > 0 {
					return final, joinErrs(errs)
				}
				return final, nil
			}
		}
		if fired {
			_ = r.saveState(m, hc.SessionID, hc.State)
		}
	}

	final.Inject = strings.Join(injects, "\n")
	if len(errs) > 0 {
		return final, joinErrs(errs)
	}
	return final, nil
}

func matches(matcher string, hc HookContext) bool {
	matcher = strings.ToLower(matcher)
	candidates := []string{hc.ToolName, hc.Prompt}
	for _, c := range candidates {
		if c != "" && strings.Contains(strings.ToLower(c), matcher) {
			return true
		}
	}
	return false
}

func joinErrs(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	parts := make([]string, len(errs))
	for i, e := range errs {
		parts[i] = e.Error()
	}
	return fmt.Errorf("%s", strings.Join(parts, "; "))
}

// VirtualTools returns every virtual tool declared by the loaded manifests in
// stable order. The host tool layer registers these as Runners that delegate
// to RenderVirtualTool.
func (r *Runtime) VirtualTools() []VirtualTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []VirtualTool
	for _, m := range r.manifests {
		out = append(out, m.VirtualTools...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ToolAliases returns the union of every plugin's renames in load order so
// later plugins can override earlier ones if they bind the same source.
func (r *Runtime) ToolAliases() []ToolAlias {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []ToolAlias
	for _, m := range r.manifests {
		out = append(out, m.ToolAliases...)
	}
	return out
}

// BlockedTools returns the union of every plugin's tool block list.
func (r *Runtime) BlockedTools() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := map[string]struct{}{}
	var out []string
	for _, m := range r.manifests {
		for _, name := range m.BlockedTools {
			if _, dup := seen[name]; dup {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// RenderVirtualTool produces the response body for a virtual tool call by
// substituting {{.input.X}} and {{.session_id}} markers. The substitution
// is deliberately tiny — virtual tools are for canned responses, not full
// templating; plugins that need logic should bind a real handler instead.
func RenderVirtualTool(vt VirtualTool, sessionID string, input map[string]any) string {
	body := vt.Response
	body = strings.ReplaceAll(body, "{{.session_id}}", sessionID)
	for k, v := range input {
		marker := "{{.input." + k + "}}"
		body = strings.ReplaceAll(body, marker, fmt.Sprintf("%v", v))
	}
	return body
}

// loadState reads the per-session state file for a plugin. Missing files are
// returned as empty maps so handlers see a consistent zero value.
func (r *Runtime) loadState(m Manifest, sessionID string) (map[string]any, error) {
	if r.stateRoot == "" || m.State == nil || m.State.Path == "" || sessionID == "" {
		return map[string]any{}, nil
	}
	path := r.stateFilePath(m, sessionID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return map[string]any{}, err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]any{}, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func (r *Runtime) saveState(m Manifest, sessionID string, state map[string]any) error {
	if r.stateRoot == "" || m.State == nil || m.State.Path == "" || sessionID == "" {
		return nil
	}
	if state == nil {
		state = map[string]any{}
	}
	path := r.stateFilePath(m, sessionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (r *Runtime) stateFilePath(m Manifest, sessionID string) string {
	// Sanitise sessionID to a single path segment so callers cannot
	// accidentally escape the state root via a "../" id.
	safe := strings.ReplaceAll(sessionID, string(os.PathSeparator), "_")
	safe = strings.ReplaceAll(safe, "..", "_")
	return filepath.Join(r.stateRoot, m.Name, safe+".json")
}
