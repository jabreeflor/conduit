# Conduit Design Language — "Minimalist & Airy"

> Companion to [`design/README.md`](../design/README.md). This document describes
> the visual language; [`design/tokens.yaml`](../design/tokens.yaml) is the
> single source of truth for the values it names.

## Overview

**Minimalist & Airy** is Conduit's warm-cream visual language. It replaces the
former dark indigo/saffron theme with an editorial, architectural feel: a soft
cream canvas, near-black ink for text and primary actions, a single restrained
brass-gold accent, and generous whitespace. The intent is calm focus — the
interface recedes so the conversation, agents, and content lead. Surfaces are
quiet and layered rather than boxed-in; type carries the hierarchy; the gold
accent appears sparingly to mark what is active or actionable.

## Modes

Three modes are emitted to every platform from `tokens.yaml`. **Light is the
default look the GUI ships rendering** — the cream "Minimalist & Airy" canvas.

| Mode | Role | Character |
|------|------|-----------|
| `light` | **Default GUI appearance** | Cream canvas, near-black ink, brass-gold accent |
| `dark` | Warm complement | Warm charcoal surfaces, cream text, lighter gold accent |
| `hc` | Accessibility | High-contrast, pure black/white anchors, WCAG AAA (7:1) |

Note: in the generated CSS, `:root` carries the `dark` mapping as the file-level
default, but the GUI sets `data-theme="light"` so the cream language is what
users see. The `hc` mode enforces every pair listed in `contrast_pairs_aaa`.

## Color

### Light-mode semantic tokens

Resolved hex values quoted directly from `design/dist/web/tokens.css`
(`[data-theme="light"]`).

| Token (`--color-*`) | Hex | Usage |
|---------------------|-----|-------|
| `surface-canvas` | `#FBF9F4` | App background (cream) |
| `surface-primary` | `#FFFFFF` | Cards, panels, composer |
| `surface-elevated` | `#FFFFFF` | Popovers, modals |
| `surface-sunken` | `#F5F3EE` | Input fields, code blocks |
| `surface-container` | `#F0EEE9` | Filled chips, soft fills |
| `surface-container-high` | `#EAE8E3` | Raised chips, avatars |
| `surface-container-lowest` | `#FFFFFF` | Pure-white wells |
| `text-body` | `#1B1C19` | Primary text (near-black ink) |
| `text-muted` | `#4B463F` | Secondary text |
| `text-subtle` | `#7C766E` | Tertiary text, placeholders |
| `text-link` | `#6E5C37` | Hyperlinks (brass-gold) |
| `border-subtle` | `#CDC5BC` | Hairline dividers |
| `border-default` | `#CDC5BC` | Default control borders |
| `border-strong` | `#7C766E` | Emphasized borders |
| `border-focus` | `#6E5C37` | Focus ring (brass-gold) |
| `brand-primary` | `#1B1C19` | Primary filled buttons, send button, logo box |
| `brand-on-primary` | `#FFFFFF` | Text/icon on `brand-primary` |
| `secondary-base` | `#6E5C37` | Brass-gold accent — active rail, links, focus |
| `secondary-container` | `#F6DDAE` | Soft gold fill (sandbox affordances) |
| `secondary-on-container` | `#73603B` | Text on `secondary-container` |

> **Correction to draft values:** `border-subtle` / `border-default` resolve to
> **`#CDC5BC`** (from `sand.400`), not `#E4E2DD`. In the light mode both map to
> `{color.sand.400}`. (`#E4E2DD` is `sand.300`, used only for `surface` steps in
> dark mode.) Also `secondary-on-container` is **`#73603B`** (`gold.650`).

### Reference ramps

The new warm scales drive everything above. Consumers must reference semantic
tokens, never these raw stops directly.

**`sand` — warm neutral (cream → charcoal):**

| Stop | Hex | | Stop | Hex |
|------|-----|--|------|-----|
| `50` | `#FBF9F4` | | `400` | `#CDC5BC` |
| `100` | `#F5F3EE` | | `500` | `#A39C92` |
| `150` | `#F0EEE9` | | `600` | `#7C766E` |
| `200` | `#EAE8E3` | | `700` | `#4B463F` |
| `300` | `#E4E2DD` | | `800` | `#30312E` |
| | | | `900` | `#1B1C19` |
| | | | `950` | `#121310` |

**`gold` — brass accent:**

| Stop | Hex | | Stop | Hex |
|------|-----|--|------|-----|
| `50` | `#FCF6E6` | | `500` | `#8C7851` |
| `100` | `#F9DFB1` | | `600` | `#6E5C37` |
| `200` | `#F6DDAE` | | `650` | `#73603B` |
| `300` | `#DCC497` | | `700` | `#554422` |
| `400` | `#B69A6A` | | `800` | `#3A2E14` |
| | | | `900` | `#261A00` |

The legacy `indigo`, `saffron`, `cyan`, `violet`, `red`, and `green` ramps remain
in `tokens.yaml` — they back model badges, status colors, and the `hc` mode.

## Typography

Three families, loaded via Google Fonts in
[`spike/gui/index.html`](../spike/gui/index.html) and referenced through
`--ref-type-family-*`.

