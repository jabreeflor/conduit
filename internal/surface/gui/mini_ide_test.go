package gui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeMiniIDEFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func newTestMiniIDE(t *testing.T) (*MiniIDE, string) {
	t.Helper()
	root := t.TempDir()
	ide, err := NewMiniIDE(root)
	if err != nil {
		t.Fatal(err)
	}
	return ide, root
}

func TestMiniIDE_OpenFileCreatesActiveTab(t *testing.T) {
	ide, root := newTestMiniIDE(t)
	path := writeMiniIDEFile(t, root, "internal/app.go", "package main\n\nfunc main() {}\n")

	tab, err := ide.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if tab.Path != path {
		t.Errorf("Path = %q, want %q", tab.Path, path)
	}
	if tab.Language != "go" {
		t.Errorf("Language = %q, want go", tab.Language)
	}
	if !tab.SoftWrap {
		t.Error("SoftWrap should default to true")
	}
	if tab.FontSize != DefaultEditorFontSize {
		t.Errorf("FontSize = %d, want %d", tab.FontSize, DefaultEditorFontSize)
	}
	if !ide.Visible() {
		t.Error("OpenFile should make the Mini IDE visible")
	}
	active := ide.ActiveTab()
	if active == nil || active.Path != path {
		t.Fatalf("ActiveTab() = %#v, want %q", active, path)
	}
}

func TestMiniIDE_UpdateSaveAndTabOptions(t *testing.T) {
	ide, root := newTestMiniIDE(t)
	path := writeMiniIDEFile(t, root, "README.md", "# Old\n")

	if _, err := ide.OpenFile("README.md"); err != nil {
		t.Fatal(err)
	}
	if err := ide.UpdateBuffer(path, "# New\n"); err != nil {
		t.Fatal(err)
	}
	if err := ide.SetSoftWrap(path, false); err != nil {
		t.Fatal(err)
	}
	if err := ide.SetFontSize(path, 16); err != nil {
		t.Fatal(err)
	}
	if err := ide.SetMinimap(path, true); err != nil {
		t.Fatal(err)
	}
	tab := ide.ActiveTab()
	if !tab.Dirty {
		t.Error("UpdateBuffer should mark tab dirty")
	}
	if tab.SoftWrap {
		t.Error("SoftWrap = true, want false")
	}
	if tab.FontSize != 16 {
		t.Errorf("FontSize = %d, want 16", tab.FontSize)
	}
	if !tab.MinimapEnabled {
		t.Error("MinimapEnabled = false, want true")
	}

	if err := ide.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# New\n" {
		t.Errorf("saved content = %q", got)
	}
	if ide.ActiveTab().Dirty {
		t.Error("Save should clear Dirty")
	}
}

