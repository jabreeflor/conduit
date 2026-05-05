// Package svg generates illustrations, diagrams, and icons by treating
// the LLM as the image generator — the model writes SVG XML directly.
//
// PRD §12.10. Plugin APIs:
//
//	design.svg.generate(description, opts)
//	design.svg.animate(svg, opts)
//	design.svg.export(svg, format)
//	design.diagram(kind, source, opts)
//	design.icon(name, opts)
//
// Style and diagram presets live in this package; the model is steered
// via system prompts derived from the chosen preset. Output is parsed
// against a complexity budget, validated for accessibility metadata,
// and (where possible) optimized via a built-in SVGO-equivalent pass.
package svg

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Style enumerates illustration presets recognized by Generate.
type Style string

const (
	StyleFlat      Style = "flat"
	StyleIsometric Style = "isometric"
	StyleLineArt   Style = "line-art"
	StyleDuotone   Style = "duotone"
	StyleGradient  Style = "gradient"
	StyleHandDrawn Style = "hand-drawn"
	StyleBlueprint Style = "blueprint"
)

// AllStyles returns the canonical preset list.
func AllStyles() []Style {
	return []Style{
		StyleFlat, StyleIsometric, StyleLineArt, StyleDuotone,
		StyleGradient, StyleHandDrawn, StyleBlueprint,
	}
}

// DiagramKind enumerates structured-diagram presets.
type DiagramKind string

const (
	DiagramArchitecture DiagramKind = "architecture"
	DiagramFlowchart    DiagramKind = "flowchart"
	DiagramSequence     DiagramKind = "sequence"
	DiagramER           DiagramKind = "er"
	DiagramNetwork      DiagramKind = "network"
	DiagramState        DiagramKind = "state"
	DiagramGantt        DiagramKind = "gantt"
)

// AllDiagramKinds returns the canonical kind list.
func AllDiagramKinds() []DiagramKind {
	return []DiagramKind{
		DiagramArchitecture, DiagramFlowchart, DiagramSequence,
		DiagramER, DiagramNetwork, DiagramState, DiagramGantt,
	}
}

// ExportFormat enumerates export targets.
type ExportFormat string

const (
	ExportSVG     ExportFormat = "svg"
	ExportPNG     ExportFormat = "png"
	ExportPDF     ExportFormat = "pdf"
	ExportLottie  ExportFormat = "lottie"
	ExportICO     ExportFormat = "ico"
	ExportReact   ExportFormat = "react"
	ExportSwiftUI ExportFormat = "swiftui"
)

// Options for Generate.
type Options struct {
	Style Style
	// Title and Description are written into <title> and <desc> for a11y.
	Title       string
	Description string
	// Width, Height in user-space units. Default 512x512.
	Width, Height int
	// MaxBytes is the complexity budget. Output exceeding it is rejected.
	// Zero means default (32 KB).
	MaxBytes int
	// Animate, when true, asks the model to include CSS keyframes or SMIL.
	Animate bool
}

// Generated is the output of Generate / Diagram / Icon.
type Generated struct {
	SVG          string
	Bytes        int
	StyleApplied Style
}

// Model is the minimal LLM interface. The host implements this against
// whichever provider is in scope.
type Model interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// Generator owns the model and configuration.
type Generator struct {
	model Model
}

// New constructs a Generator.
func New(model Model) *Generator {
	return &Generator{model: model}
}

// Generate produces an illustration from a natural-language description.
func (g *Generator) Generate(ctx context.Context, description string, opts Options) (*Generated, error) {
	if g.model == nil {
		return nil, errors.New("svg: nil model")
	}
	if strings.TrimSpace(description) == "" {
		return nil, errors.New("svg: empty description")
	}
	style := opts.Style
	if style == "" {
		style = StyleFlat
	}
	if !validStyle(style) {
		return nil, fmt.Errorf("svg: unknown style %q", style)
	}
	w, h := dims(opts.Width, opts.Height)
	budget := opts.MaxBytes
	if budget == 0 {
		budget = 32 * 1024
	}

	system := stylePrompt(style, w, h, opts.Animate)
	user := userPrompt(description, opts.Title, opts.Description)

	raw, err := g.model.Complete(ctx, system, user)
	if err != nil {
		return nil, fmt.Errorf("svg: model error: %w", err)
	}
	out, err := extractSVG(raw)
	if err != nil {
		return nil, err
	}
	out = ensureAccessibility(out, opts.Title, opts.Description)
	out = optimize(out)
	if len(out) > budget {
		return nil, fmt.Errorf("svg: output %d bytes exceeds budget %d", len(out), budget)
	}
	return &Generated{SVG: out, Bytes: len(out), StyleApplied: style}, nil
}

