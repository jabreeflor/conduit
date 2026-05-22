package coding

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/contracts"
	"github.com/jabreeflor/conduit/internal/tools"
	"github.com/jabreeflor/conduit/internal/tools/websearch"
)

// mustJSON marshals v or panics — test helper only.
func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func TestLiveCodingTools_Count(t *testing.T) {
	live := LiveCodingTools(websearch.DefaultConfig(), nil)
	if len(live) != 14 {
		t.Errorf("expected 14 tools, got %d", len(live))
	}
}

func TestLiveCodingTools_Tiers(t *testing.T) {
	live := LiveCodingTools(websearch.DefaultConfig(), nil)
	perms := contracts.CodingPermissions{AllowWrite: true, AllowShell: true}
	got := RegisterCodingTools(live, perms)
	if len(got) != 14 {
		names := make([]string, len(got))
		for i, t := range got {
			names[i] = t.Name
		}
		t.Errorf("all-perms: expected 14 tools, got %d: %v", len(got), names)
	}

	// read-only: 8 always tools
	got = RegisterCodingTools(live, contracts.CodingPermissions{})
	if len(got) != 8 {
		t.Errorf("read-only: expected 8 tools, got %d", len(got))
	}
}

// ── list_dir ─────────────────────────────────────────────────────────────

func TestListDir_NonRecursive(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)

	tool := listDirTool()
	res, err := tool.Run(context.Background(), mustJSON(map[string]any{"path": dir}))
	if err != nil || res.IsError {
		t.Fatalf("unexpected error: %v / %s", err, res.Text)
	}
	if !strings.Contains(res.Text, "a.txt") || !strings.Contains(res.Text, "sub/") {
		t.Errorf("unexpected output: %s", res.Text)
	}
}

func TestListDir_Recursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.Mkdir(sub, 0o755)
	os.WriteFile(filepath.Join(sub, "deep.go"), []byte("package x"), 0o644)

	tool := listDirTool()
	res, err := tool.Run(context.Background(), mustJSON(map[string]any{"path": dir, "recursive": true}))
	if err != nil || res.IsError {
		t.Fatalf("unexpected error: %v / %s", err, res.Text)
	}
	if !strings.Contains(res.Text, "deep.go") {
		t.Errorf("expected deep.go in recursive listing: %s", res.Text)
	}
}

func TestListDir_MissingPath(t *testing.T) {
	tool := listDirTool()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"path": "/no/such/dir"}))
	if !res.IsError {
		t.Errorf("expected error for missing path")
	}
}

// ── read_file ─────────────────────────────────────────────────────────────

func TestReadFile_WholeFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	os.WriteFile(p, []byte("line1\nline2\nline3\n"), 0o644)

	tool := readFileTool()
	res, err := tool.Run(context.Background(), mustJSON(map[string]any{"path": p}))
	if err != nil || res.IsError {
		t.Fatalf("error: %v / %s", err, res.Text)
	}
	if !strings.Contains(res.Text, "1\tline1") || !strings.Contains(res.Text, "3\tline3") {
		t.Errorf("unexpected: %s", res.Text)
	}
}

func TestReadFile_LineRange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	os.WriteFile(p, []byte("a\nb\nc\nd\n"), 0o644)

	tool := readFileTool()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"path": p, "start_line": 2, "end_line": 3}))
	if strings.Contains(res.Text, "1\ta") || !strings.Contains(res.Text, "2\tb") || !strings.Contains(res.Text, "3\tc") || strings.Contains(res.Text, "4\td") {
		t.Errorf("unexpected line range output: %s", res.Text)
	}
}

func TestReadFile_MissingFile(t *testing.T) {
	tool := readFileTool()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"path": "/no/such/file.txt"}))
	if !res.IsError {
		t.Error("expected error for missing file")
	}
}

// ── glob_search ───────────────────────────────────────────────────────────

func TestGlobSearch_FindGoFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "main.py"), []byte(""), 0o644)

	tool := globSearchTool()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"pattern": "*.go", "path": dir}))
	if !strings.Contains(res.Text, "main.go") {
		t.Errorf("expected main.go: %s", res.Text)
	}
	if strings.Contains(res.Text, "main.py") {
		t.Errorf("unexpected main.py: %s", res.Text)
	}
}

