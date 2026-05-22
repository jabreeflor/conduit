package gui

import (
	"strings"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestWorktreeTabViewIncludesStatusAndHistory(t *testing.T) {
	tab := NewWorktreeTab()
	tab.Update(contracts.CodingWorktreeState{
		SessionID:      "code-1",
		CWD:            "/repo-wt",
		Branch:         "main",
		WorktreePath:   "/repo-wt",
		WorktreeBranch: "feature/x",
		WorktreeActive: true,
		History: []contracts.CodingWorktreeEvent{{
			At:     time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC),
			Action: "enter",
			Branch: "feature/x",
		}},
	})

	out := tab.View()
	for _, want := range []string{"Worktree", "Session: code-1", "Status: active on feature/x", "Path: /repo-wt", "enter feature/x"} {
		if !strings.Contains(out, want) {
			t.Fatalf("View() missing %q:\n%s", want, out)
		}
	}
}
