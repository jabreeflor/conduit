package computeruse_test

import (
	"runtime"
	"testing"

	"github.com/jabreeflor/conduit/internal/computeruse"
	"github.com/jabreeflor/conduit/internal/mcp"
)

func TestConfigDisabledByDefault(t *testing.T) {
	var c computeruse.Config
	if c.IsActive() {
		t.Fatal("zero-value Config must be inactive")
	}
	if _, ok := c.ServerEntry(); ok {
		t.Fatal("zero-value Config must not produce a ServerEntry")
	}
}

func TestConfigGatedByGOOS(t *testing.T) {
	c := computeruse.Config{Enabled: true}
	got := c.IsActive()
	want := runtime.GOOS == "darwin"
	if got != want {
		t.Fatalf("IsActive on %s: got %v, want %v", runtime.GOOS, got, want)
	}
}

func TestConfigForceNonDarwin(t *testing.T) {
	c := computeruse.Config{Enabled: true, ForceNonDarwin: true}
	if !c.IsActive() {
		t.Fatal("ForceNonDarwin should activate on any GOOS when Enabled")
	}
	entry, ok := c.ServerEntry()
	if !ok {
		t.Fatal("expected ServerEntry when ForceNonDarwin is set")
	}
	if entry.Name != computeruse.ServerName {
		t.Errorf("entry.Name = %q, want %q", entry.Name, computeruse.ServerName)
	}
	if entry.Transport != mcp.TransportStdio {
		t.Errorf("entry.Transport = %q, want %q", entry.Transport, mcp.TransportStdio)
	}
	if entry.Command != computeruse.DefaultCommand {
		t.Errorf("entry.Command = %q, want %q", entry.Command, computeruse.DefaultCommand)
	}
	if len(entry.Args) != 1 || entry.Args[0] != "mcp" {
		t.Errorf("entry.Args = %v, want [mcp]", entry.Args)
	}
	if !entry.IsEnabled() {
		t.Error("entry should be marked enabled")
	}
}

func TestConfigOverridesCommandAndArgs(t *testing.T) {
	c := computeruse.Config{
		Enabled:        true,
		ForceNonDarwin: true,
		Command:        "/opt/cu/bin/open-computer-use",
		Args:           []string{"mcp", "--verbose"},
		Env:            []string{"FOO=bar"},
		Allowlist:      []string{"list_apps", "get_app_state"},
	}
	entry, ok := c.ServerEntry()
	if !ok {
		t.Fatal("expected ServerEntry")
	}
	if entry.Command != c.Command {
		t.Errorf("Command override lost: got %q, want %q", entry.Command, c.Command)
	}
	if len(entry.Args) != 2 || entry.Args[1] != "--verbose" {
		t.Errorf("Args override lost: %v", entry.Args)
	}
	if len(entry.Env) != 1 || entry.Env[0] != "FOO=bar" {
		t.Errorf("Env not propagated: %v", entry.Env)
	}
	if len(entry.Allowlist) != 2 {
		t.Errorf("Allowlist not propagated: %v", entry.Allowlist)
	}
}

func TestServerEntryIsolatesSlices(t *testing.T) {
	// Mutating the returned entry must not affect the source Config.
	c := computeruse.Config{
		Enabled:        true,
		ForceNonDarwin: true,
		Args:           []string{"mcp"},
		Allowlist:      []string{"list_apps"},
		Env:            []string{"X=1"},
	}
	entry, _ := c.ServerEntry()
	entry.Args[0] = "tampered"
	entry.Allowlist[0] = "tampered"
	entry.Env[0] = "tampered"

	if c.Args[0] != "mcp" {
		t.Errorf("Args aliased: %v", c.Args)
	}
	if c.Allowlist[0] != "list_apps" {
		t.Errorf("Allowlist aliased: %v", c.Allowlist)
	}
	if c.Env[0] != "X=1" {
		t.Errorf("Env aliased: %v", c.Env)
	}
}

func TestServerNameIsStable(t *testing.T) {
	// Allowlists, telemetry, and per-app approval (#39) key off this
	// string. Changing it is a breaking change.
	if computeruse.ServerName != "open-computer-use" {
		t.Errorf("ServerName = %q; do not rename without a migration", computeruse.ServerName)
	}
}
