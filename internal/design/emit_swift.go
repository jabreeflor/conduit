package design

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// EmitSwift renders semantic color tokens as a SwiftUI extension.
// Reference scale is intentionally omitted — Swift consumers should use
// semantic names. Modes become a `ConduitTheme` enum; colors are resolved
// via `Color.conduit(.surface.canvas, theme: .dark)` style API.
func EmitSwift(t *Tokens) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Conduit Design Tokens — generated %s. Do not edit by hand.\n", time.Now().UTC().Format("2006-01-02"))
	b.WriteString("// Source: design/tokens.yaml. Regenerate with `make tokens`.\n\n")
	b.WriteString("import SwiftUI\n\n")

	b.WriteString("public enum ConduitTheme: String, CaseIterable, Sendable {\n")
	for _, mode := range t.Modes() {
		fmt.Fprintf(&b, "    case %s\n", mode)
	}
	b.WriteString("}\n\n")

	b.WriteString("public extension Color {\n")
	b.WriteString("    /// Look up a semantic Conduit color by its dotted path (e.g. \"surface.canvas\").\n")
	b.WriteString("    static func conduit(_ path: String, theme: ConduitTheme = .dark) -> Color {\n")
	b.WriteString("        switch theme {\n")
	for _, mode := range t.Modes() {
		fmt.Fprintf(&b, "        case .%s: return Self._conduit_%s[path] ?? .clear\n", mode, mode)
	}
	b.WriteString("        }\n")
	b.WriteString("    }\n")
	b.WriteString("}\n\n")

	for _, mode := range t.Modes() {
		fmt.Fprintf(&b, "private extension Color {\n")
		fmt.Fprintf(&b, "    static let _conduit_%s: [String: Color] = [\n", mode)
		colorRoot, ok := t.Semantic[mode]["color"].(Tree)
		if !ok {
			b.WriteString("    ]\n}\n\n")
			continue
		}
		Walk(colorRoot, func(path []string, value string) {
			rgba, ok := hexToSwiftColorLiteral(value)
			if !ok {
				return
			}
			fmt.Fprintf(&b, "        \"%s\": %s,\n", strings.Join(path, "."), rgba)
		})
		b.WriteString("    ]\n}\n\n")
	}

	return b.String()
}

func hexToSwiftColorLiteral(hex string) (string, bool) {
	s := strings.TrimSpace(strings.TrimPrefix(hex, "#"))
	if len(s) != 6 {
		return "", false
	}
	parse := func(c string) (float64, bool) {
		v, err := strconv.ParseUint(c, 16, 8)
		if err != nil {
			return 0, false
		}
		return float64(v) / 255, true
	}
	r, ok := parse(s[0:2])
	if !ok {
		return "", false
	}
	g, ok := parse(s[2:4])
	if !ok {
		return "", false
	}
	bl, ok := parse(s[4:6])
	if !ok {
		return "", false
	}
	return fmt.Sprintf("Color(red: %.4f, green: %.4f, blue: %.4f)", r, g, bl), true
}