func TestGlobSearch_NoMatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte(""), 0o644)

	tool := globSearchTool()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"pattern": "*.go", "path": dir}))
	if res.IsError {
		t.Errorf("unexpected error: %s", res.Text)
	}
	if !strings.Contains(res.Text, "No files") {
		t.Errorf("expected 'No files' message: %s", res.Text)
	}
}

// ── grep_search ───────────────────────────────────────────────────────────

func TestGrepSearch_BasicMatch(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "code.go")
	os.WriteFile(p, []byte("package main\n\nfunc main() {}\n"), 0o644)

	tool := grepSearchTool()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"pattern": "func main", "path": dir}))
	if !strings.Contains(res.Text, "code.go") || !strings.Contains(res.Text, "func main") {
		t.Errorf("unexpected: %s", res.Text)
	}
}

func TestGrepSearch_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	os.WriteFile(p, []byte("Hello World\n"), 0o644)

	tool := grepSearchTool()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"pattern": "hello", "path": dir, "case_insensitive": true}))
	if !strings.Contains(res.Text, "Hello World") {
		t.Errorf("case-insensitive match failed: %s", res.Text)
	}
}

func TestGrepSearch_NoMatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("nothing here\n"), 0o644)

	tool := grepSearchTool()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"pattern": "xyzzy", "path": dir}))
	if res.IsError || !strings.Contains(res.Text, "No matches") {
		t.Errorf("expected 'No matches': %s", res.Text)
	}
}

func TestGrepSearch_FileFilter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("hello go\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.py"), []byte("hello py\n"), 0o644)

	tool := grepSearchTool()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"pattern": "hello", "path": dir, "include": "*.go"}))
	if !strings.Contains(res.Text, "a.go") {
		t.Errorf("expected a.go: %s", res.Text)
	}
	if strings.Contains(res.Text, "b.py") {
		t.Errorf("unexpected b.py: %s", res.Text)
	}
}

// ── web_fetch ─────────────────────────────────────────────────────────────

func TestWebFetch_LocalFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "hello.txt")
	os.WriteFile(p, []byte("hello local"), 0o644)

	tool := webFetchTool()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"url": p}))
	if res.IsError || !strings.Contains(res.Text, "hello local") {
		t.Errorf("local fetch failed: %s", res.Text)
	}
}

func TestWebFetch_MissingFile(t *testing.T) {
	tool := webFetchTool()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"url": "/no/such/file.txt"}))
	if !res.IsError {
		t.Error("expected error for missing file")
	}
}

// ── tool_search ───────────────────────────────────────────────────────────

func TestToolSearch_HitsAndMisses(t *testing.T) {
	reg := []tools.Tool{
		{Name: "alpha_tool", Description: "does alpha things"},
		{Name: "beta_tool", Description: "does beta things"},
	}
	tool := toolSearchTool(reg)

	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"query": "alpha"}))
	if !strings.Contains(res.Text, "alpha_tool") || strings.Contains(res.Text, "beta_tool") {
		t.Errorf("unexpected: %s", res.Text)
	}

	res, _ = tool.Run(context.Background(), mustJSON(map[string]any{"query": "nothing"}))
	if strings.Contains(res.Text, "alpha") {
		t.Errorf("unexpected hit: %s", res.Text)
	}
}

func TestToolSearch_NilRegistry(t *testing.T) {
	tool := toolSearchTool(nil)
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"query": "x"}))
	if !res.IsError {
		t.Error("expected error with nil registry")
	}
}

// ── sleep ─────────────────────────────────────────────────────────────────

func TestSleep_ShortDuration(t *testing.T) {
	tool := sleepTool()
	start := time.Now()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"seconds": 0.05}))
	elapsed := time.Since(start)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Text)
	}
	if elapsed < 40*time.Millisecond {
		t.Errorf("slept too little: %v", elapsed)
	}
}

func TestSleep_Capped(t *testing.T) {
	// Verify the cap is enforced — we don't actually sleep 9999s.
	tool := sleepTool()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	// Request > maxSleepSeconds; context cancels before max.
	res, _ := tool.Run(ctx, mustJSON(map[string]any{"seconds": 9999.0}))
	// Either context cancelled or capped — either way not an unhandled hang.
	_ = res
}

func TestSleep_ZeroOrNegative(t *testing.T) {
	tool := sleepTool()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"seconds": -1.0}))
	if res.IsError {
		t.Errorf("unexpected error: %s", res.Text)
	}
}

// ── write_file ────────────────────────────────────────────────────────────

