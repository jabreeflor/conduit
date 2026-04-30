package tui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
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
	if !strings.Contains(got, "status: model claude-haiku-4-5; escalates to claude-opus-4-6") {
		t.Fatalf("output %q does not include model escalation status", got)
	}
	if !strings.Contains(got, "[session:") {
		t.Fatalf("output %q does not include session ID in status bar", got)
	}
}

func TestFormatStatusBarAllFields(t *testing.T) {
	s := contracts.UsageSummary{
		Model:          "claude-sonnet-4-6",
		SessionID:      "1746057234567",
		TotalCostUSD:   0.0123,
		ActiveWorkflow: "code-review",
	}

	got := formatStatusBar(s)

	if !strings.Contains(got, "[claude-sonnet-4-6]") {
		t.Errorf("status bar %q missing model", got)
	}
	if !strings.Contains(got, "[$0.0123]") {
		t.Errorf("status bar %q missing cost", got)
	}
	if !strings.Contains(got, "[session:1746057234567]") {
		t.Errorf("status bar %q missing session ID", got)
	}
	if !strings.Contains(got, "[workflow:code-review]") {
		t.Errorf("status bar %q missing active workflow", got)
	}
}

func TestFormatStatusBarNoWorkflow(t *testing.T) {
	s := contracts.UsageSummary{
		Model:     "claude-haiku-4-5",
		SessionID: "123",
	}

	got := formatStatusBar(s)

	if strings.Contains(got, "workflow") {
		t.Errorf("status bar %q should not include workflow when idle", got)
	}
}
