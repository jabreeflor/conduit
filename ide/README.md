# IDE integrations

This directory holds the editor extensions that connect to a running Conduit
instance (PRD §6.25.14, issue #143).

| Editor                    | Path              | Status                |
| ------------------------- | ----------------- | --------------------- |
| VS Code (+ Cursor / Windsurf, both VS Code-API compatible) | [`vscode/`](./vscode/) | Scaffold + connect command |
| JetBrains (IntelliJ etc.) | [`jetbrains/`](./jetbrains/) | Scaffold + connect action |

## Scope

The PR that adds these scaffolds intentionally ships the **minimum** an IDE
needs:

- a sidebar entry,
- keyboard shortcuts,
- a single "Connect to running Conduit" command that probes the local API
  port (`http://127.0.0.1:8923/v1/healthz`) and reports back,
- editor selection / file context sharing helpers (paths only — no file
  contents are uploaded by the scaffold).

Full feature parity (chat panel, diff review, inline suggestions) is **out of
scope** here and tracked under separate per-feature issues. The goal is to
unblock parallel work on the rich UI surfaces by establishing the build
system, manifest, and connection plumbing for each editor.

## Why VS Code covers Cursor / Windsurf too

Both Cursor and Windsurf are VS Code forks and consume the same `.vsix`
extension format and API. The `vscode/` extension installs unmodified in all
three. We document install steps for each in the README and may publish to
their separate marketplaces from a single CI build.

## Cross-platform

Both extensions rely only on platform-neutral APIs (`fetch` in VS Code,
`HttpClient` in JetBrains) and ship as portable bundles. macOS, Windows, and
Linux all work — the only platform-specific bit is the running Conduit
binary itself, which today is macOS-only.
