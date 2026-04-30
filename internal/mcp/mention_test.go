package mcp_test

import (
	"reflect"
	"testing"

	"github.com/jabreeflor/conduit/internal/mcp"
)

func TestParseMentions(t *testing.T) {
	cases := []struct {
		input string
		want  []string // Raw fields
	}{
		{"no mentions here", nil},
		{"use @read_file for this", []string{"@read_file"}},
		{"@write_file and @delete_file both need approval", []string{"@write_file", "@delete_file"}},
		{"email me@example.com", nil}, // bare @ inside a word is not a mention
		{"@Tool1 @tool-name end", []string{"@Tool1", "@tool-name"}},
	}

	for _, tc := range cases {
		got := mcp.ParseMentions(tc.input)
		var raws []string
		for _, m := range got {
			raws = append(raws, m.Raw)
		}
		if !reflect.DeepEqual(raws, tc.want) {
			t.Errorf("ParseMentions(%q) = %v, want %v", tc.input, raws, tc.want)
		}
	}
}

func TestStripMentions(t *testing.T) {
	cleaned, names := mcp.StripMentions("please @read_file the config and @write_file the output")
	wantCleaned := "please  the config and  the output"
	wantNames := []string{"read_file", "write_file"}
	if cleaned != wantCleaned {
		t.Errorf("cleaned = %q, want %q", cleaned, wantCleaned)
	}
	if !reflect.DeepEqual(names, wantNames) {
		t.Errorf("names = %v, want %v", names, wantNames)
	}
}

func TestResolveMentions(t *testing.T) {
	available := []mcp.ToolDef{
		{Name: "read_file"},
		{Name: "write_file"},
	}

	resolved, unresolved := mcp.ResolveMentions([]string{"read_file", "missing_tool"}, available)
	if len(resolved) != 1 || resolved[0].Name != "read_file" {
		t.Errorf("resolved = %+v", resolved)
	}
	if len(unresolved) != 1 || unresolved[0] != "missing_tool" {
		t.Errorf("unresolved = %+v", unresolved)
	}
}
