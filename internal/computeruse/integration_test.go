package computeruse_test

import (
	"testing"

	"github.com/jabreeflor/conduit/internal/computeruse"
	"github.com/jabreeflor/conduit/internal/config"
	"github.com/jabreeflor/conduit/internal/mcp"
)

func TestFromRootConfig(t *testing.T) {
	root := config.Config{
		ComputerUse: config.ComputerUseConfig{
			Enabled:        true,
			Command:        "open-computer-use",
			Args:           []string{"mcp"},
			Env:            []string{"X=1"},
			Allowlist:      []string{"list_apps"},
			ForceNonDarwin: true,
		},
	}
	got := computeruse.FromRootConfig(root)
	if !got.Enabled || !got.ForceNonDarwin {
		t.Fatalf("flags not propagated: %+v", got)
	}
	if got.Command != "open-computer-use" {
		t.Errorf("Command = %q", got.Command)
	}
	if len(got.Args) != 1 || got.Args[0] != "mcp" {
		t.Errorf("Args = %v", got.Args)
	}
	if len(got.Env) != 1 || got.Env[0] != "X=1" {
		t.Errorf("Env = %v", got.Env)
	}
	if len(got.Allowlist) != 1 || got.Allowlist[0] != "list_apps" {
		t.Errorf("Allowlist = %v", got.Allowlist)
	}
}

func TestMergeIntoInactive(t *testing.T) {
	in := mcp.Config{Servers: []mcp.ServerEntry{{Name: "other"}}}
	out := computeruse.MergeInto(in, computeruse.Config{Enabled: false})
	if len(out.Servers) != 1 || out.Servers[0].Name != "other" {
		t.Fatalf("inactive merge changed config: %+v", out.Servers)
	}
}

func TestMergeIntoActiveAppends(t *testing.T) {
	in := mcp.Config{Servers: []mcp.ServerEntry{{Name: "other"}}}
	out := computeruse.MergeInto(in, computeruse.Config{
		Enabled:        true,
		ForceNonDarwin: true,
	})
	if len(out.Servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(out.Servers))
	}
	if out.Servers[1].Name != computeruse.ServerName {
		t.Errorf("appended server name = %q", out.Servers[1].Name)
	}
}

func TestMergeIntoUserOverrideWins(t *testing.T) {
	// If a user has explicitly configured the server (e.g. with a custom
	// binary path), the integration must not stomp it.
	custom := mcp.ServerEntry{
		Name:      computeruse.ServerName,
		Transport: mcp.TransportStdio,
		Command:   "/custom/path",
		Args:      []string{"mcp", "--debug"},
	}
	in := mcp.Config{Servers: []mcp.ServerEntry{custom}}
	out := computeruse.MergeInto(in, computeruse.Config{
		Enabled:        true,
		ForceNonDarwin: true,
	})
	if len(out.Servers) != 1 {
		t.Fatalf("expected 1 server (user override), got %d", len(out.Servers))
	}
	if out.Servers[0].Command != "/custom/path" {
		t.Errorf("user override lost: %+v", out.Servers[0])
	}
}
