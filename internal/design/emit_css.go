package design

import (
	"fmt"
	"strings"
	"time"
)

// EmitCSS renders the resolved tokens as CSS custom properties.
// Layout:
//
//	:root           ← reference scale + dark mode (the default)
//	[data-theme=light]
//	[data-theme=hc]
func EmitCSS(t *Tokens) string {
	var b strings.Builder
	fmt.Fprintf(&b, "/* Conduit Design Tokens — generated %s. Do not edit by hand. */\n", time.Now().UTC().Format("2006-01-02"))
	b.WriteString("/* Source: design/tokens.yaml. Regenerate with `make tokens`. */\n\n")

	b.WriteString(":root {\n")
	b.WriteString("  color-scheme: dark;\n")
	emitLeaves(&b, "  --ref-", t.Reference)
	b.WriteString("\n  /* dark mode is the default */\n")
	if dark, ok := t.Semantic["dark"]; ok {
		emitLeaves(&b, "  --", dark)
	}
	b.WriteString("}\n\n")

	for _, mode := range []string{"light", "hc"} {
		tree, ok := t.Semantic[mode]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "[data-theme=\"%s\"], :root[data-theme=\"%s\"] {\n", mode, mode)
		if mode == "light" {
			b.WriteString("  color-scheme: light;\n")
		}
		emitLeaves(&b, "  --", tree)
		b.WriteString("}\n\n")
	}

	b.WriteString("@media (prefers-color-scheme: light) {\n")
	b.WriteString("  :root:not([data-theme]) {\n")
	b.WriteString("    color-scheme: light;\n")
	if light, ok := t.Semantic["light"]; ok {
		emitLeaves(&b, "    --", light)
	}
	b.WriteString("  }\n")
	b.WriteString("}\n")

	return b.String()
}

func emitLeaves(b *strings.Builder, prefix string, t Tree) {
	Walk(t, func(path []string, value string) {
		fmt.Fprintf(b, "%s%s: %s;\n", prefix, strings.Join(path, "-"), value)
	})
}
