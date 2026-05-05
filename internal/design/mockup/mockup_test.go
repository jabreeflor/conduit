package mockup

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

type setCatalog struct{ ok map[string]bool }

func (s setCatalog) Has(n string) bool { return s.ok[n] }
func (s setCatalog) Names() []string {
	out := make([]string, 0, len(s.ok))
	for k := range s.ok {
		out = append(out, k)
	}
	return out
}
func (s setCatalog) Paths() []string { return s.Names() }

const validReply = `<html><body>` +
	`<div data-component="button" style="background:var(--color-bg-accent)">Run</div>` +
	`</body></html>`

func TestGenerate_HTMLDefault(t *testing.T) {
	m := &fakeModel{reply: validReply}
	g := New(m, nil, nil)
	res, err := g.Generate(context.Background(), Options{Description: "primary button"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.Outputs[FormatHTML]; !ok {
		t.Fatal("missing HTML output")
	}
	if got := string(res.Outputs[FormatHTML]); !strings.Contains(got, "data-component=\"button\"") {
		t.Errorf("HTML missing component marker: %q", got)
	}
	if len(res.ComponentsUsed) != 1 || res.ComponentsUsed[0] != "button" {
		t.Errorf("ComponentsUsed = %v, want [button]", res.ComponentsUsed)
	}
	if len(res.TokensUsed) != 1 || res.TokensUsed[0] != "color-bg-accent" {
		t.Errorf("TokensUsed = %v, want [color-bg-accent]", res.TokensUsed)
	}
}

func TestGenerate_AllFormats(t *testing.T) {
	g := New(&fakeModel{reply: validReply}, nil, nil)
	res, err := g.Generate(context.Background(), Options{
		Description: "x",
		Formats:     AllFormats(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range AllFormats() {
		if _, ok := res.Outputs[f]; !ok {
			t.Errorf("missing format %q", f)
		}
	}
	if !strings.Contains(string(res.Outputs[FormatReact]), "export function Mockup") {
		t.Error("react scaffold malformed")
	}
	if !strings.Contains(string(res.Outputs[FormatSwiftUI]), "struct Mockup") {
		t.Error("swiftui scaffold malformed")
	}
	if !strings.Contains(string(res.Outputs[FormatSVG]), "<svg") {
		t.Error("svg placeholder malformed")
	}
}

func TestGenerate_RejectsUnknownComponent(t *testing.T) {
	cat := setCatalog{ok: map[string]bool{"card": true}}
	g := New(&fakeModel{reply: validReply}, cat, nil)
	_, err := g.Generate(context.Background(), Options{Description: "x"})
	if err == nil || !strings.Contains(err.Error(), "unknown components") {
		t.Fatalf("expected unknown-component error, got %v", err)
	}
}

func TestGenerate_RejectsUnknownToken(t *testing.T) {
	tok := setCatalog{ok: map[string]bool{"color-fg-primary": true}}
	g := New(&fakeModel{reply: validReply}, nil, tok)
	_, err := g.Generate(context.Background(), Options{Description: "x"})
	if err == nil || !strings.Contains(err.Error(), "unknown tokens") {
		t.Fatalf("expected unknown-token error, got %v", err)
	}
}

func TestGenerate_AcceptsKnownComponentsAndTokens(t *testing.T) {
	cat := setCatalog{ok: map[string]bool{"button": true}}
	tok := setCatalog{ok: map[string]bool{"color-bg-accent": true}}
	g := New(&fakeModel{reply: validReply}, cat, tok)
	if _, err := g.Generate(context.Background(), Options{Description: "x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGenerate_ValidatesInputs(t *testing.T) {
	g := New(&fakeModel{reply: validReply}, nil, nil)
	cases := []struct {
		name string
		opts Options
		want string
	}{
		{"empty desc", Options{}, "empty description"},
		{"bad mode", Options{Description: "x", Mode: "neon"}, "invalid mode"},
		{"bad format", Options{Description: "x", Formats: []Format{"gif"}}, "unknown format"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := g.Generate(context.Background(), tc.opts)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want contains %q", err, tc.want)
			}
		})
	}
}

func TestGenerate_ModelError(t *testing.T) {
	g := New(&fakeModel{err: errors.New("boom")}, nil, nil)
	_, err := g.Generate(context.Background(), Options{Description: "x"})
	if err == nil || !strings.Contains(err.Error(), "model error") {
		t.Fatalf("err = %v, want model error", err)
	}
}

func TestGenerate_NoModel(t *testing.T) {
	g := New(nil, nil, nil)
	_, err := g.Generate(context.Background(), Options{Description: "x"})
	if err == nil || !strings.Contains(err.Error(), "nil model") {
		t.Fatalf("err = %v, want nil model", err)
	}
}

func TestExtractHTML_FencedBlock(t *testing.T) {
	wrapped := "```html\n" + validReply + "\n```"
	g := New(&fakeModel{reply: wrapped}, nil, nil)
	if _, err := g.Generate(context.Background(), Options{Description: "x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractHTML_Missing(t *testing.T) {
	g := New(&fakeModel{reply: "no html here"}, nil, nil)
	_, err := g.Generate(context.Background(), Options{Description: "x"})
	if err == nil || !strings.Contains(err.Error(), "no <html>") {
		t.Fatalf("err = %v, want no <html>", err)
	}
}

func TestPromptIncludesHistoryAndDiff(t *testing.T) {
	m := &fakeModel{reply: validReply}
	g := New(m, nil, nil)
	_, err := g.Generate(context.Background(), Options{
		Description: "current",
		History:     []Turn{{Role: "user", Content: "earlier"}},
		DiffSource:  "+ added line",
		Mode:        "light",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.gotU, "earlier") {
		t.Error("user prompt missing history")
	}
	if !strings.Contains(m.gotU, "+ added line") {
		t.Error("user prompt missing diff")
	}
	if !strings.Contains(m.gotS, "mode: light") {
		t.Error("system prompt missing mode")
	}
}
