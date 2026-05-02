package keybindings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultKeymapBindsAllPRDCommands(t *testing.T) {
	km := Default()

	// PRD §6.15 lists these as the headline available commands. Every one
	// must have at least one default binding so the TUI ships usable.
	required := []Command{
		CommandChatNew,
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
	}
	for _, cmd := range required {
		if got := km.KeysFor(cmd); len(got) == 0 {
			t.Errorf("default keymap is missing a binding for %q", cmd)
		}
	}
}

func TestNormalizeKey(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Ctrl+P", "ctrl+p"},
		{"  ctrl+p  ", "ctrl+p"},
		{"shift+ctrl+k", "ctrl+shift+k"},
		{"mod+k", "ctrl+k"}, // mod alias resolves to ctrl
		{"alt+space", "alt+space"},
		{"shift+enter", "shift+enter"},
		{"return", "enter"},
		{"esc", "esc"},
		{"escape", "esc"},
		{"x", "x"},
		{"ctrl+x ctrl+s", "ctrl+x ctrl+s"},
		{"Ctrl+X CTRL+S", "ctrl+x ctrl+s"},
	}
	for _, tc := range cases {
		got, err := NormalizeKey(tc.in)
		if err != nil {
			t.Errorf("NormalizeKey(%q) error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("NormalizeKey(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeKeyRejectsMalformed(t *testing.T) {
	bad := []string{"", "  ", "ctrl+", "+", "ctrl++p"}
	for _, s := range bad {
		if _, err := NormalizeKey(s); err == nil {
			t.Errorf("NormalizeKey(%q) should have errored", s)
		}
	}
}

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestLoadFromMissingFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keybindings.json")
	km, warnings, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if got := km.KeysFor(CommandChatNew); len(got) == 0 {
		t.Fatalf("defaults should bind chat.new")
	}
}

func TestLoadFromUserOverridesMergeOnTopOfDefaults(t *testing.T) {
	dir := t.TempDir()
	// User remaps mod+n to summon and rebinds chat.new to a chord. The
	// default mod+n binding should be replaced; other defaults stay.
	body := `[
		{"key": "ctrl+n", "command": "conduit.summon"},
		{"key": "ctrl+x ctrl+s", "command": "chat.new"}
	]`
	path := writeFile(t, dir, "keybindings.json", body)

	km, warnings, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	if cmd := km.CommandFor("ctrl+n"); cmd != CommandConduitSummon {
		t.Errorf("ctrl+n should map to conduit.summon, got %q", cmd)
	}
	if !km.Matches("ctrl+x ctrl+s", CommandChatNew) {
		t.Errorf("ctrl+x ctrl+s should now trigger chat.new")
	}
	// Default mod+k -> commandPalette.toggle should still apply.
	if cmd := km.CommandFor("mod+k"); cmd != CommandCommandPaletteToggle {
		t.Errorf("mod+k should still map to commandPalette.toggle, got %q", cmd)
	}
	// Default ctrl+p (toggle panel) is untouched.
	if cmd := km.CommandFor("ctrl+p"); cmd != CommandTUITogglePanel {
		t.Errorf("ctrl+p should still map to tui.togglePanel, got %q", cmd)
	}
}

func TestLoadFromMalformedJSONFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "keybindings.json", `{not json`)

	km, warnings, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected at least one warning for malformed JSON")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w.Reason, "parse JSON") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a parse-JSON warning, got %v", warnings)
	}
	// Defaults still in effect.
	if cmd := km.CommandFor("mod+k"); cmd != CommandCommandPaletteToggle {
		t.Fatalf("defaults should still apply after JSON error; got %q", cmd)
	}
}

func TestLoadFromUnknownCommandIsIgnoredWithWarning(t *testing.T) {
	dir := t.TempDir()
	body := `[
		{"key": "ctrl+y", "command": "totally.bogus"},
		{"key": "ctrl+z", "command": "chat.new"}
	]`
	path := writeFile(t, dir, "keybindings.json", body)

	km, warnings, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected exactly one warning for unknown command, got %v", warnings)
	}
	if !strings.Contains(warnings[0].Reason, "unknown command") {
		t.Errorf("expected unknown-command warning, got %q", warnings[0].Reason)
	}
	if cmd := km.CommandFor("ctrl+y"); cmd != "" {
		t.Errorf("ctrl+y should be unbound, got %q", cmd)
	}
	if !km.Matches("ctrl+z", CommandChatNew) {
		t.Errorf("ctrl+z should bind chat.new despite an earlier bad entry")
	}
}

func TestLoadFromMalformedKeyIsIgnoredWithWarning(t *testing.T) {
	dir := t.TempDir()
	body := `[
		{"key": "", "command": "chat.new"},
		{"key": "ctrl+", "command": "chat.new"},
		{"key": "ctrl+y", "command": "chat.new"}
	]`
	path := writeFile(t, dir, "keybindings.json", body)

	km, warnings, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if len(warnings) < 2 {
		t.Errorf("expected warnings for both malformed keys, got %v", warnings)
	}
	if !km.Matches("ctrl+y", CommandChatNew) {
		t.Errorf("ctrl+y should be a valid override even when neighbours are bad")
	}
}

func TestLoadFromCreatesParentDir(t *testing.T) {
	root := t.TempDir()
	// Nested dir does not exist yet.
	path := filepath.Join(root, "nested", "keybindings.json")
	_, _, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("expected parent dir to be created: %v", err)
	}
}

func TestEmptyCommandUnbindsKey(t *testing.T) {
	dir := t.TempDir()
	// User explicitly clears the default mod+k binding.
	body := `[
		{"key": "mod+k", "command": ""}
	]`
	path := writeFile(t, dir, "keybindings.json", body)

	km, _, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cmd := km.CommandFor("mod+k"); cmd != "" {
		t.Errorf("mod+k should be unbound after explicit empty-command override, got %q", cmd)
	}
}

func TestKeymapMatches(t *testing.T) {
	km := Default()
	if !km.Matches("Ctrl+P", CommandTUITogglePanel) {
		t.Errorf("Ctrl+P (any case) should still match the panel toggle")
	}
	if km.Matches("ctrl+p", CommandChatNew) {
		t.Errorf("ctrl+p must not match an unrelated command")
	}
}

func TestKeysFor(t *testing.T) {
	km := Default()
	keys := km.KeysFor(CommandConduitQuit)
	// Defaults bind both esc and ctrl+c to quit.
	if len(keys) < 2 {
		t.Fatalf("conduit.quit should have at least two default keys, got %v", keys)
	}
}
