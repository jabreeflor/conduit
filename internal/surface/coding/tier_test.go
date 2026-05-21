package coding

import (
	"sort"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestRegisterCodingTools(t *testing.T) {
	base := DefaultCodingTools()

	tests := []struct {
		name        string
		perms       contracts.CodingPermissions
		wantCount   int
		wantNames   []string
		wantMissing []string
	}{
		{
			name:        "default deny write and shell",
			perms:       contracts.CodingPermissions{},
			wantCount:   8,
			wantNames:   []string{"list_dir", "read_file", "glob_search", "grep_search", "web_fetch", "web_search", "tool_search", "sleep"},
			wantMissing: []string{"write_file", "edit_file", "notebook_edit", "bash"},
		},
		{
			name:        "allow write only",
			perms:       contracts.CodingPermissions{AllowWrite: true},
			wantCount:   11,
			wantNames:   []string{"list_dir", "write_file", "edit_file", "notebook_edit"},
			wantMissing: []string{"bash"},
		},
		{
			name:        "allow shell only",
			perms:       contracts.CodingPermissions{AllowShell: true},
			wantCount:   9,
			wantNames:   []string{"list_dir", "bash"},
			wantMissing: []string{"write_file", "edit_file", "notebook_edit"},
		},
		{
			name:      "allow write and shell",
			perms:     contracts.CodingPermissions{AllowWrite: true, AllowShell: true},
			wantCount: 12,
			wantNames: []string{"list_dir", "write_file", "bash"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RegisterCodingTools(base, tc.perms)
			if len(got) != tc.wantCount {
				names := make([]string, 0, len(got))
				for _, tl := range got {
					names = append(names, tl.Name)
				}
				sort.Strings(names)
				t.Fatalf("count: got %d (%v), want %d", len(got), names, tc.wantCount)
			}
			gotSet := make(map[string]bool, len(got))
			for _, tl := range got {
				gotSet[tl.Name] = true
			}
			for _, want := range tc.wantNames {
				if !gotSet[want] {
					t.Errorf("expected tool %q to be present", want)
				}
			}
			for _, missing := range tc.wantMissing {
				if gotSet[missing] {
					t.Errorf("expected tool %q to be filtered out", missing)
				}
			}
		})
	}
}

func TestStubRunnerReportsNotImplemented(t *testing.T) {
	tools := DefaultCodingTools()
	if len(tools) == 0 {
		t.Fatal("expected default tools")
	}
	tool := tools[0].Tool
	res, err := tool.Run(nil, nil)
	if err != nil {
		t.Fatalf("stub runner returned error: %v", err)
	}
	if res.Text == "" {
		t.Fatal("stub runner returned empty text")
	}
}
