package tui

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunBootsAgainstCore(t *testing.T) {
	var out bytes.Buffer

	if err := Run(&out); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Conduit core online") {
		t.Fatalf("output %q does not include boot message", got)
	}
	if !strings.Contains(got, "tui") || !strings.Contains(got, "gui") {
		t.Fatalf("output %q does not include expected surfaces", got)
	}
}
