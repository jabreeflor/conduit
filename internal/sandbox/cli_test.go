package sandbox

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// runCLI dispatches args against m and asserts success, returning stdout.
func runCLI(t *testing.T, m *Manager, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if err := dispatchCLI(context.Background(), m, args, &stdout, &stderr); err != nil {
		t.Fatalf("CLI %v failed: %v\nstderr=%s", args, err, stderr.String())
	}
	return stdout.String()
}

// runCLIErr dispatches args against m and returns (stdout, stderr, err)
// without asserting success.
func runCLIErr(t *testing.T, m *Manager, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := dispatchCLI(context.Background(), m, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func TestCLINoArgsPrintsUsage(t *testing.T) {
	m := newTestManager(t)
	_, stderr, err := runCLIErr(t, m)
	if err == nil {
		t.Fatal("dispatchCLI with no args should return an error")
	}
	if !strings.Contains(stderr, "usage:") {
		t.Fatalf("usage missing from stderr: %q", stderr)
	}
}

func TestCLIUnknownVerb(t *testing.T) {
	m := newTestManager(t)
	_, _, err := runCLIErr(t, m, "frobnicate")
	if err == nil {
		t.Fatal("unknown verb should error")
	}
}

func TestCLICreateListSwitchActiveDestroyRoundTrip(t *testing.T) {
	m := newTestManager(t)

	// Empty list shows the empty-state message.
	out := runCLI(t, m, "list")
	if !strings.Contains(out, "no sandboxes") {
		t.Errorf("empty list missing empty-state message: %q", out)
	}

	// Create.
	out = runCLI(t, m, "create", "alpha")
	if !strings.Contains(out, "created sandbox") || !strings.Contains(out, "alpha") {
		t.Errorf("create output: %q", out)
	}

	// active reads back as none (we did not pass --activate).
	out = runCLI(t, m, "active")
	if !strings.Contains(out, "no active sandbox") {
		t.Errorf("active = %q, want 'no active sandbox'", out)
	}

	// switch.
	out = runCLI(t, m, "switch", "alpha")
	if !strings.Contains(out, "switched to sandbox") {
		t.Errorf("switch output: %q", out)
	}

	// active reads back as alpha.
	out = runCLI(t, m, "active")
	if strings.TrimSpace(out) != "alpha" {
		t.Errorf("active = %q, want alpha", strings.TrimSpace(out))
	}

	// list now marks alpha with '*'.
	out = runCLI(t, m, "list")
	if !strings.Contains(out, "*") {
		t.Errorf("list missing active marker: %q", out)
	}

	// destroy --force (flag must precede positional with stdlib flag pkg).
	out = runCLI(t, m, "destroy", "--force", "alpha")
	if !strings.Contains(out, "destroyed sandbox") {
		t.Errorf("destroy output: %q", out)
	}

	// active reads back as none.
	out = runCLI(t, m, "active")
	if !strings.Contains(out, "no active sandbox") {
		t.Errorf("active after destroy = %q", out)
	}
}

func TestCLICreateActivate(t *testing.T) {
	m := newTestManager(t)
	out := runCLI(t, m, "create", "--activate", "alpha")
	if !strings.Contains(out, "active sandbox: alpha") {
		t.Fatalf("--activate output: %q", out)
	}
	out = runCLI(t, m, "active")
	if strings.TrimSpace(out) != "alpha" {
		t.Fatalf("active = %q, want alpha", strings.TrimSpace(out))
	}
}

func TestCLICreateAcceptsResourceFlags(t *testing.T) {
	m := newTestManager(t)
	runCLI(t, m,
		"create",
		"--quota-bytes", "1048576",
		"--memory-bytes", "524288",
		"--cpu-limit", "2.5",
		"alpha",
	)
	ws, err := m.Open("alpha")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if ws.Quota() != 1<<20 || ws.MemoryLimit() != 1<<19 || ws.CPULimit() != 2.5 {
		t.Fatalf("limits: q=%d m=%d c=%f", ws.Quota(), ws.MemoryLimit(), ws.CPULimit())
	}
}

func TestCLICreateRequiresName(t *testing.T) {
	m := newTestManager(t)
	if _, _, err := runCLIErr(t, m, "create"); err == nil {
		t.Fatal("create with no name should error")
	}
}

func TestCLISwitchMissingErrors(t *testing.T) {
	m := newTestManager(t)
	if _, _, err := runCLIErr(t, m, "switch", "ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("switch missing err=%v, want ErrNotFound", err)
	}
}

func TestCLICloneViaCLI(t *testing.T) {
	m := newTestManager(t)
	runCLI(t, m, "create", "src")

	// Plant a payload so we can verify the clone surfaces it.
	srcWS, err := m.Open("src")
	if err != nil {
		t.Fatalf("Open src: %v", err)
	}
	writeFile(t, srcWS.Path(SubdirWorkspace)+"/data.txt", []byte("payload"))

	out := runCLI(t, m, "clone", "src", "dst")
	if !strings.Contains(out, "cloned") {
		t.Fatalf("clone output: %q", out)
	}

	dstWS, err := m.Open("dst")
	if err != nil {
		t.Fatalf("Open dst: %v", err)
	}
	got, err := os.ReadFile(dstWS.Path(SubdirWorkspace) + "/data.txt")
	if err != nil {
		t.Fatalf("read clone payload: %v", err)
	}
	if string(got) != "payload" {
		t.Fatalf("clone payload = %q, want payload", got)
	}
}

func TestCLIClonePropagatesOverrides(t *testing.T) {
	m := newTestManager(t)
	runCLI(t, m, "create", "--memory-bytes", "1073741824", "src")
	runCLI(t, m, "clone", "--memory-bytes", "536870912", "src", "dst")
	ws, err := m.Open("dst")
	if err != nil {
		t.Fatalf("Open dst: %v", err)
	}
	if ws.MemoryLimit() != 1<<29 {
		t.Fatalf("clone MemoryLimit = %d, want %d", ws.MemoryLimit(), 1<<29)
	}
}

func TestCLIDestroyClearsActive(t *testing.T) {
	m := newTestManager(t)
	runCLI(t, m, "create", "--activate", "x")
	runCLI(t, m, "destroy", "--force", "x")
	out := runCLI(t, m, "active")
	if !strings.Contains(out, "no active sandbox") {
		t.Fatalf("active after destroy = %q", out)
	}
}

func TestCLIHelpVerbsPrintUsage(t *testing.T) {
	m := newTestManager(t)
	for _, verb := range []string{"help", "--help", "-h"} {
		out := runCLI(t, m, verb)
		if !strings.Contains(out, "usage:") {
			t.Fatalf("%s output missing usage: %q", verb, out)
		}
	}
}

func TestHumanBytesEdges(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "—"},
		{-1, "—"},
		{500, "500B"},
		{1 << 10, "1.0KiB"},
		{1 << 20, "1.0MiB"},
		{1 << 30, "1.0GiB"},
	}
	for _, c := range cases {
		if got := humanBytes(c.in); got != c.want {
			t.Errorf("humanBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatCPUEdges(t *testing.T) {
	if got := formatCPU(0); got != "—" {
		t.Errorf("formatCPU(0) = %q, want —", got)
	}
	if got := formatCPU(2.5); !strings.HasPrefix(got, "2.5") {
		t.Errorf("formatCPU(2.5) = %q, want starts with 2.5", got)
	}
}
