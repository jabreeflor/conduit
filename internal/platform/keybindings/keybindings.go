// Package keybindings loads the user-configurable keymap for Conduit's TUI
// (and, eventually, GUI) surfaces. PRD §6.15.
//
// The runtime resolves bindings by command ID — TUI input handlers ask the
// resolved keymap "what key triggers chat.new?" rather than hardcoding key
// strings. Defaults live in this package; user overrides are loaded from
// ~/.conduit/keybindings.json.
//
// File format mirrors the PRD example:
//
//	[
//	  { "key": "alt+space", "command": "conduit.summon" },
//	  { "key": "mod+n",     "command": "chat.new" },
//	  { "key": "mod+k",     "command": "commandPalette.toggle" }
//	]
//
// Key string format:
//   - lowercase tokens joined with "+" (e.g. "ctrl+k", "shift+enter")
//   - "mod" is a portable alias that resolves to "ctrl" (matches T3 Code's
//     convention for non-mac hosts; the GUI surface may map it to cmd later)
//   - sequences of two chords are written space-separated, e.g.
//     "ctrl+x ctrl+s"
//   - a "when" clause may gate a binding to a UI context (e.g.
//     "sessionActive"); empty/missing means the binding always applies
//
// Malformed or unknown entries produce a Warning rather than a hard failure
// so a mistyped user file never bricks the TUI.
package keybindings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Command is a canonical command ID that the surfaces dispatch on. Centralising
// the IDs here means the PRD list, the loader, and the TUI input handler all
// agree on the same strings — and unknown IDs in the user file get caught by
// the validator instead of silently misbehaving.
type Command string

const (
	CommandChatNew              Command = "chat.new"
	CommandChatNewLocal         Command = "chat.newLocal"
	CommandChatFork             Command = "chat.fork"
	CommandSessionLoad          Command = "session.load"
	CommandSessionFork          Command = "session.fork"
	CommandWorkflowRun          Command = "workflow.run"
	CommandWorkflowPause        Command = "workflow.pause"
	CommandMemoryInspect        Command = "memory.inspect"
	CommandTerminalToggle       Command = "terminal.toggle"
	CommandCommandPaletteToggle Command = "commandPalette.toggle"
	CommandConduitSummon        Command = "conduit.summon"
	CommandConduitQuit          Command = "conduit.quit"

	// TUI-internal commands — surfaced here so users can rebind them too.
	// They aren't in the PRD's headline list but are the only existing TUI
	// shortcuts, and giving them stable IDs is the whole point of this issue.
	CommandTUITogglePanel    = Command("tui.togglePanel")
	CommandTUIExpandTool     = Command("tui.expandTool")
	CommandTUISubmit         = Command("tui.submit")
	CommandTUISetupLocal     = Command("setup.local")
	CommandTUISetupAPI       = Command("setup.externalAPI")
	CommandTUISessionBrowser = Command("tui.sessionBrowser")
)

// AllCommands is the registry of canonical command IDs. Order matters for
// rendering help screens and for deterministic test output.
var AllCommands = []Command{
	CommandChatNew,
	CommandChatNewLocal,
	CommandChatFork,
	CommandSessionLoad,
	CommandSessionFork,
	CommandWorkflowRun,
	CommandWorkflowPause,
	CommandMemoryInspect,
	CommandTerminalToggle,
	CommandCommandPaletteToggle,
	CommandConduitSummon,
	CommandConduitQuit,
	CommandTUITogglePanel,
	CommandTUIExpandTool,
	CommandTUISubmit,
	CommandTUISetupLocal,
	CommandTUISetupAPI,
	CommandTUISessionBrowser,
}

// IsKnown reports whether c is in the canonical registry.
func IsKnown(c Command) bool {
	for _, k := range AllCommands {
		if k == c {
			return true
		}
	}
	return false
}

// Binding is one entry from the keybindings file. Multiple Bindings may target
// the same Command (e.g. esc and ctrl+c both quitting).
type Binding struct {
	Key     string  `json:"key"`
	Command Command `json:"command"`
	When    string  `json:"when,omitempty"`
}