func TestMiniIDE_RejectsPathsOutsideRoot(t *testing.T) {
	ide, root := newTestMiniIDE(t)
	outside := filepath.Join(filepath.Dir(root), "outside.go")
	if err := os.WriteFile(outside, []byte("package outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })

	if _, err := ide.OpenFile(outside); err == nil {
		t.Fatal("OpenFile outside root returned nil error")
	}
	if err := ide.UpdateBuffer("../outside.go", "x"); err == nil {
		t.Fatal("UpdateBuffer outside root returned nil error")
	}
}

func TestMiniIDE_BuildFileTreeIsSortedAndLanguageAware(t *testing.T) {
	ide, root := newTestMiniIDE(t)
	writeMiniIDEFile(t, root, "zeta.go", "package zeta\n")
	writeMiniIDEFile(t, root, "cmd/main.ts", "export const main = true\n")
	writeMiniIDEFile(t, root, "alpha.md", "# Alpha\n")
	writeMiniIDEFile(t, root, ".hidden/secret.go", "package hidden\n")

	tree, err := ide.BuildFileTree(3, 20)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, child := range tree.Children {
		names = append(names, child.Name)
	}
	wantNames := []string{"cmd", "alpha.md", "zeta.go"}
	if !reflect.DeepEqual(names, wantNames) {
		t.Fatalf("children = %#v, want %#v", names, wantNames)
	}
	if tree.Children[0].Children[0].Language != "typescript" {
		t.Errorf("cmd/main.ts language = %q, want typescript", tree.Children[0].Children[0].Language)
	}
}

func TestMiniIDE_CompletionsIncludeKeywordsAndSymbols(t *testing.T) {
	ide, root := newTestMiniIDE(t)
	path := writeMiniIDEFile(t, root, "main.go", "package main\n\nfunc renderPanel() {}\nvar retryCount int\n")
	if _, err := ide.OpenFile(path); err != nil {
		t.Fatal(err)
	}

	got, err := ide.Completions(path, "re", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"renderPanel", "retryCount", "return"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Completions = %#v, want %#v", got, want)
	}
}

func TestMiniIDE_SuggestionsCanBeResolved(t *testing.T) {
	ide, root := newTestMiniIDE(t)
	path := writeMiniIDEFile(t, root, "main.go", "package main\n")

	err := ide.AddSuggestion(AISuggestion{
		ID:          "s1",
		Path:        path,
		StartLine:   1,
		EndLine:     1,
		Replacement: "package conduit\n",
		Explanation: "rename package",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ide.ResolveSuggestion("s1", SuggestionAccepted); err != nil {
		t.Fatal(err)
	}
	suggestions, err := ide.SuggestionsForFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("len suggestions = %d, want 1", len(suggestions))
	}
	if suggestions[0].Status != SuggestionAccepted {
		t.Errorf("Status = %v, want SuggestionAccepted", suggestions[0].Status)
	}
}

func TestMiniIDE_TerminalSplits(t *testing.T) {
	ide, root := newTestMiniIDE(t)
	if err := ide.AddTerminal("test", "Tests", root, "/bin/zsh"); err != nil {
		t.Fatal(err)
	}
	if err := ide.AppendTerminalOutput("test", "$ go test ./...", "ok ./internal/gui"); err != nil {
		t.Fatal(err)
	}
	if err := ide.SetTerminalCollapsed(DefaultTerminalID, true); err != nil {
		t.Fatal(err)
	}

	panes := ide.Terminals()
	if len(panes) != 2 {
		t.Fatalf("len terminals = %d, want 2", len(panes))
	}
	if !panes[0].Collapsed {
		t.Error("default terminal should be collapsed")
	}
	if !panes[1].Active {
		t.Error("new terminal should be active")
	}
	if got := panes[1].Lines; !reflect.DeepEqual(got, []string{"$ go test ./...", "ok ./internal/gui"}) {
		t.Errorf("terminal lines = %#v", got)
	}
}

func TestMiniIDE_ExternalLaunchesFilterAndSubstitutePath(t *testing.T) {
	ide, root := newTestMiniIDE(t)
	md := writeMiniIDEFile(t, root, "notes/todo.md", "# Todo\n")
	goFile := writeMiniIDEFile(t, root, "main.go", "package main\n")

	mdLaunches, err := ide.ExternalLaunches(md)
	if err != nil {
		t.Fatal(err)
	}
	var mdNames []string
	for _, launch := range mdLaunches {
		mdNames = append(mdNames, launch.Name)
	}
	if !reflect.DeepEqual(mdNames, []string{"VS Code", "Obsidian"}) {
		t.Fatalf("markdown launches = %#v", mdNames)
	}
	if !strings.Contains(mdLaunches[1].Args[0], md) {
		t.Errorf("Obsidian args = %#v, want substituted path %q", mdLaunches[1].Args, md)
	}

	goLaunches, err := ide.ExternalLaunches(goFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(goLaunches) != 1 || goLaunches[0].Name != "VS Code" {
		t.Fatalf("go launches = %#v, want only VS Code", goLaunches)
	}
}
