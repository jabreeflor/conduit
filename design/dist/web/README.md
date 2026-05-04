# @conduit/design (web)

CSS custom-property tokens for the Conduit design system. Generated from
[`design/tokens.yaml`](https://github.com/jabreeflor/conduit/blob/main/design/tokens.yaml)
by `cmd/design-tokens`.

## Install

```bash
npm install @conduit/design
```

## Usage

```css
@import "@conduit/design/tokens.css";

.button-primary {
  background: var(--color-bg-accent);
  color: var(--color-fg-on-accent);
  font-family: var(--font-family-sans);
}
```

Three modes are emitted: `:root` (dark, default), `[data-theme="light"]`,
and `[data-theme="hc"]` (high-contrast, WCAG AAA 7:1). Switch by setting
`data-theme` on the root element.

## Versioning

Tokens follow the host repo's semver. Breaking renames bump the minor
version pre-1.0 and the major version post-1.0.