| Role | Family | Var |
|------|--------|-----|
| Display / headline | **Manrope** | `--ref-type-family-display` |
| Body / UI | **Hanken Grotesk** | `--ref-type-family-sans` |
| Labels / code | **JetBrains Mono** | `--ref-type-family-mono` |

Size scale (`--ref-type-size-*`):

| Token | Size | | Token | Size |
|-------|------|--|-------|------|
| `caption` | 11px | | `heading-3` | 18px |
| `small` | 12px | | `heading-2` | 22px |
| `body` | 14px | | `heading-1` | 28px |
| `body-large` | 16px | | `display` | 36px |
| | | | `display-large` | 48px |

Weights: `regular` 400, `medium` 500, `semibold` 600, `bold` 700.
Line heights: `tight` 1.2, `normal` 1.5, `relaxed` 1.7.

## Spacing, radius & motion

**Spacing** (`--ref-space-*`) — 4px base step:

| `xs` | `sm` | `md` | `lg` | `xl` | `2xl` | `3xl` |
|------|------|------|------|------|-------|-------|
| 4px | 8px | 16px | 24px | 32px | 48px | 64px |

**Radius** (`--ref-radius-*`):

| `sm` | `md` | `lg` | `xl` | `full` |
|------|------|------|------|--------|
| 4px | 8px | 12px | 16px | 9999px |

**Motion** (`--ref-motion-*`):

| Duration | | Easing | |
|----------|--|--------|--|
| `instant` 0ms | `fast` 100ms | `default` | `cubic-bezier(0.2, 0.8, 0.2, 1)` |
| `normal` 200ms | `slow` 400ms | `spring` | `cubic-bezier(0.34, 1.56, 0.64, 1)` |
| | | `linear` | `linear` |

## Components

Key GUI surfaces expressed in the new language:

- **Sidebar** — persistent left rail on `surface-canvas`. Brand mark at top;
  primary nav (New chat, Projects, Plugins, Automations, Soul); collapsible
  Projects and Chats sections; user-profile footer. The active item is marked
  with the brass-gold `secondary-base` rail.
- **Top app bar** — global search field plus splitscreen / dock controls,
  rendered on `surface-primary` with `border-subtle` hairlines.
- **Welcome screen** — rotated diamond brand mark over a centered composer.
  The composer carries sandbox and model pills; context pills sit beneath; and a
  row of four bento integration cards anchors the layout. Whitespace is generous
  and editorial.
- **Team Activity panel** — a right-hand companion on the welcome screen, quiet
  surfaces with subtle borders.
- **Chat panel** — user messages as near-black (`brand-primary`) bubbles;
  assistant replies as plain body text on the cream canvas; tool calls render as
  collapsible disclosure rows. A right-hand **soul.md** context panel shows the
  active persona.
- **Spotlight** — command palette layered at `z-spotlight` (1400). Categorized
  rows (Command / Workflow / Memory / Session) carry colored kind labels, with
  footer key hints for navigation and selection.

## Screenshots

Captured from the live GUI (`spike/gui`) at 1512×945, light mode:

| Surface | Screenshot |
|---------|------------|
| Welcome | [`docs/design-system/screenshots/01-welcome.png`](design-system/screenshots/01-welcome.png) |
| Chat + soul.md | [`docs/design-system/screenshots/02-chat.png`](design-system/screenshots/02-chat.png) |
| Spotlight | [`docs/design-system/screenshots/03-spotlight.png`](design-system/screenshots/03-spotlight.png) |
| Projects list/hub | [`docs/design-system/screenshots/04-projects.png`](design-system/screenshots/04-projects.png) |
| New Project | [`docs/design-system/screenshots/05-new-project.png`](design-system/screenshots/05-new-project.png) |
| Project dashboard | [`docs/design-system/screenshots/06-project-dashboard.png`](design-system/screenshots/06-project-dashboard.png) |
| Welcome (dark theme) | [`docs/design-system/screenshots/07-welcome-dark.png`](design-system/screenshots/07-welcome-dark.png) |
| soul.md editor (live write) | [`docs/design-system/screenshots/08-soul-editor.png`](design-system/screenshots/08-soul-editor.png) |
| Soul page (live read/edit) | [`docs/design-system/screenshots/09-soul-page.png`](design-system/screenshots/09-soul-page.png) |
| Settings (theme + live About) | [`docs/design-system/screenshots/10-settings.png`](design-system/screenshots/10-settings.png) |
| Plugins (empty state) | [`docs/design-system/screenshots/11-plugins.png`](design-system/screenshots/11-plugins.png) |
| Chat (live streamed reply) | [`docs/design-system/screenshots/12-chat-live.png`](design-system/screenshots/12-chat-live.png) |

Regenerate with the running dev server via `node spike/gui/capture.mjs` (requires Playwright + a dev server on :1420).

## Source of truth & regeneration

- **`design/tokens.yaml` is authoritative.** `cmd/design-tokens` compiles it to
  `design/dist/{web,apple,tui}`. Never hand-edit generated output.
- Run **`make tokens`** to regenerate after any change.
- **CI enforces `make tokens-check`** — manual edits to `dist/` fail the build.
- **Reference scales must not be removed.** Tests assert specific stops exist
  (e.g. `--ref-color-saffron-300`), so the legacy ramps stay even though the new
  language leans on `sand` and `gold`.
- Reference **semantic** tokens in product and plugin code, never raw reference
  stops. Mode switching is host-controlled.