// Defaults returns the built-in keymap. Inspired by T3 Code's bindings list
// (PRD §6.15) and the existing TUI shortcuts in internal/tui/model.go.
//
// Returning a fresh slice on every call keeps callers from accidentally
// mutating shared state.
func Defaults() []Binding {
	return []Binding{
		{Key: "alt+space", Command: CommandConduitSummon},
		{Key: "mod+n", Command: CommandChatNew},
		{Key: "mod+shift+n", Command: CommandChatNewLocal},
		{Key: "mod+k", Command: CommandCommandPaletteToggle},
		{Key: "mod+j", Command: CommandTerminalToggle},
		{Key: "mod+r", Command: CommandWorkflowRun, When: "workflowSelected"},
		{Key: "mod+s", Command: CommandSessionFork, When: "sessionActive"},
		{Key: "mod+f", Command: CommandChatFork},
		{Key: "mod+o", Command: CommandSessionLoad},
		{Key: "mod+m", Command: CommandMemoryInspect},
		{Key: "mod+.", Command: CommandWorkflowPause, When: "workflowSelected"},

		// Existing TUI shortcuts kept stable so the issue is non-breaking.
		{Key: "ctrl+p", Command: CommandTUITogglePanel},
		{Key: "x", Command: CommandTUIExpandTool},
		{Key: "enter", Command: CommandTUISubmit},
		{Key: "l", Command: CommandTUISetupLocal},
		{Key: "a", Command: CommandTUISetupAPI},
		// PRD §6.13 session tree browser. ctrl+b avoids collision with
		// session.fork on mod+s, which has a different `when` scope.
		{Key: "ctrl+b", Command: CommandTUISessionBrowser},

		// Two paths to quit because the existing TUI accepts both — and the
		// PRD's command list explicitly includes conduit.quit.
		{Key: "esc", Command: CommandConduitQuit},
		{Key: "ctrl+c", Command: CommandConduitQuit},
	}
}

// Warning is a non-fatal problem the loader noticed while parsing the user
// file. Callers (CLI, TUI splash) typically print these rather than aborting.
type Warning struct {
	Source string // file path or "<defaults>"
	Line   int    // best-effort, 0 if unknown
	Reason string
}

func (w Warning) String() string {
	if w.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", w.Source, w.Line, w.Reason)
	}
	if w.Source != "" {
		return fmt.Sprintf("%s: %s", w.Source, w.Reason)
	}
	return w.Reason
}

// Keymap is the resolved binding table the surfaces query at runtime.
type Keymap struct {
	// byCommand[cmd] -> normalised key strings (deduped, deterministic order).
	byCommand map[Command][]string
	// byKey["ctrl+p"] -> commands triggered. Two commands sharing a key is
	// allowed only if they have non-overlapping "when" guards; we don't enforce
	// that here, surfaces are responsible for context-aware dispatch.
	byKey map[string][]Binding
	// raw is the resolved Binding list, after defaults+overrides+normalisation.
	raw []Binding
}

// Bindings returns the resolved binding list (a fresh copy).
func (k *Keymap) Bindings() []Binding {
	out := make([]Binding, len(k.raw))
	copy(out, k.raw)
	return out
}

// KeysFor returns the normalised key strings bound to cmd, in stable order.
// Empty slice means the command is unbound — surfaces should treat that as
// "shortcut disabled" rather than crashing.
func (k *Keymap) KeysFor(cmd Command) []string {
	if k == nil {
		return nil
	}
	out := make([]string, len(k.byCommand[cmd]))
	copy(out, k.byCommand[cmd])
	return out
}

// CommandFor returns the first command bound to the given key, or "" if no
// binding matches. The key argument is normalised before lookup so callers
// can pass raw input strings like "Ctrl+P".
func (k *Keymap) CommandFor(key string) Command {
	if k == nil {
		return ""
	}
	norm, err := NormalizeKey(key)
	if err != nil {
		return ""
	}
	binds := k.byKey[norm]
	if len(binds) == 0 {
		return ""
	}
	return binds[0].Command
}

// Matches reports whether key triggers cmd under the current keymap. This is
// the primary lookup TUI input handlers should use.
func (k *Keymap) Matches(key string, cmd Command) bool {
	if k == nil {
		return false
	}
	norm, err := NormalizeKey(key)
	if err != nil {
		return false
	}
	for _, b := range k.byKey[norm] {
		if b.Command == cmd {
			return true
		}
	}
	return false
}

// Default builds a Keymap from Defaults() with no warnings. Useful for tests
// and headless paths where the user file does not exist.
func Default() *Keymap {
	km, _ := build(Defaults(), nil, "<defaults>")
	return km
}

// Load reads ~/.conduit/keybindings.json (creating ~/.conduit if missing) and
// merges it on top of Defaults. Errors reading the file are non-fatal — the
// caller still gets a valid Keymap built from defaults plus any warnings.
//
// The file is created on disk only if the directory itself was missing; we
// don't materialise an empty bindings.json on every boot because that would
// surprise users who deliberately deleted theirs.
func Load() (*Keymap, []Warning, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fall back to defaults; refusing to start because $HOME is unset
		// would be worse than running with built-in shortcuts.
		km := Default()
		return km, []Warning{{Reason: fmt.Sprintf("resolve home dir: %v; using defaults", err)}}, nil
	}
	return LoadFrom(filepath.Join(home, ".conduit", "keybindings.json"))
}

