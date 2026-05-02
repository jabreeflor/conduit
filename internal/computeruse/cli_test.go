package computeruse

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withFakeHome redirects HOME so CLI calls that touch ~/.conduit land in a
// scratch directory rather than the developer's real home.
func withFakeHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func TestRunCLI_HelpExits(t *testing.T) {
	withFakeHome(t)
	var out, errBuf bytes.Buffer
	if err := RunCLI(context.Background(), nil, &out, &errBuf); err != nil {
		t.Fatalf("RunCLI(nil): %v", err)
	}
	if !strings.Contains(out.String(), "computer-use") {
		t.Errorf("help output missing program name: %q", out.String())
	}
}

func TestRunCLI_ApproveListRevokeRoundTrip(t *testing.T) {
	home := withFakeHome(t)
	var out, errBuf bytes.Buffer

	args := []string{"approve", "--bundle", "com.apple.Safari", "--name", "Safari", "--scope", "full", "--note", "test", "--yes"}
	if err := RunCLI(context.Background(), args, &out, &errBuf); err != nil {
		t.Fatalf("approve: %v (stderr=%s)", err, errBuf.String())
	}
	if !strings.Contains(out.String(), "approved") {
		t.Errorf("expected approve output, got %q", out.String())
	}

	// File should now exist at $HOME/.conduit/approved-apps.json.
	path := filepath.Join(home, DefaultApprovalsPath)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("approvals file not created: %v", err)
	}

	// list should surface the entry.
	out.Reset()
	errBuf.Reset()
	if err := RunCLI(context.Background(), []string{"list"}, &out, &errBuf); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out.String(), "com.apple.Safari") {
		t.Errorf("list missing app: %q", out.String())
	}
	if !strings.Contains(out.String(), "active") {
		t.Errorf("list should mark entry active: %q", out.String())
	}

	// check should succeed (exit 0).
	out.Reset()
	errBuf.Reset()
	if err := RunCLI(context.Background(), []string{"check", "--bundle", "com.apple.Safari"}, &out, &errBuf); err != nil {
		t.Fatalf("check approved: %v", err)
	}
	if !strings.Contains(out.String(), "approved") {
		t.Errorf("check output unexpected: %q", out.String())
	}

	// revoke flips state.
	out.Reset()
	errBuf.Reset()
	if err := RunCLI(context.Background(), []string{"revoke", "--bundle", "com.apple.Safari"}, &out, &errBuf); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if !strings.Contains(out.String(), "revoked") {
		t.Errorf("revoke output unexpected: %q", out.String())
	}

	// check after revoke must return ErrNotApproved.
	out.Reset()
	errBuf.Reset()
	err := RunCLI(context.Background(), []string{"check", "--bundle", "com.apple.Safari"}, &out, &errBuf)
	if !errors.Is(err, ErrNotApproved) {
		t.Errorf("check after revoke err = %v, want ErrNotApproved", err)
	}

	// list --active should now be empty.
	out.Reset()
	errBuf.Reset()
	if err := RunCLI(context.Background(), []string{"list", "--active"}, &out, &errBuf); err != nil {
		t.Fatalf("list --active: %v", err)
	}
	if !strings.Contains(out.String(), "no approvals") {
		t.Errorf("list --active should be empty: %q", out.String())
	}
}

func TestRunCLI_ApproveRequiresAppRef(t *testing.T) {
	withFakeHome(t)
	var out, errBuf bytes.Buffer
	err := RunCLI(context.Background(), []string{"approve", "--yes"}, &out, &errBuf)
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Errorf("expected error about missing --bundle/--name, got %v", err)
	}
}

func TestRunCLI_RevokeUnknownApp(t *testing.T) {
	withFakeHome(t)
	var out, errBuf bytes.Buffer
	err := RunCLI(context.Background(), []string{"revoke", "--bundle", "com.unknown"}, &out, &errBuf)
	if !errors.Is(err, ErrUnknownApp) {
		t.Errorf("revoke unknown err = %v, want ErrUnknownApp", err)
	}
}

func TestRunCLI_UnknownSubcommand(t *testing.T) {
	withFakeHome(t)
	var out, errBuf bytes.Buffer
	err := RunCLI(context.Background(), []string{"bogus"}, &out, &errBuf)
	if err == nil || !strings.Contains(err.Error(), "unknown computer-use subcommand") {
		t.Errorf("expected unknown subcommand error, got %v", err)
	}
}

func TestParseScope(t *testing.T) {
	cases := map[string]Scope{
		"":          ScopeFull,
		"full":      ScopeFull,
		"FULL":      ScopeFull,
		"read_only": ScopeReadOnly,
		"readonly":  ScopeReadOnly,
		"ro":        ScopeReadOnly,
	}
	for in, want := range cases {
		got, err := parseScope(in)
		if err != nil {
			t.Errorf("parseScope(%q) err = %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseScope(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := parseScope("nuke"); err == nil {
		t.Error("parseScope(nuke) should error")
	}
}
