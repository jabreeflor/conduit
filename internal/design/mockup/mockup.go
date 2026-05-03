// Package mockup turns a natural-language description into a rendered
// Conduit mockup using the design system's tokens and components.
//
// The package exposes a small, model-agnostic interface so the host can
// route the request through whichever LLM provider it prefers. The
// generator walks the model output, validates that only known component
// names and semantic token paths were used, and emits the chosen output
// formats (HTML, SVG, PNG, React scaffold, SwiftUI scaffold).
//
// PRD §12.9. Plugin API exposed as `design.mockup(description, options)`.
package mockup

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Format names a deliverable the generator can produce.
type Format string

const (
	FormatHTML    Format = "html"
	FormatSVG     Format = "svg"
	FormatPNG     Format = "png"
	FormatReact   Format = "react"
	FormatSwiftUI Format = "swiftui"
)

// AllFormats is the canonical ordering returned by Formats().
func AllFormats() []Format {
	return []Format{FormatHTML, FormatSVG, FormatPNG, FormatReact, FormatSwiftUI}
}

// Options controls a single Generate call.
type Options struct {
	// Description is the natural-language prompt. Required.
	Description string
	// Formats lists the deliverables the caller wants. Empty means HTML only.
	Formats []Format
	// Mode is the design-system mode ("dark", "light", "hc").
	// Empty defaults to "dark".
	Mode string
	// Width and Height are the target viewport dimensions in CSS px.
	// Zero values default to 1280x800.
	Width, Height int
	// DiffSource, when non-empty, instructs the generator to render
	// "the mockup that would result from applying this code diff" — see
	// PRD §12.9 mockup-from-diff. Optional.
	DiffSource string
	// History is prior turns of a conversational iteration loop. Each
	// entry is a (role, content) pair where role is "user" or "model".
	History []Turn
}

// Turn captures one message in the iteration loop.
type Turn struct {
	Role    string
	Content string
}

// Result is the rendered mockup, one entry per requested format.
type Result struct {
	// Outputs maps format -> rendered bytes (UTF-8 for text formats).
	Outputs map[Format][]byte
	// ComponentsUsed lists component names the generator referenced.
	ComponentsUsed []string
	// TokensUsed lists semantic token paths referenced.
	TokensUsed []string
}

// Model is the minimal LLM interface the generator needs. The host
// implements this against whichever provider is in scope.
type Model interface {
	// Complete returns a single string response for the prompt. The
	// generator handles parsing and validation; the model implementation
	// should not retry or post-process.
	Complete(ctx context.Context, system, user string) (string, error)
}

// ComponentCatalog enumerates valid component names the LLM may use.
// The host wires this from the design system's component registry. A
// nil catalog disables component validation (used in tests).
type ComponentCatalog interface {
	Has(name string) bool
	Names() []string
}

// TokenCatalog enumerates valid semantic token paths. Host wires from
// design/tokens.yaml. A nil catalog disables token validation.
type TokenCatalog interface {
	Has(path string) bool
	Paths() []string
}

// Generator turns descriptions into mockups.
type Generator struct {
	model      Model
	components ComponentCatalog
	tokens     TokenCatalog
}

// New constructs a Generator. The catalogs are optional — pass nil to
// skip validation (useful for tests or early scaffolding).
func New(model Model, components ComponentCatalog, tokens TokenCatalog) *Generator {
	return &Generator{model: model, components: components, tokens: tokens}
}

