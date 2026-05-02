# ADR-003: GUI Stack — Tauri

**Status:** Accepted
**Date:** 2026-05-02
**Issue:** [#43 — Decide GUI stack (SwiftUI vs Tauri vs Electron)](https://github.com/jabreeflor/conduit/issues/43)

---

## Context

ADR-001 establishes Go as the core language. ADR-002 selects Bubble Tea for the in-process TUI. The macOS GUI (PRD §11.2) is a separate surface — a visual-task front-end primarily for computer use workflows — and must deliver:

- **Three-column layout:** sidebar (sessions / workflows / memory / skills) + main content area + agent panel (chat + status)
- **Live computer use screenshot stream** rendered in the main content area, updated per workflow step
- **Workflow DAG visualization** with pulsing current step, solid completed steps, red failed steps
- **Canvas panel:** an embedded `WKWebView` that renders agent-injected HTML via `[[canvas: html]]` tags (already mandated by §11.2)
- **Session tree browser** (fork, replay from any turn), SOUL.md/USER.md editor, memory inspector, evals tab
- **Command palette (`mod+k`)**, keyboard-first density, dark-by-default visual language inspired by Claude Desktop + Codex + T3 Code
- **Connection model:** the GUI talks to the shared Conduit core process over **WebSocket push** (PRD §19 Phase 5) — the GUI is a view layer, not a model host

The iOS companion app (§9) and the watchOS complication (§9.7) are **separate Apple-platform surfaces** that *must* be built with WidgetKit / SwiftUI — Apple does not permit web-stack widgets or complications. That constraint is independent of the macOS GUI choice.

Three candidates were evaluated: **SwiftUI**, **Tauri**, and **Electron**.

---

## Decision

**Tauri (Rust + system WebView + JS/TS frontend)** is the GUI stack for Conduit's macOS app.

---

## Evaluation

### SwiftUI

| Factor | Assessment |
|---|---|
| Native macOS feel | **Excellent** — first-party AppKit/SwiftUI integration, system-quality animations, sidebar/toolbar/sheet idioms |
| Distribution | Single `.app` bundle, no embedded runtime; native code-signing and notarization path |
| Footprint | Smallest of the three — no embedded browser, no Node, no Rust toolchain |
| Connection to Go core | Foreign to the core — must talk over the same WebSocket as any other client; no in-process advantage |
| Canvas panel (`WKWebView`) | Native — `WKWebView` is a SwiftUI-friendly view; `[[canvas: html]]` injection is a `loadHTMLString` call |
| Sharing with iOS companion (§9) | **Strongest** — view models, domain types, and even some views can be shared via a Swift package consumed by both targets |
| Cross-platform future (Linux/Windows) | **None** — SwiftUI is Apple-only; a future Linux/Windows GUI would be a separate codebase |
| Iteration speed | Slower — Xcode build cycle, simulator-style preview, less hot-reload than web |
| Talent / contribution surface | Narrow — Swift+SwiftUI is a smaller contributor pool than web stack |

### Tauri

| Factor | Assessment |
|---|---|
| Native macOS feel | **Acceptable** — system `WKWebView` (same renderer as Safari) + Tauri's native menus, dialogs, and tray; not pixel-perfect AppKit but indistinguishable for our chrome-light, content-dense layout |
| Distribution | `.app` bundle, code-signable and notarizable; no embedded Chromium (unlike Electron) |
| Footprint | **Small** — typical Tauri app is 10–20 MB vs Electron's 100+ MB; uses the OS-provided WebView |
| Connection to Go core | Frontend speaks WebSocket directly to the core; Tauri's Rust shell is a thin host (windowing, menus, IPC) — no need for a fat Rust backend |
| Canvas panel (`WKWebView`) | **Trivial** — the entire GUI *is* a `WKWebView`, so `[[canvas: html]]` is just a sub-component of the same web app; no WebView↔native bridge needed |
| Sharing with iOS companion (§9) | **None** at the view layer (iOS must be SwiftUI). Shared API types can be generated from the Go core (e.g., OpenAPI / TypeScript + Swift codegen) — same approach works regardless of macOS choice |
| Cross-platform future (Linux/Windows) | **Yes** — Tauri targets Linux (WebKitGTK) and Windows (WebView2) from the same codebase; aligns with §17 mentions of remote/headless Conduit servers |
| Iteration speed | **Fast** — web hot-reload, Vite/Next dev server, browser DevTools for layout debugging |
| Talent / contribution surface | **Wide** — JS/TS + React/Svelte/Solid; standard web tooling |

### Electron

| Factor | Assessment |
|---|---|
| Native macOS feel | Acceptable — same web-stack ceiling as Tauri |
| Distribution | `.app` bundle with embedded Chromium; code-signable and notarizable |
| Footprint | **Heavy** — 100+ MB minimum, embedded Chromium + Node runtime per app |
| Connection to Go core | Same as Tauri — frontend talks WebSocket; no in-process advantage |
| Canvas panel | Same as Tauri — entire GUI is a webview |
| Sharing with iOS | None — same as Tauri |
| Cross-platform future | Yes — Linux and Windows from one codebase |
| Iteration speed | Fast — same web tooling story |
| Talent / contribution surface | Widest — most established web-desktop framework |
| Runtime model | Each window is a Chromium process; resident memory is 3–5× Tauri for the same UI |

---

## Rationale

Tauri and Electron are close on most axes; the decision was driven by **footprint and runtime model**, not contributor reach or feature parity.

Conduit is positioned as a developer tool that runs alongside the TUI and a Go core process. A 100+ MB Electron bundle with an embedded Chromium and a per-window Node runtime sets the wrong tone: it is heavier than the rest of the system put together, and resident memory grows 3–5× per open window. Tauri's 10–20 MB shell over the system `WKWebView` keeps the GUI proportional to the tool — the same renderer that already powers the Canvas panel (§11.2), no second browser bundled.

Rust as a third toolchain language is accepted as a bounded cost: the Tauri shell is treated as a windowing host (see Mitigations), so the Rust surface stays small. We accept rendering drift across macOS WebKit versions in exchange for security updates riding the OS — bounded by a minimum-macOS floor below.

---

## Consequences

### Positive

- **Canvas panel is free** — §11.2 requires a `WKWebView` for agent-rendered HTML; the entire Tauri GUI runs in `WKWebView`, so the Canvas is a sub-component of the same web app rather than an embedded native view with a JS bridge.
- **Small distribution** — 10–20 MB `.app` bundle keeps Conduit feeling like a developer tool, not a productivity-suite install.
- **Cross-platform path** — the same GUI codebase ports to Linux (WebKitGTK) and Windows (WebView2) when remote/headless Conduit servers (§17) gain non-Mac UI users.
- **Fast iteration** — Vite hot-reload + browser DevTools for the GUI layout; no Xcode build cycle.
- **Wide contributor surface** — anyone who can ship React/Svelte/Solid can contribute to the GUI; SwiftUI would have constrained the pool to Apple-platform devs.
- **Decoupled from core release cycle** — Tauri shell is thin; updating the GUI does not require a Go core rebuild, and vice versa.

### Negative / Mitigations

- **Weaker native feel than SwiftUI.** Acceptable. The GUI is keyboard-first and chrome-light per the Claude Desktop + Codex + T3 Code reference (§11.2); we are not competing with Notes.app on AppKit polish. We will CSS-recreate two specific macOS idioms because they carry the "feels like a Mac app" signal disproportionately: translucent/vibrant sidebar background and native traffic-light spacing in the title bar. Everything else — selection animations, sheets, popovers — uses our own design system and is judged against the reference apps, not against AppKit.

- **No view-layer sharing with iOS companion (§9).** Compensated by sharing **API contracts**, not views. The Go core publishes its WebSocket protocol and REST endpoints as an OpenAPI schema; both the Tauri frontend (TypeScript client) and the iOS companion (Swift client) are generated from it. Shared surface: workflow DAG schema, session/turn types, quick-action config, status events. Views, layouts, and interaction code are intentionally *not* shared — the two surfaces have different shapes and trying to share UI code across them would be net-negative.

- **Rust as a third toolchain language.** Bounded by an explicit rule: **the Tauri Rust layer is a windowing host only — no business logic, no protocol parsing, no model state.** It owns window lifecycle, native menus, file dialogs, the system tray, and the WebSocket bootstrap to the Go core. Any logic that could live in the frontend or the core lives there instead. Reviewers should reject Rust PRs that violate this.

- **System WebView drift across macOS versions.** Bounded by a minimum-macOS floor of **macOS 13 (Ventura)**, which gives us a modern `WKWebView` baseline (CSS container queries, `:has()`, modern flexbox). Older macOS versions are not a supported GUI target — they can still use the TUI, which has no such constraint.

---

## Spike Summary

No spike was built for this ADR; the decision is a design choice, not an architectural unknown. A scaffolding spike (`spike/gui/`) is tracked separately as a **Phase 5 build task** in PRD §19: *"Scaffold GUI stack"*.

That spike, when built, should demonstrate:

1. Tauri shell connecting to the Conduit core over WebSocket
2. Three-column layout (sidebar / main content / agent panel) at the target dimensions
3. `[[canvas: html]]` injection rendering into a Canvas sub-panel
4. Hot-reload dev loop (`tauri dev`) and a release build under 25 MB

---

## References

- [Tauri](https://tauri.app/)
- ADR-001: Core Language — Go
- ADR-002: TUI Stack — Bubble Tea + Lipgloss
- PRD §11.2 — GUI (macOS App)
- PRD §9 — iPhone Widget & Companion App
- PRD §9.7 — Apple Watch Complication
- PRD §19 Phase 5 — GUI + Spotlight UI
