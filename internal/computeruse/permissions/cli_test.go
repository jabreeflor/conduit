package permissions

import (
	"bytes"
	"context"
	"flag"
	"strings"
	"testing"
)

func TestRunCLICheckPrintsBothPermissions(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := RunCLI(context.Background(), []string{"check"}, &stdout, &stderr); err != nil {
		t.Fatalf("RunCLI(check) error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "screen_recording") || !strings.Contains(out, "accessibility") {
		t.Errorf("RunCLI(check) output missing required permissions: %q", out)
	}
}

func TestRunCLIDefaultIsCheck(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := RunCLI(context.Background(), nil, &stdout, &stderr); err != nil {
		t.Fatalf("RunCLI() error: %v", err)
	}
	if !strings.Contains(stdout.String(), "PERMISSION") {
		t.Errorf("RunCLI() default did not print check table: %q", stdout.String())
	}
}

func TestRunCLIRejectsUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := RunCLI(context.Background(), []string{"explode"}, &stdout, &stderr)
	if err != flag.ErrHelp {
		t.Fatalf("RunCLI(explode) = %v, want flag.ErrHelp", err)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("usage line missing from stderr: %q", stderr.String())
	}
}

func TestRunCLIOpenRejectsUnknownPermission(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := RunCLI(context.Background(), []string{"open", "qubits"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("RunCLI(open qubits) returned nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown permission") {
		t.Errorf("error = %q, want 'unknown permission' message", err.Error())
	}
}

func TestRunCLIOpenWithoutArgPrintsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := RunCLI(context.Background(), []string{"open"}, &stdout, &stderr)
	if err != flag.ErrHelp {
		t.Fatalf("RunCLI(open) = %v, want flag.ErrHelp", err)
	}
}