// Generate runs one mockup synthesis.
func (g *Generator) Generate(ctx context.Context, opts Options) (*Result, error) {
	if g.model == nil {
		return nil, errors.New("mockup: nil model")
	}
	if strings.TrimSpace(opts.Description) == "" {
		return nil, errors.New("mockup: empty description")
	}
	mode := opts.Mode
	if mode == "" {
		mode = "dark"
	}
	if mode != "dark" && mode != "light" && mode != "hc" {
		return nil, fmt.Errorf("mockup: invalid mode %q", mode)
	}
	formats := opts.Formats
	if len(formats) == 0 {
		formats = []Format{FormatHTML}
	}
	for _, f := range formats {
		if !validFormat(f) {
			return nil, fmt.Errorf("mockup: unknown format %q", f)
		}
	}

	system := buildSystemPrompt(g.components, g.tokens, mode, opts.Width, opts.Height)
	user := buildUserPrompt(opts)

	raw, err := g.model.Complete(ctx, system, user)
	if err != nil {
		return nil, fmt.Errorf("mockup: model error: %w", err)
	}
	html, err := extractHTML(raw)
	if err != nil {
		return nil, fmt.Errorf("mockup: parse model output: %w", err)
	}

	if g.components != nil {
		if missing := unknownTokens(html, "data-component=\"", g.components.Has); len(missing) > 0 {
			return nil, fmt.Errorf("mockup: model used unknown components: %s",
				strings.Join(missing, ", "))
		}
	}
	if g.tokens != nil {
		if missing := unknownTokens(html, "var(--", g.tokens.Has); len(missing) > 0 {
			return nil, fmt.Errorf("mockup: model used unknown tokens: %s",
				strings.Join(missing, ", "))
		}
	}

	res := &Result{
		Outputs:        make(map[Format][]byte, len(formats)),
		ComponentsUsed: collect(html, "data-component=\""),
		TokensUsed:     collect(html, "var(--"),
	}
	for _, f := range formats {
		out, err := emit(f, html, opts)
		if err != nil {
			return nil, err
		}
		res.Outputs[f] = out
	}
	return res, nil
}

func validFormat(f Format) bool {
	for _, ok := range AllFormats() {
		if ok == f {
			return true
		}
	}
	return false
}

func buildSystemPrompt(c ComponentCatalog, t TokenCatalog, mode string, w, h int) string {
	if w == 0 {
		w = 1280
	}
	if h == 0 {
		h = 800
	}
	var b strings.Builder
	fmt.Fprintf(&b, "You are the Conduit Design mockup generator. Render the request as one HTML fragment.\n")
	fmt.Fprintf(&b, "Viewport: %dx%d, mode: %s.\n", w, h, mode)
	b.WriteString("Use only Conduit Design components, annotated with data-component=\"<name>\".\n")
	b.WriteString("Use only semantic tokens via CSS custom properties, e.g. var(--color-fg-primary).\n")
	if c != nil {
		names := c.Names()
		sort.Strings(names)
		fmt.Fprintf(&b, "Available components: %s.\n", strings.Join(names, ", "))
	}
	if t != nil {
		paths := t.Paths()
		sort.Strings(paths)
		if len(paths) > 80 {
			paths = paths[:80]
		}
		fmt.Fprintf(&b, "Token examples (incomplete): %s.\n", strings.Join(paths, ", "))
	}
	b.WriteString("Wrap the fragment in <html><body>…</body></html>. No commentary.\n")
	return b.String()
}

func buildUserPrompt(opts Options) string {
	var b strings.Builder
	for _, t := range opts.History {
		fmt.Fprintf(&b, "[%s] %s\n", t.Role, t.Content)
	}
	if opts.DiffSource != "" {
		b.WriteString("Mockup-from-diff. Render the UI that results from applying this code change:\n")
		b.WriteString(opts.DiffSource)
		b.WriteString("\n")
	}
	b.WriteString(opts.Description)
	return b.String()
}

