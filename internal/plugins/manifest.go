// Package plugins implements Conduit's plugin runtime — manifest discovery,
// lifecycle hook dispatch, virtual tools, tool aliases/blocks, and
// per-session state persistence. PRD §6.24.5.
//
// A plugin is a directory under one of the discovery roots whose top-level
// `plugin.json` describes its name, version, lifecycle hooks, and any tool
// transforms it wants to perform. The runtime stays intentionally
// declarative: hooks are referenced by name and resolved by the host
// (initially in-process Go callbacks; out-of-process executables are a
// future extension that does not change the manifest shape).
package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LifecycleHook identifies one of the six manifest-declared lifecycle points.
// They mirror the PRD §6.24.5 vocabulary exactly so plugin authors can copy
// docs into manifests without translation.
type LifecycleHook string

const (
	HookBeforePrompt   LifecycleHook = "beforePrompt"
	HookAfterTurn      LifecycleHook = "afterTurn"
	HookOnResume       LifecycleHook = "onResume"
	HookBeforePersist  LifecycleHook = "beforePersist"
	HookBeforeDelegate LifecycleHook = "beforeDelegate"
	HookAfterDelegate  LifecycleHook = "afterDelegate"
)

// AllLifecycleHooks lists every supported hook in spec order. Used by the
// dispatcher to validate manifests and by surfaces that enumerate hook points.
var AllLifecycleHooks = []LifecycleHook{
	HookBeforePrompt,
	HookAfterTurn,
	HookOnResume,
	HookBeforePersist,
	HookBeforeDelegate,
	HookAfterDelegate,
}

// ToolAlias renames an existing host tool when exposed to the model. The
// underlying Runner is unchanged; the alias affects discovery and prompt
// surfaces only.
type ToolAlias struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// VirtualTool is a plugin-defined tool that, when invoked, returns a static
// templated response without dispatching to a Runner. The template body is
// rendered with the call's input map via Go's text/template-style {{.field}}
// substitution (implemented in dispatcher.go to keep this file declarative).
type VirtualTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"schema,omitempty"`
	// Response is the raw text returned to the model. Supports {{.input.foo}}
	// and {{.session_id}} substitutions handled by RenderVirtualTool.
	Response string `json:"response"`
}

// Manifest is the parsed `plugin.json`. It is intentionally JSON (not YAML)
// to mirror the Codex/Claude plugin convention referenced in the PRD.
type Manifest struct {
	Name         string         `json:"name"`
	Version      string         `json:"version"`
	Description  string         `json:"description,omitempty"`
	Hooks        []HookBinding  `json:"hooks,omitempty"`
	ToolAliases  []ToolAlias    `json:"tool_aliases,omitempty"`
	BlockedTools []string       `json:"blocked_tools,omitempty"`
	VirtualTools []VirtualTool  `json:"virtual_tools,omitempty"`
	State        *StateBinding  `json:"state,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	// Path is filled in by Discover; it is the absolute path to the plugin
	// root directory so callers can resolve plugin-relative state files.
	Path string `json:"-"`
}

// HookBinding is one entry in Manifest.Hooks. Handler is the in-process
// callback name registered via Runtime.RegisterHandler. Manifests that
// reference an unknown handler load successfully but the dispatcher reports
// the gap so plugin authors see what's missing.
type HookBinding struct {
	Event   LifecycleHook `json:"event"`
	Handler string        `json:"handler"`
	// Matcher is an optional substring filter applied to the contextual
	// payload (e.g. tool name for delegate events). Empty matches everything.
	Matcher string `json:"matcher,omitempty"`
}

// StateBinding declares where session-scoped plugin state is persisted.
// Path is interpreted relative to the plugin root and namespaced per session
// id by the runtime. Format defaults to JSON.
type StateBinding struct {
	Path   string `json:"path"`
	Format string `json:"format,omitempty"`
}

// Validate checks the manifest for shape errors that would crash the
// dispatcher. It is intentionally strict so a typo in `event:` surfaces at
// discovery time, not when the user is mid-conversation.
func (m Manifest) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("plugin manifest: name is required")
	}
	if strings.TrimSpace(m.Version) == "" {
		return errors.New("plugin manifest: version is required")
	}
	for i, h := range m.Hooks {
		if !isKnownHook(h.Event) {
			return fmt.Errorf("plugin manifest: hooks[%d]: unknown event %q", i, h.Event)
		}
		if strings.TrimSpace(h.Handler) == "" {
			return fmt.Errorf("plugin manifest: hooks[%d]: handler is required", i)
		}
	}
	seen := map[string]struct{}{}
	for i, vt := range m.VirtualTools {
		if strings.TrimSpace(vt.Name) == "" {
			return fmt.Errorf("plugin manifest: virtual_tools[%d]: name is required", i)
		}
		if _, dup := seen[vt.Name]; dup {
			return fmt.Errorf("plugin manifest: virtual_tools[%d]: duplicate name %q", i, vt.Name)
		}
		seen[vt.Name] = struct{}{}
	}
	for i, a := range m.ToolAliases {
		if strings.TrimSpace(a.From) == "" || strings.TrimSpace(a.To) == "" {
			return fmt.Errorf("plugin manifest: tool_aliases[%d]: both from and to are required", i)
		}
	}
	return nil
}

func isKnownHook(e LifecycleHook) bool {
	for _, k := range AllLifecycleHooks {
		if k == e {
			return true
		}
	}
	return false
}

// LoadManifest parses a single plugin.json file from disk. The Path field on
// the returned manifest is set to the directory containing the manifest.
func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("plugins: read manifest %q: %w", path, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("plugins: parse manifest %q: %w", path, err)
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	m.Path = filepath.Dir(path)
	return m, nil
}

// Discover walks each root for `<root>/<plugin>/plugin.json` and returns the
// loaded manifests in alphabetical order by name. Missing roots are normal on
// first run and silently skipped. Per-plugin parse errors are returned in a
// joined error so a single bad manifest does not hide the others.
func Discover(roots []string) ([]Manifest, error) {
	var manifests []Manifest
	var errs []error
	seen := map[string]struct{}{}
	for _, root := range roots {
		if root == "" {
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("plugins: read root %q: %w", root, err))
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			manifestPath := filepath.Join(root, e.Name(), "plugin.json")
			if _, statErr := os.Stat(manifestPath); statErr != nil {
				continue
			}
			m, err := LoadManifest(manifestPath)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			// Earlier roots win on name collision (workspace before user, by
			// caller convention). Skip with a recorded conflict error so the
			// surface can show the loser path.
			if _, dup := seen[m.Name]; dup {
				errs = append(errs, fmt.Errorf("plugins: duplicate plugin name %q at %s (shadowed)", m.Name, m.Path))
				continue
			}
			seen[m.Name] = struct{}{}
			manifests = append(manifests, m)
		}
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].Name < manifests[j].Name })
	if len(errs) > 0 {
		return manifests, errors.Join(errs...)
	}
	return manifests, nil
}

// DefaultRoots returns the canonical lookup hierarchy: workspace plugins
// override user plugins. Either argument may be empty when the caller has no
// workspace or no home directory.
func DefaultRoots(home, workspace string) []string {
	var roots []string
	if workspace != "" {
		roots = append(roots, filepath.Join(workspace, ".conduit", "plugins"))
	}
	if home != "" {
		roots = append(roots, filepath.Join(home, ".conduit", "plugins"))
	}
	return roots
}
