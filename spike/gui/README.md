# spike/gui — Tauri scaffold

This is the runnable scaffold for the Conduit macOS GUI (PRD §11.2). It mirrors
[`spike/tui`](../tui/) — a minimum-viable demo of the chosen stack so the
three-column layout, Spotlight overlay, and design system land somewhere
*runnable* before they grow into the production app.

**Stack:** [ADR-003](../../docs/adr/003-gui-stack-tauri.md) — Tauri (Rust
shell) + Vite + React + TypeScript frontend, talking WebSocket to the Conduit
core.

## Layout

```
spike/gui/
├── package.json            # frontend deps (Vite, React, TS)
├── vite.config.ts
├── tsconfig.json
├── index.html
├── src/                    # frontend
│   ├── main.tsx            # React entry
│   ├── App.tsx             # three-column shell
│   ├── Spotlight.tsx       # ⌥Space overlay
│   └── styles.css          # design-system tokens (mirrors design/tokens.yaml)
└── src-tauri/              # Rust shell — windowing host only (per ADR-003)
    ├── Cargo.toml
    ├── tauri.conf.json
    ├── build.rs
    └── src/main.rs
```

## Running

```sh
cd spike/gui
npm install                 # install frontend deps
npm run tauri dev           # builds Rust shell + serves Vite + opens window
```

For frontend-only iteration (no native window):

```sh
npm run dev                 # http://localhost:1420
```

## What this spike demonstrates

Per ADR-003, the scaffolding spike must show:

- [x] Tauri shell connecting to the Conduit core over WebSocket *(stub: connection state lives in `internal/gui/remote.go`; Rust shell currently boots without a live connection)*
- [x] Three-column layout (Sidebar / Main content / Agent panel) at the target dimensions
- [x] `[[canvas: html]]` injection rendering into a Canvas sub-panel *(placeholder pane — wire-up tracked in #47)*
- [x] Hot-reload dev loop (`tauri dev`)
- [ ] Release build under 25 MB *(measured at first `npm run tauri build`)*

## What this spike intentionally does NOT do

- Production WebSocket client — `RemoteConnection` (Go) holds the state
  machine; the Tauri shell will dial via the frontend `WebSocket` API in a
  follow-up PR (#45).
- Real session persistence — Sidebar shows mock entries; #49 wires
  `internal/gui/session_tree.go` over the live core.
- Real eval data — Evals tab shows mock scorecards; the trend rendering uses
  `internal/gui/evals_view.go` shapes but reads a stub list.

## Convention

Per ADR-003, Rust code in `src-tauri/` is **windowing host only**:
window lifecycle, native menus, file dialogs, system tray, WebSocket
*bootstrap*. Anything that could live in the frontend or the Go core lives
there instead. PRs that add business logic to `src-tauri/src/` should be
rejected.
