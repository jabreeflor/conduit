# ADR-002: TUI Stack — Bubble Tea + Lipgloss

**Status:** Accepted  
**Date:** 2026-04-29  
**Issue:** [#2 — Decide TUI stack](https://github.com/jabreeflor/conduit/issues/2)

---

## Context

ADR-001 establishes Go as the core language. The TUI is the primary developer-facing surface (PRD §11.1) and must deliver:

- Three-panel layout: status bar (top), conversation + context panels (split), input (bottom)
- Collapsible tool-call blocks with live status indicators (⟳ / ✓ / ✗)
- Streaming model output rendered incrementally without full redraws
- User-configurable key bindings (`mod+p` panel toggle, `shift+enter` multiline input)
- Session cost and model name always visible in the status bar

Two candidates were evaluated against these requirements: **Textual** (Python) and **Bubble Tea** (Go).

---

## Decision

**Bubble Tea + Lipgloss** is the TUI stack for Conduit.

---

## Evaluation

### Textual (Python)

| Factor | Assessment |
|---|---|
| Layout system | Excellent — CSS-like, flexbox-inspired grid |
| Animation / live updates | Good — reactive widgets, smooth redraws |
| Language fit | **Mismatched** — requires a Python runtime alongside the Go binary |
| Distribution | Requires Python 3.11+ in PATH; breaks single-binary constraint from ADR-001 |
| IPC with Go engine | Mandatory subprocess boundary — adds latency and a serialization layer at every event |
| Key binding system | Built-in, configurable |
| Community | Active, maintained by Textualize |

Textual is a compelling TUI framework in isolation, but it introduces a **language boundary** directly in the hot path of the engine-to-UI loop. Every streaming token, every tool-call status update, and every hook event would cross a JSON-over-subprocess bridge. That bridge is a persistent latency source, a distribution problem (two runtimes), and a failure domain the engine cannot observe or control. Disqualified by ADR-001's single-binary constraint.

### Bubble Tea (Go)

| Factor | Assessment |
|---|---|
| Language fit | Native Go — same binary as the engine, zero IPC overhead |
| Architecture | Elm-style update loop (`Model → Msg → Cmd`) maps cleanly onto Conduit's event bus |
| Layout | Lipgloss handles fixed-width box sizing, borders, and color; Lipgloss `Join*` for panel composition |
| Live updates | `tea.Cmd` + channel-driven `Msg` feed streaming tokens directly into the render cycle |
| Key binding system | `bubbles/key` — bindable, rebindable, composable |
| Viewport scrolling | `bubbles/viewport` — zero boilerplate scrollable panels |
| Distribution | Compiles into the single `conduit` binary — no runtime dependency |
| Community | Charmbracelet ecosystem; production-deployed in `gh`, `lazygit`, and `gum` |

The Elm message loop is not just a stylistic choice — it is an architectural fit. Conduit's engine already routes events (hook dispatch, model stream chunks, workflow step transitions) as discrete messages. The TUI becomes a straightforward subscriber to that event stream: each engine event maps to a `tea.Msg`, the `Update` function applies it to the `Model`, and `View` renders it. No threads, no shared state, no locks.

---

## Consequences

### Positive

- **Zero distribution cost** — Bubble Tea and Lipgloss are pure-Go; `go build` folds them into the single `conduit` binary.
- **Direct engine access** — the TUI runs in the same process as the engine; streaming tokens arrive via Go channels with sub-millisecond latency.
- **Elm message loop = natural event bus consumer** — hook events, model stream chunks, and workflow state transitions all become `tea.Msg` values; no serialization required.
- **`bubbles` library** reduces boilerplate: `viewport` for scrollable panels, `textarea` for the input, `key` for configurable bindings, `spinner` for loading states.
- **Proven at scale** — `gh` (GitHub CLI) and `lazygit` both use Bubble Tea for complex multi-panel interfaces with live updates.

### Negative / Mitigations

- **Lipgloss layout is width-based, not flexbox** — complex responsive layouts require more explicit sizing than Textual's CSS grid. Mitigation: the three-panel layout has fixed proportions (left ~60 %, right ~40 %); terminal resize sends a `tea.WindowSizeMsg` that triggers a recalculation.
- **No built-in table/tree widget** — the session tree browser and workflow step list require custom rendering. Mitigation: `bubbletea-table` community library handles the session tree; workflow steps are a simple indexed list.
- **Mouse support is opt-in and limited** — Conduit is keyboard-first by design, so this is acceptable. Mouse scroll in viewports is enabled via `viewport.WithMouseWheelEnabled`.

---

## Spike Summary

`spike/tui/` contains a working prototype demonstrating:

1. **Three-panel layout** via Lipgloss — status bar, conversation viewport, context panel, input textarea
2. **Collapsible tool-call blocks** — press `enter` on a tool-call line to expand/collapse; ⟳/✓/✗ status prefixes
3. **Key bindings** — `mod+p` to toggle the context panel, `shift+enter` for multiline input, `ctrl+c` / `esc` to quit
4. **Streaming simulation** — a ticker sends fake token `Msg` values every 50 ms, demonstrating that the viewport updates incrementally without full redraws

Run with: `cd spike/tui && go run .`

---

## References

- [Bubble Tea GitHub](https://github.com/charmbracelet/bubbletea)
- [Lipgloss GitHub](https://github.com/charmbracelet/lipgloss)
- [Bubbles component library](https://github.com/charmbracelet/bubbles)
- ADR-001: Core Language — Go
- PRD §11.1 — TUI specification
