package eval

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSuiteFileParsesAssertions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "suite.yaml")
	data := []byte(`
name: morning-brief-eval
cases:
  - name: trigger
    input: run morning brief
    model: claude-opus-4-6
    expect:
      tool_calls_include: [memory.read]
      reply_contains_tag: "[[canvas:html]]"
      duration_max_seconds: 30
      cost_max_usd: 0.10
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	suite, err := LoadSuiteFile(path)
	if err != nil {
		t.Fatalf("LoadSuiteFile: %v", err)
	}
	if suite.Name != "morning-brief-eval" {
		t.Errorf("Name = %q", suite.Name)
	}
	if got := suite.Cases[0].Expect.ToolCallsInclude[0]; got != "memory.read" {
		t.Errorf("tool_calls_include = %q", got)
	}
}

func TestLoadSuitesDirectorySortsYAML(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.yaml", "a.yml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`
name: suite-`+name+`
cases:
  - name: case
    input: hello
    expect:
      reply_contains: hello
`), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	suites, err := LoadSuites(dir)
	if err != nil {
		t.Fatalf("LoadSuites: %v", err)
	}
	if len(suites) != 2 {
		t.Fatalf("len(suites) = %d, want 2", len(suites))
	}
	if suites[0].Name != "suite-a.yml" {
		t.Errorf("first suite = %q, want sorted a.yml", suites[0].Name)
	}
}
