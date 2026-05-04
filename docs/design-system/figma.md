# Figma — Source of Truth

> Source: PRD §12.8

Figma is the visual source of truth for the Conduit design system.
`design/tokens.yaml` is the *machine* source of truth for token values;
the two are kept in sync via [Tokens Studio](https://tokens.studio/).

## File structure

The maintained Figma file is organized into five top-level pages:

| Page | Contents |
|------|----------|
| **Foundations** | Color, typography, spacing, radius, motion, elevation tokens. Mirrors `design/tokens.yaml`. |
| **Components** | Buttons, inputs, cards, dialogs, lists, tabs, toasts. Auto-layout, variants for state and mode. |
| **Patterns** | Reusable compositions: command palette, model picker, session list, hook editor. |
| **Surfaces** | Full surfaces — TUI panels, GUI windows, Spotlight overlay, iPhone widgets, watch complications. |
| **Prototypes** | Interactive flows for review and stakeholder demos. |

## Linking the file

The canonical file URL lives in
[`docs/design-system/figma-link.txt`](./figma-link.txt). It is referenced
from:

- [Repo `README.md`](../../README.md)
- [`design/README.md`](../../design/README.md)
- This document

To rotate the link (e.g. moving teams, branching the file), update
`figma-link.txt` only — every consumer renders that value.

## Token sync via Tokens Studio

[Tokens Studio for Figma](https://tokens.studio/) hosts the canonical
mapping between Figma styles/variables and `design/tokens.yaml`.

Workflow:

1. **Designer edits in Figma.** Tokens Studio writes the change to its
   GitHub-sync repo branch (`design-tokens-sync`).
2. **CI in this repo** opens a PR that re-emits `design/dist/` via
   `make tokens` from the updated `design/tokens.yaml`.
3. **Reviewer** confirms `make tokens-check`, contrast tests, and
   visual diff before merge.

Reverse direction (engineer edits `tokens.yaml` first):

1. Edit `design/tokens.yaml`, run `make tokens`, open a PR here.
2. After merge, Tokens Studio pulls the change into Figma on next sync.

> **Integration TODO.** The Tokens Studio sync repo and CI workflow are
> not yet wired up — see issue #104 for the package-publish pipeline
> and a future ticket for the Tokens Studio bridge.

## Editing rules

- **Foundations are owned by `design/tokens.yaml`.** Never invent a
  one-off color in Figma; add it to `tokens.yaml` and re-sync.
- **Components must use Foundation styles**, not raw hex/px values.
- **Variants follow the mode set:** `dark` (default), `light`, `hc`.
- **Accessibility annotations** (focus order, contrast pair, ARIA role)
  live on each component frame as a Figma comment.

## Access

Read access is open. Edit access is granted by request — see the
maintainers list in the repo `README.md`.
