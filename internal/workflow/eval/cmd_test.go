package eval

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCLI_RunStoresResults(t *testing.T) {
	dir := t.TempDir()
	suitePath := filepath.Join(dir, "suite.yaml")
	if err := os.WriteFile(suitePath, []byte(`
name: suite
cases:
  - name: case
    input: hello
    expect:
      reply_contains: hello
`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := RunCLI(context.Background(), []string{"run", suitePath, "--model", "claude-opus-4-6", "--results-dir", filepath.Join(dir, "results")}, &out, &out)
	if err != nil {
		t.Fatalf("RunCLI: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Score: 1/1") {
		t.Errorf("output missing score: %s", got)
	}
	if !strings.Contains(got, "Results:") {
		t.Errorf("output missing results path: %s", got)
	}
}

func TestRunCLI_ReportEmpty(t *testing.T) {
	var out bytes.Buffer
	err := RunCLI(context.Background(), []string{"report", "--results-dir", t.TempDir()}, &out, &out)
	if err != nil {
		t.Fatalf("RunCLI: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "No eval results found.") {
		t.Errorf("output = %q", got)
	}
}

func TestRunCLI_Replay(t *testing.T) {
	var out bytes.Buffer
	err := RunCLI(context.Background(), []string{"replay", "sess-1", "--model", "gpt-4o", "--diff"}, &out, &out)
	if err != nil {
		t.Fatalf("RunCLI: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Replay queued for session sess-1 on gpt-4o") {
		t.Errorf("output = %q", got)
	}
}
