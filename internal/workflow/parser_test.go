package workflow

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

// TestParseFile_Fixtures parses each top-level shape and asserts the
// expected action variant survives a YAML round trip. The fixtures double
// as documentation for authors.
func TestParseFile_Fixtures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		file       string
		wantID     string
		wantSteps  int
		wantFirst  contracts.WorkflowActionKind
		wantBranch bool
	}{
		{file: "tool_workflow.yaml", wantID: "ship-release", wantSteps: 2, wantFirst: contracts.WorkflowActionTool},
		{file: "model_workflow.yaml", wantID: "summarize-doc", wantSteps: 2, wantFirst: contracts.WorkflowActionTool},
		{file: "branch_workflow.yaml", wantID: "triage", wantSteps: 2, wantBranch: true, wantFirst: contracts.WorkflowActionModel},
		{file: "subagent_workflow.yaml", wantID: "research-spawn", wantSteps: 1, wantFirst: contracts.WorkflowActionSubagent},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join("testdata", tc.file)
			def, err := ParseFile(path)
			if err != nil {
				t.Fatalf("ParseFile(%q) error: %v", path, err)
			}
			if def.ID != tc.wantID {
				t.Errorf("id = %q, want %q", def.ID, tc.wantID)
			}
			if len(def.Steps) != tc.wantSteps {
				t.Fatalf("len(Steps) = %d, want %d", len(def.Steps), tc.wantSteps)
			}
			gotKind, count := def.Steps[0].Action()
			if count != 1 {
				t.Fatalf("first step action count = %d, want 1", count)
			}
			if gotKind != tc.wantFirst {
				t.Errorf("first step kind = %q, want %q", gotKind, tc.wantFirst)
			}
			if tc.wantBranch {
				if def.Steps[1].Branch == nil {
					t.Fatalf("expected branch on second step, got nil")
				}
				if len(def.Steps[1].Branch.Cases) < 2 {
					t.Errorf("len(Branch.Cases) = %d, want >= 2", len(def.Steps[1].Branch.Cases))
				}
			}
			// Validate must accept every fixture so the canonical examples
			// stay correct.
			if err := Validate(def); err != nil {
				t.Errorf("Validate(%q) returned error: %v", path, err)
			}
		})
	}
}

// TestParse_RejectionClasses exercises the parser-level rejection
// behaviours: empty document, malformed YAML, and unknown fields.
func TestParse_RejectionClasses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   string
		wantErr error
		wantSub string
	}{
		{
			name:    "empty input",
			input:   "",
			wantErr: ErrEmptyDocument,
		},
		{
			name:    "whitespace only",
			input:   "   \n  \n",
			wantErr: ErrEmptyDocument,
		},
		{
			name:    "malformed yaml",
			input:   "id: ok\nsteps: [oops",
			wantSub: "parse",
		},
		{
			name: "unknown top-level field",
			input: `id: x
version: "1"
mystery: 42
steps:
  - id: a
    tool:
      name: noop
`,
			wantSub: "mystery",
		},
		{
			name: "unknown step field",
			input: `id: x
version: "1"
steps:
  - id: a
    nope: true
    tool:
      name: noop
`,
			wantSub: "nope",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse([]byte(tc.input))
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want errors.Is(%v) = true", err, tc.wantErr)
			}
			if tc.wantSub != "" && !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

// TestParseFile_NotFound surfaces a clear error when the file does not exist.
func TestParseFile_NotFound(t *testing.T) {
	t.Parallel()
	_, err := ParseFile(filepath.Join("testdata", "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "read") {
		t.Errorf("err = %q, want substring %q", err.Error(), "read")
	}
}

// TestParse_HappyMinimal verifies the smallest legal workflow round-trips
// and exposes the expected action kind.
func TestParse_HappyMinimal(t *testing.T) {
	t.Parallel()
	const src = `id: minimal
version: "1.0"
steps:
  - id: only
    tool:
      name: shell
`
	def, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if def.ID != "minimal" {
		t.Errorf("id = %q", def.ID)
	}
	if len(def.Steps) != 1 || def.Steps[0].Tool == nil || def.Steps[0].Tool.Name != "shell" {
		t.Errorf("unexpected step shape: %+v", def.Steps)
	}
}
