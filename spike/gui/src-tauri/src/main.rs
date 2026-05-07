// Conduit GUI scaffold — Tauri shell.
//
// Per ADR-003 (docs/adr/003-gui-stack-tauri.md), this binary is a windowing
// host only:
//   - window lifecycle
//   - native menus, dialogs, system tray
//   - WebSocket bootstrap to the Conduit core
//
// Anything that could live in the frontend (TypeScript) or the Go core
// lives there instead. PRs that grow this file beyond windowing should be
// rejected.

#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    tauri::Builder::default()
        .run(tauri::generate_context!())
        .expect("error while running Conduit GUI");
}