// Diagram renders a structured diagram from a textual source (DSL,
// Mermaid, plain bullet list — the prompt accommodates any). The kind
// drives the layout-and-symbol vocabulary the model is told to use.
func (g *Generator) Diagram(ctx context.Context, kind DiagramKind, source string, opts Options) (*Generated, error) {
	if !validKind(kind) {
		return nil, fmt.Errorf("svg: unknown diagram kind %q", kind)
	}
	desc := fmt.Sprintf("%s diagram. Source:\n%s", kind, source)
	if opts.Style == "" {
		opts.Style = StyleFlat
	}
	return g.Generate(ctx, desc, opts)
}

// Icon renders a named icon at the requested size. The name is passed
// through to the model so it can choose between drawing fresh or
// adapting a known glyph.
func (g *Generator) Icon(ctx context.Context, name string, opts Options) (*Generated, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("svg: empty icon name")
	}
	if opts.Width == 0 {
		opts.Width = 24
	}
	if opts.Height == 0 {
		opts.Height = 24
	}
	if opts.Style == "" {
		opts.Style = StyleLineArt
	}
	return g.Generate(ctx, "Icon: "+name, opts)
}

// Animate wraps an existing SVG with a keyframe animation block. The
// caller chooses CSS or SMIL via opts; this scaffold uses CSS by
// default. Returned SVG embeds a <style> child with the keyframes.
func (g *Generator) Animate(ctx context.Context, svgIn string, opts Options) (*Generated, error) {
	if !strings.Contains(svgIn, "<svg") {
		return nil, errors.New("svg: input is not an SVG")
	}
	style := `<style>@keyframes conduit-spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } } svg { animation: conduit-spin 4s linear infinite; }</style>`
	out := strings.Replace(svgIn, ">", ">"+style, 1)
	return &Generated{SVG: out, Bytes: len(out), StyleApplied: opts.Style}, nil
}

// Export converts an SVG into the requested format.
//
// Native SVG is a passthrough. React/SwiftUI emit thin scaffolds.
// PNG/PDF/Lottie/ICO are integration TODOs — they require a renderer
// (rsvg, librsvg, or a headless browser) the host does not yet bundle.
func (g *Generator) Export(svgIn string, format ExportFormat) ([]byte, error) {
	switch format {
	case ExportSVG:
		return []byte(svgIn), nil
	case ExportReact:
		return []byte(reactSVGScaffold(svgIn)), nil
	case ExportSwiftUI:
		return []byte(swiftUISVGScaffold()), nil
	case ExportPNG, ExportPDF, ExportLottie, ExportICO:
		// TODO: bundle a renderer.
		return []byte("conduit-svg-export-placeholder:" + string(format)), nil
	}
	return nil, fmt.Errorf("svg: unknown export format %q", format)
}

// --- internals ---------------------------------------------------------------

func validStyle(s Style) bool {
	for _, ok := range AllStyles() {
		if ok == s {
			return true
		}
	}
	return false
}

func validKind(k DiagramKind) bool {
	for _, ok := range AllDiagramKinds() {
		if ok == k {
			return true
		}
	}
	return false
}

func dims(w, h int) (int, int) {
	if w == 0 {
		w = 512
	}
	if h == 0 {
		h = 512
	}
	return w, h
}

func stylePrompt(s Style, w, h int, animate bool) string {
	var b strings.Builder
	b.WriteString("You are the Conduit Design SVG generator. Output a single self-contained <svg> document.\n")
	fmt.Fprintf(&b, "Viewport: %dx%d units. Style preset: %s.\n", w, h, s)
	b.WriteString(stylePresetGuide(s))
	if animate {
		b.WriteString("Include CSS @keyframes (preferred) or SMIL animation tags so the result is animated.\n")
	}
	b.WriteString("Always include <title> and <desc> children for accessibility.\n")
	b.WriteString("Use only Conduit semantic CSS variables (var(--color-...)) for fills/strokes.\n")
	b.WriteString("Reply with the SVG document only — no commentary, no fences.\n")
	return b.String()
}