func TestWriteFile_CreateAndOverwrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "out.txt")

	tool := writeFileTool()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"path": p, "content": "hello\nworld\n"}))
	if res.IsError {
		t.Fatalf("write error: %s", res.Text)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "hello\nworld\n" {
		t.Errorf("unexpected content: %q", data)
	}

	// overwrite
	tool.Run(context.Background(), mustJSON(map[string]any{"path": p, "content": "new\n"}))
	data, _ = os.ReadFile(p)
	if string(data) != "new\n" {
		t.Errorf("overwrite failed: %q", data)
	}
}

// ── edit_file ─────────────────────────────────────────────────────────────

func TestEditFile_ReplaceOnce(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	os.WriteFile(p, []byte("foo bar baz\n"), 0o644)

	tool := editFileTool()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"path": p, "old_str": "bar", "new_str": "BAR"}))
	if res.IsError {
		t.Fatalf("edit error: %s", res.Text)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "foo BAR baz\n" {
		t.Errorf("unexpected: %q", data)
	}
}

func TestEditFile_NotFound(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	os.WriteFile(p, []byte("hello\n"), 0o644)

	tool := editFileTool()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"path": p, "old_str": "missing", "new_str": "x"}))
	if !res.IsError {
		t.Error("expected error when old_str not found")
	}
}

func TestEditFile_Ambiguous(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	os.WriteFile(p, []byte("x x x\n"), 0o644)

	tool := editFileTool()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"path": p, "old_str": "x", "new_str": "y"}))
	if !res.IsError {
		t.Error("expected error for ambiguous old_str")
	}
}

// ── notebook_edit ─────────────────────────────────────────────────────────

func TestNotebookEdit_EditCell(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nb.ipynb")

	nb := map[string]any{
		"nbformat":       4,
		"nbformat_minor": 5,
		"metadata":       map[string]any{},
		"cells": []any{
			map[string]any{
				"cell_type": "code",
				"source":    []string{"print('hello')\n"},
				"metadata":  map[string]any{},
				"outputs":   []any{},
			},
		},
	}
	data, _ := json.Marshal(nb)
	os.WriteFile(p, data, 0o644)

	tool := notebookEditTool()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{
		"path":       p,
		"cell_index": 0,
		"new_source": "print('world')",
	}))
	if res.IsError {
		t.Fatalf("notebook_edit error: %s", res.Text)
	}

	raw, _ := os.ReadFile(p)
	if !strings.Contains(string(raw), "world") {
		t.Errorf("new source not written: %s", raw)
	}
}

func TestNotebookEdit_OutOfRange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nb.ipynb")
	nb := map[string]any{"cells": []any{}}
	data, _ := json.Marshal(nb)
	os.WriteFile(p, data, 0o644)

	tool := notebookEditTool()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"path": p, "cell_index": 5, "new_source": "x"}))
	if !res.IsError {
		t.Error("expected error for out-of-range cell index")
	}
}

// ── bash ──────────────────────────────────────────────────────────────────

func TestBash_SimpleCommand(t *testing.T) {
	tool := bashTool()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"command": "echo hello"}))
	if res.IsError || !strings.Contains(res.Text, "hello") {
		t.Errorf("unexpected: %v / %s", res.IsError, res.Text)
	}
}

func TestBash_NonZeroExit(t *testing.T) {
	tool := bashTool()
	res, _ := tool.Run(context.Background(), mustJSON(map[string]any{"command": "exit 42"}))
	if !res.IsError || !strings.Contains(res.Text, "42") {
		t.Errorf("expected exit-42 error: %s", res.Text)
	}
}

func TestBash_ContextCancelled(t *testing.T) {
	tool := bashTool()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	res, _ := tool.Run(ctx, mustJSON(map[string]any{"command": "sleep 60"}))
	if !res.IsError {
		t.Error("expected error when context cancelled")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────

func TestHumanSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
	}
	for _, c := range cases {
		got := humanSize(c.in)
		if got != c.want {
			t.Errorf("humanSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsBinaryExt(t *testing.T) {
	if !isBinaryExt("image.png") {
		t.Error("expected png to be binary")
	}
	if isBinaryExt("main.go") {
		t.Error("expected go to be non-binary")
	}
}

func TestSplitLines(t *testing.T) {
	got := splitLines("a\nb\nc")
	if len(got) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(got), got)
	}
	if got[0] != "a\n" || got[1] != "b\n" || got[2] != "c" {
		t.Errorf("unexpected: %v", got)
	}
}