// LoadFrom is Load with an explicit path. Exported for tests and for callers
// that already resolved the config dir.
func LoadFrom(path string) (*Keymap, []Warning, error) {
	var warnings []Warning

	// Ensure the parent directory exists so users can `cat > keybindings.json`
	// without first having to mkdir ~/.conduit themselves.
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			warnings = append(warnings, Warning{Source: path, Reason: fmt.Sprintf("create config dir: %v", err)})
		}
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || (err == nil && len(strings.TrimSpace(string(data))) == 0) {
		km, defaultWarnings := build(Defaults(), nil, "<defaults>")
		warnings = append(warnings, defaultWarnings...)
		return km, warnings, nil
	}
	if err != nil {
		warnings = append(warnings, Warning{Source: path, Reason: fmt.Sprintf("read: %v; using defaults", err)})
		km, defaultWarnings := build(Defaults(), nil, "<defaults>")
		warnings = append(warnings, defaultWarnings...)
		return km, warnings, nil
	}

	overrides, parseWarnings := parseUserFile(path, data)
	warnings = append(warnings, parseWarnings...)

	km, mergeWarnings := build(Defaults(), overrides, path)
	warnings = append(warnings, mergeWarnings...)
	return km, warnings, nil
}

func parseUserFile(path string, data []byte) ([]Binding, []Warning) {
	var raw []Binding
	if err := json.Unmarshal(data, &raw); err != nil {
		// Malformed JSON falls back to defaults with a warning rather than
		// crashing the TUI mid-boot. The user can fix their file and reload.
		return nil, []Warning{{Source: path, Reason: fmt.Sprintf("parse JSON (using defaults instead): %v", err)}}
	}
	return raw, nil
}

// build merges defaults + overrides into a Keymap. Overrides win at the
// (key, when) granularity: a user entry for "mod+n" replaces the default
// binding on "mod+n", but defaults for unrelated keys stay intact.
//
// Setting a user entry's command to "" (empty) is treated as "unbind this
// key" — the default binding for that key is dropped without replacement.
func build(defaults []Binding, overrides []Binding, source string) (*Keymap, []Warning) {
	var warnings []Warning

	// Step 1: index defaults by key (after normalisation).
	type entry struct {
		Binding Binding
	}
	indexed := map[string][]entry{}
	keyOrder := []string{} // preserve declaration order for deterministic output

	addOrReplace := func(b Binding, src string, idx int) {
		norm, err := NormalizeKey(b.Key)
		if err != nil {
			warnings = append(warnings, Warning{Source: src, Line: idx + 1, Reason: fmt.Sprintf("invalid key %q: %v", b.Key, err)})
			return
		}
		// Empty command means "unbind this key" when it appears in overrides.
		if b.Command == "" {
			if src == source && source != "<defaults>" {
				delete(indexed, norm)
				return
			}
			warnings = append(warnings, Warning{Source: src, Line: idx + 1, Reason: fmt.Sprintf("binding %q has empty command", b.Key)})
			return
		}
		if !IsKnown(b.Command) {
			warnings = append(warnings, Warning{Source: src, Line: idx + 1, Reason: fmt.Sprintf("unknown command %q (ignored)", b.Command)})
			return
		}
		b.Key = norm
		// Replace any existing entry on the same (key, when) — last writer wins.
		next := []entry{{Binding: b}}
		for _, e := range indexed[norm] {
			if e.Binding.When != b.When {
				next = append(next, e)
			}
		}
		if _, seen := indexed[norm]; !seen {
			keyOrder = append(keyOrder, norm)
		}
		indexed[norm] = next
	}

	for i, b := range defaults {
		addOrReplace(b, "<defaults>", i)
	}
	for i, b := range overrides {
		addOrReplace(b, source, i)
	}

	// Materialise the resolved list in deterministic order: keys in declaration
	// order, with defaults appearing before user overrides for the same key.
	resolved := make([]Binding, 0, len(indexed)*2)
	for _, k := range keyOrder {
		es := indexed[k]
		// Sort entries by when-clause for determinism (empty when first).
		sort.SliceStable(es, func(i, j int) bool { return es[i].Binding.When < es[j].Binding.When })
		for _, e := range es {
			resolved = append(resolved, e.Binding)
		}
	}

	byCommand := map[Command][]string{}
	byKey := map[string][]Binding{}
	for _, b := range resolved {
		byKey[b.Key] = append(byKey[b.Key], b)
		// Dedupe keys within a command's list.
		seen := false
		for _, k := range byCommand[b.Command] {
			if k == b.Key {
				seen = true
				break
			}
		}
		if !seen {
			byCommand[b.Command] = append(byCommand[b.Command], b.Key)
		}
	}

	return &Keymap{byCommand: byCommand, byKey: byKey, raw: resolved}, warnings
}