func stylePresetGuide(s Style) string {
	switch s {
	case StyleFlat:
		return "Flat: solid fills, no gradients, no shadows.\n"
	case StyleIsometric:
		return "Isometric: 30-degree axes, parallel projection, no perspective foreshortening.\n"
	case StyleLineArt:
		return "Line art: stroked paths only, no fills, uniform stroke width.\n"
	case StyleDuotone:
		return "Duotone: exactly two semantic colors plus white/transparent.\n"
	case StyleGradient:
		return "Gradient: linear or radial gradients allowed; keep palette to <=3 hues.\n"
	case StyleHandDrawn:
		return "Hand-drawn: rough, slightly irregular paths; emulate pencil texture via stroke-dasharray.\n"
	case StyleBlueprint:
		return "Blueprint: white-on-blue grid, technical-drawing aesthetic.\n"
	}
	return ""
}

func userPrompt(description, title, desc string) string {
	var b strings.Builder
	if title != "" {
		fmt.Fprintf(&b, "Title: %s\n", title)
	}
	if desc != "" {
		fmt.Fprintf(&b, "Description: %s\n", desc)
	}
	b.WriteString(description)
	return b.String()
}

var svgRe = regexp.MustCompile(`(?s)<svg.*?</svg>`)

func extractSVG(raw string) (string, error) {
	m := svgRe.FindString(raw)
	if m == "" {
		return "", errors.New("svg: no <svg>…</svg> block in model output")
	}
	return m, nil
}

// ensureAccessibility guarantees <title> (and <desc> if provided) are
// present as the first children of <svg>.
func ensureAccessibility(in, title, desc string) string {
	if title == "" && desc == "" {
		return in
	}
	// Insert after the opening <svg ...> tag.
	end := strings.Index(in, ">")
	if end < 0 {
		return in
	}
	insert := ""
	if title != "" && !strings.Contains(in, "<title>") {
		insert += "<title>" + escapeXML(title) + "</title>"
	}
	if desc != "" && !strings.Contains(in, "<desc>") {
		insert += "<desc>" + escapeXML(desc) + "</desc>"
	}
	if insert == "" {
		return in
	}
	return in[:end+1] + insert + in[end+1:]
}

// optimize is a stand-in for SVGO. It collapses runs of whitespace
// between tags and trims leading/trailing space. A full SVGO port is
// out of scope for the scaffold.
func optimize(in string) string {
	in = strings.TrimSpace(in)
	in = collapseSpaceBetweenTags(in)
	return in
}

func collapseSpaceBetweenTags(in string) string {
	re := regexp.MustCompile(`>\s+<`)
	return re.ReplaceAllString(in, "><")
}

func escapeXML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}

func reactSVGScaffold(svgIn string) string {
	// The actual SVG body is preserved as a comment so a follow-up
	// codegen pass can convert SVG attributes to JSX (camelCase, className, …).
	var b strings.Builder
	b.WriteString("// Generated by @conduit/design svg.\n")
	b.WriteString("// --- begin SVG ---\n")
	for _, line := range strings.Split(svgIn, "\n") {
		b.WriteString("// ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("// --- end SVG ---\n")
	b.WriteString("export function Illustration() {\n")
	b.WriteString("  return null; // replace with translated JSX SVG\n")
	b.WriteString("}\n")
	return b.String()
}

func swiftUISVGScaffold() string {
	return "// Generated by ConduitDesign svg. SwiftUI does not render SVG natively;\n" +
		"// translate the document into Path/Shape calls or load via a third-party library.\n" +
		"import SwiftUI\n\nstruct Illustration: View { var body: some View { EmptyView() } }\n"
}

// ListStyles returns the string names of all style presets, sorted.
// Useful for surfacing in plugin help / UI.
func ListStyles() []string {
	out := make([]string, 0, len(AllStyles()))
	for _, s := range AllStyles() {
		out = append(out, string(s))
	}
	sort.Strings(out)
	return out
}

// ListDiagramKinds is the diagram-kind equivalent of ListStyles.
func ListDiagramKinds() []string {
	out := make([]string, 0, len(AllDiagramKinds()))
	for _, k := range AllDiagramKinds() {
		out = append(out, string(k))
	}
	sort.Strings(out)
	return out
}
