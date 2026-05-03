package svg

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeModel struct {
	reply string
	err   error
	gotS  string
	gotU  string
}

func (f *fakeModel) Complete(_ context.Context, system, user string) (string, error) {
	f.gotS, f.gotU = system, user
	return f.reply, f.err
}

const validSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><circle r="10" cx="12" cy="12"/></svg>`

func TestGenerate_HappyPath(t *testing.T) {
	m := &fakeModel{reply: "noise " + validSVG + " trailing"}
	g := New(m)
	out, err := g.Generate(context.Background(), "a circle", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.SVG, "<svg") || !strings.HasSuffix(out.SVG, "</svg>") {
		t.Errorf("unexpected SVG: %q", out.SVG)
	}
	if out.StyleApplied != StyleFlat {
		t.Errorf("default style = %v, want flat", out.StyleApplied)
	}
}

func TestGenerate_Validations(t *testing.T) {
	g := New(&fakeModel{reply: validSVG})
	cases := []struct {
		name string
		desc string
		opts Options
		want string
	}{
		{"empty desc", "", Options{}, "empty description"},
		{"bad style", "x", Options{Style: "neon"}, "unknown style"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := g.Generate(context.Background(), tc.desc, tc.opts)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestGenerate_NoModel(t *testing.T) {
	g := New(nil)
	_, err := g.Generate(context.Background(), "x", Options{})
	if err == nil || !strings.Contains(err.Error(), "nil model") {
		t.Fatalf("err = %v", err)
	}
}

func TestGenerate_ModelError(t *testing.T) {
	g := New(&fakeModel{err: errors.New("boom")})
	_, err := g.Generate(context.Background(), "x", Options{})
	if err == nil || !strings.Contains(err.Error(), "model error") {
		t.Fatalf("err = %v", err)
	}
}

func TestGenerate_NoSVG(t *testing.T) {
	g := New(&fakeModel{reply: "no svg here"})
	_, err := g.Generate(context.Background(), "x", Options{})
	if err == nil || !strings.Contains(err.Error(), "no <svg>") {
		t.Fatalf("err = %v", err)
	}
}

func TestGenerate_BudgetExceeded(t *testing.T) {
	huge := "<svg>" + strings.Repeat("a", 200) + "</svg>"
	g := New(&fakeModel{reply: huge})
	_, err := g.Generate(context.Background(), "x", Options{MaxBytes: 50})
	if err == nil || !strings.Contains(err.Error(), "exceeds budget") {
		t.Fatalf("err = %v", err)
	}
}

func TestGenerate_AccessibilityInjected(t *testing.T) {
	g := New(&fakeModel{reply: validSVG})
	out, err := g.Generate(context.Background(), "x", Options{
		Title:       "Cat",
		Description: "An orange cat",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.SVG, "<title>Cat</title>") {
		t.Errorf("title not injected: %q", out.SVG)
	}
	if !strings.Contains(out.SVG, "<desc>An orange cat</desc>") {
		t.Errorf("desc not injected: %q", out.SVG)
	}
}

func TestGenerate_AccessibilityEscaped(t *testing.T) {
	g := New(&fakeModel{reply: validSVG})
	out, _ := g.Generate(context.Background(), "x", Options{Title: `<x"&y>`})
	if !strings.Contains(out.SVG, "&lt;x&quot;&amp;y&gt;") {
		t.Errorf("title not XML-escaped: %q", out.SVG)
	}
}

func TestStylePresetGuide(t *testing.T) {
	for _, s := range AllStyles() {
		if stylePresetGuide(s) == "" {
			t.Errorf("missing guide for style %q", s)
		}
	}
}

func TestDiagram(t *testing.T) {
	m := &fakeModel{reply: validSVG}
	g := New(m)
	_, err := g.Diagram(context.Background(), DiagramFlowchart, "A -> B", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.gotU, "flowchart") {
		t.Errorf("user prompt missing kind: %q", m.gotU)
	}
}

func TestDiagram_BadKind(t *testing.T) {
	g := New(&fakeModel{reply: validSVG})
	_, err := g.Diagram(context.Background(), "uml", "x", Options{})
	if err == nil || !strings.Contains(err.Error(), "unknown diagram kind") {
		t.Fatalf("err = %v", err)
	}
}

func TestIcon(t *testing.T) {
	m := &fakeModel{reply: validSVG}
	g := New(m)
	out, err := g.Icon(context.Background(), "search", Options{})
	if err != nil || out == nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.gotU, "search") {
		t.Errorf("user prompt missing icon name: %q", m.gotU)
	}
}

func TestIcon_Empty(t *testing.T) {
	g := New(&fakeModel{reply: validSVG})
	_, err := g.Icon(context.Background(), "", Options{})
	if err == nil {
		t.Fatal("expected error for empty icon name")
	}
}

func TestAnimate(t *testing.T) {
	g := New(&fakeModel{})
	out, err := g.Animate(context.Background(), validSVG, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.SVG, "@keyframes") {
		t.Errorf("animation not injected: %q", out.SVG)
	}
}

func TestAnimate_NotSVG(t *testing.T) {
	g := New(&fakeModel{})
	_, err := g.Animate(context.Background(), "<html>", Options{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExport_AllFormats(t *testing.T) {
	g := New(&fakeModel{})
	formats := []ExportFormat{ExportSVG, ExportPNG, ExportPDF, ExportLottie, ExportICO, ExportReact, ExportSwiftUI}
	for _, f := range formats {
		t.Run(string(f), func(t *testing.T) {
			out, err := g.Export(validSVG, f)
			if err != nil {
				t.Fatal(err)
			}
			if len(out) == 0 {
				t.Error("empty output")
			}
		})
	}
}

func TestExport_BadFormat(t *testing.T) {
	g := New(&fakeModel{})
	_, err := g.Export(validSVG, "bmp")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListStyles(t *testing.T) {
	if len(ListStyles()) != len(AllStyles()) {
		t.Errorf("ListStyles count mismatch")
	}
	if len(ListDiagramKinds()) != len(AllDiagramKinds()) {
		t.Errorf("ListDiagramKinds count mismatch")
	}
}

func TestOptimize_CollapsesWhitespace(t *testing.T) {
	in := "<svg>\n  <g>\n    <circle/>\n  </g>\n</svg>"
	if out := optimize(in); strings.Contains(out, "  ") || strings.Contains(out, "\n") {
		t.Errorf("optimize did not collapse whitespace: %q", out)
	}
}
