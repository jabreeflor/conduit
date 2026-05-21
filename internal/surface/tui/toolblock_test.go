package tui

import (
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestRenderToolBlock_collapsed(t *testing.T) {
	cases := []struct {
		name       string
		status     contracts.ToolStatus
		wantIcon   string
		wantToggle string
	}{
		{"running", contracts.ToolStatusRunning, iconRunning, iconCollapsed},
		{"done", contracts.ToolStatusDone, iconDone, iconCollapsed},
		{"failed", contracts.ToolStatusFailed, iconFailed, iconCollapsed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderToolBlock(contracts.ToolCall{
				Name:   "read_file",
				Input:  `{"path":"foo"}`,
				Status: tc.status,
			})
			if !strings.Contains(got, tc.wantIcon) {
				t.Errorf("collapsed %s: want status icon %q in %q", tc.name, tc.wantIcon, got)
			}
			if !strings.Contains(got, tc.wantToggle) {
				t.Errorf("collapsed %s: want toggle icon %q in %q", tc.name, tc.wantToggle, got)
			}
			if strings.Contains(got, "input:") {
				t.Errorf("collapsed: should not show input, got %q", got)
			}
			if strings.Contains(got, "\n") {
				t.Errorf("collapsed: should be single line, got %q", got)
			}
		})
	}
}

func TestRenderToolBlock_expanded(t *testing.T) {
	tc := contracts.ToolCall{
		Name:     "web_search",
		Input:    `{"query":"bubbletea"}`,
		Output:   `{"results":[]}`,
		Status:   contracts.ToolStatusDone,
		Expanded: true,
	}
	got := RenderToolBlock(tc)

	if !strings.Contains(got, iconExpanded) {
		t.Errorf("expanded: want toggle icon %q in %q", iconExpanded, got)
	}
	if !strings.Contains(got, iconDone) {
		t.Errorf("expanded: want status icon %q in %q", iconDone, got)
	}
	if !strings.Contains(got, "input:") {
		t.Errorf("expanded: want input line in %q", got)
	}
	if !strings.Contains(got, tc.Input) {
		t.Errorf("expanded: want input value in %q", got)
	}
	if !strings.Contains(got, "output:") {
		t.Errorf("expanded: want output line in %q", got)
	}
	if !strings.Contains(got, tc.Output) {
		t.Errorf("expanded: want output value in %q", got)
	}
}

func TestRenderToolBlock_expandedNoOutput(t *testing.T) {
	tc := contracts.ToolCall{
		Name:     "read_file",
		Input:    `{"path":"foo"}`,
		Status:   contracts.ToolStatusRunning,
		Expanded: true,
	}
	got := RenderToolBlock(tc)

	if !strings.Contains(got, "input:") {
		t.Errorf("want input line in %q", got)
	}
	if strings.Contains(got, "output:") {
		t.Errorf("want no output line when Output is empty, got %q", got)
	}
}

func TestRenderToolBlock_nameInHeader(t *testing.T) {
	tc := contracts.ToolCall{Name: "my_special_tool", Status: contracts.ToolStatusRunning}
	got := RenderToolBlock(tc)
	if !strings.Contains(got, "my_special_tool") {
		t.Errorf("want tool name in header, got %q", got)
	}
}