// extractHTML pulls the first <html>…</html> block out of the model's
// reply. We accept either a raw fragment or a fenced code block.
func extractHTML(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "<html"); i >= 0 {
		j := strings.LastIndex(s, "</html>")
		if j > i {
			return s[i : j+len("</html>")], nil
		}
	}
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```html")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
		if strings.Contains(s, "<html") {
			return s, nil
		}
	}
	return "", errors.New("no <html> block in model output")
}

// unknownTokens scans html for occurrences of marker followed by a name,
// returns the names that fail isKnown. Used for both component and token
// validation by varying marker.
func unknownTokens(html, marker string, isKnown func(string) bool) []string {
	seen := map[string]bool{}
	var out []string
	rest := html
	for {
		i := strings.Index(rest, marker)
		if i < 0 {
			break
		}
		rest = rest[i+len(marker):]
		end := strings.IndexAny(rest, "\")\t\n ")
		if end < 0 {
			break
		}
		name := rest[:end]
		if name == "" || seen[name] {
			rest = rest[end:]
			continue
		}
		seen[name] = true
		if !isKnown(name) {
			out = append(out, name)
		}
		rest = rest[end:]
	}
	sort.Strings(out)
	return out
}

func collect(html, marker string) []string {
	seen := map[string]bool{}
	var out []string
	rest := html
	for {
		i := strings.Index(rest, marker)
		if i < 0 {
			break
		}
		rest = rest[i+len(marker):]
		end := strings.IndexAny(rest, "\")\t\n ")
		if end < 0 {
			break
		}
		name := rest[:end]
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
		rest = rest[end:]
	}
	sort.Strings(out)
	return out
}

// emit converts the validated HTML fragment to the requested format.
//
// HTML and the React/SwiftUI scaffolds produce real output. SVG and PNG
// are integration TODOs — they require a headless renderer (Playwright /
// chromedp / wkhtmltopdf) which the host does not yet bundle. Those
// formats return a small placeholder so callers can still wire up the
// API surface end-to-end.
func emit(f Format, html string, opts Options) ([]byte, error) {
	switch f {
	case FormatHTML:
		return []byte(html), nil
	case FormatReact:
		return []byte(reactScaffold(html)), nil
	case FormatSwiftUI:
		return []byte(swiftUIScaffold()), nil
	case FormatSVG:
		// TODO: render html via headless browser and capture as SVG.
		return []byte(placeholderSVG(opts)), nil
	case FormatPNG:
		// TODO: same as SVG but rasterized.
		return []byte("conduit-mockup-png-placeholder"), nil
	}
	return nil, fmt.Errorf("emit: unknown format %q", f)
}

func reactScaffold(html string) string {
	// Minimal scaffold. The HTML body is preserved as a comment so a
	// human (or follow-up codegen pass) can translate it into idiomatic
	// JSX. We deliberately do NOT inject the HTML at runtime to avoid
	// requiring a sanitizer the scaffold consumer may not have.
	return "// Generated by @conduit/design mockup. Translate the markup below into JSX.\n" +
		"// --- begin mockup HTML ---\n" +
		commentBlock(html) +
		"// --- end mockup HTML ---\n" +
		"export function Mockup() {\n" +
		"  return <div data-mockup=\"conduit\" />;\n" +
		"}\n"
}

func commentBlock(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		b.WriteString("// ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func swiftUIScaffold() string {
	return "// Generated by ConduitDesign mockup. Replace with native views once translated.\n" +
		"import SwiftUI\n\n" +
		"struct Mockup: View {\n" +
		"    var body: some View {\n" +
		"        Text(\"Conduit mockup preview\")\n" +
		"    }\n" +
		"}\n"
}

func placeholderSVG(opts Options) string {
	w, h := opts.Width, opts.Height
	if w == 0 {
		w = 1280
	}
	if h == 0 {
		h = 800
	}
	return fmt.Sprintf(
		"<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%d\" height=\"%d\">"+
			"<rect width=\"100%%\" height=\"100%%\" fill=\"#10131E\"/>"+
			"<text x=\"50%%\" y=\"50%%\" fill=\"#FFFFFF\" text-anchor=\"middle\" "+
			"font-family=\"sans-serif\" font-size=\"24\">Mockup placeholder — render not wired</text>"+
			"</svg>", w, h)
}
