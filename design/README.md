# Conduit Design System

Open-source design package — tokens, generated outputs, component
documentation, icon set, and usage guidelines. Plugin developers import
once and match the host app exactly.

> Source: PRD §12.7

## Source of truth

[`design/tokens.yaml`](./tokens.yaml) is the single source of truth.
`cmd/design-tokens` compiles it to per-platform outputs in
[`design/dist/`](./dist):

| Platform | Output | Package |
|----------|--------|---------|
| Web | `dist/web/tokens.css` | [`@conduit/design`](./dist/web) (npm) |
| Apple | `dist/apple/Tokens.swift` | [`ConduitDesign`](./dist/apple) (Swift Package) |
| TUI | `dist/tui/theme-{dark,light,hc}.json` | [`conduit-design`](./dist/tui) (PyPI) |

Run `make tokens` to regenerate. CI enforces `make tokens-check` so
manual edits to `dist/` are caught.

## Modes

Three modes are emitted everywhere:

- **dark** — default
- **light**
- **hc** — high-contrast, all foreground/background pairs in
  `contrast_pairs_aaa` meet WCAG AAA (7:1)

## Component documentation

Browse [`docs/design-system/components.html`](../docs/design-system/components.html)
for the live component gallery and
[`docs/mockups/`](../docs/mockups) for fidelity references.

## Figma library

Track the Figma source-of-truth file linked from
[`docs/design-system/`](../docs/design-system) — token sync via Tokens
Studio keeps Figma and `tokens.yaml` aligned. See issue #105.

## Icon set

Lucide-derived icon set is shipped as part of each platform package.
Additions go in `design/icons/` (planned) and are re-emitted by
`cmd/design-tokens` on the next run.

## Usage guidelines

- **Never use reference tokens directly** in product code; reference
  semantic tokens (`color.fg.primary`, not `color.indigo.100`).
- **Mode switching is host-controlled.** Plugins should not hardcode a
  mode — read from the host theme.
- **Contrast pairs are enforced.** Adding a new fg/bg pair? Add it to
  `contrast_pairs_aaa` in `tokens.yaml` so the contrast test guards it.

## Packages

| Package | Registry | Status |
|---------|----------|--------|
| `@conduit/design` | npm | scaffolded, publish via `npm publish` from `design/dist/web` |
| `ConduitDesign` | Swift Package Index | scaffolded, tag a release to publish |
| `conduit-design` | PyPI | scaffolded, build from `design/dist/tui` |

Publish workflows are intentionally manual until the design system
stabilizes.
