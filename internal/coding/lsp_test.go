package coding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const sampleGo = `// Package x demonstrates Go LSP coverage.
package x

// Greet says hi to the named caller.
func Greet(name string) string {
	return "hi " + name
}

type Thing struct {
	Name string
}

// Rename mutates the Thing receiver.
func (t *Thing) Rename(name string) {
	t.Name = name
}

const Answer = 42

var Greeting = Greet("world")
`

func TestGoLSPSymbols(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "x.go", sampleGo)
	rt := NewLSPRuntime()
	syms, err := rt.Symbols(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Greet":    "func",
		"Thing":    "type",
		"Rename":   "method",
		"Answer":   "const",
		"Greeting": "var",
	}
	gotKinds := map[string]string{}
	for _, s := range syms {
		gotKinds[s.Name] = s.Kind
	}
	for n, k := range want {
		if gotKinds[n] != k {
			t.Errorf("symbol %s: want kind %s, got %s", n, k, gotKinds[n])
		}
	}
}

func TestGoLSPDefinitionAndReferences(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", sampleGo)
	writeFile(t, dir, "b.go", `package x

func use() string { return Greet("you") }
`)
	p := &GoLSPProvider{}
	defs, err := p.Definition(dir, "Greet")
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) != 1 || !strings.HasSuffix(defs[0].Path, "a.go") {
		t.Errorf("expected one Greet definition in a.go, got %+v", defs)
	}
	refs, err := p.References(dir, "Greet")
	if err != nil {
		t.Fatal(err)
	}
	// declaration + use in b.go + use in var initializer in a.go = 3
	if len(refs) < 3 {
		t.Errorf("expected >=3 Greet references, got %d: %+v", len(refs), refs)
	}
}

func TestGoLSPHover(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", sampleGo)
	p := &GoLSPProvider{}
	h, ok, err := p.Hover(dir, "Greet")
	if err != nil || !ok {
		t.Fatalf("hover Greet not found: ok=%v err=%v", ok, err)
	}
	if !strings.Contains(h.Doc, "says hi") {
		t.Errorf("hover doc missing: %+v", h)
	}
	if !strings.Contains(h.Signature, "func Greet(name string) string") {
		t.Errorf("hover signature off: %s", h.Signature)
	}
}

func TestGoLSPDiagnostics(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "broken.go", "package x\nfunc bad( { }\n")
	rt := NewLSPRuntime()
	diags, _ := rt.Diagnostics(dir)
	if len(diags) == 0 {
		t.Errorf("expected at least one diagnostic on broken file")
	}
}

func TestLSPSkipsHiddenAndVendor(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "x.go", sampleGo)
	writeFile(t, dir, "vendor/dep.go", "package dep\nfunc ShouldNotShow() {}\n")
	writeFile(t, dir, ".cache/y.go", "package y\nfunc AlsoSkip() {}\n")
	syms, err := (&GoLSPProvider{}).Symbols(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range syms {
		if s.Name == "ShouldNotShow" || s.Name == "AlsoSkip" {
			t.Errorf("vendor/hidden symbol leaked: %+v", s)
		}
	}
}

func TestRuntimeProviderFor(t *testing.T) {
	rt := NewLSPRuntime()
	if rt.ProviderFor(".go") == nil {
		t.Errorf("expected go provider")
	}
	if rt.ProviderFor("py") != nil {
		t.Errorf("expected nil for unregistered language")
	}
}
