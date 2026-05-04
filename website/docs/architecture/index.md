# Architecture

Conduit is a Go monorepo. The top-level layout:

```
cmd/conduit/        # binary entry point
internal/
  router/           # provider routing & failover
  sessions/         # JSONL session store
  memory/           # pluggable memory providers
  hooks/            # subprocess hook runtime
  workflow/         # YAML workflow engine
  coding/           # `conduit code` REPL
  tools/            # built-in tool implementations
  tui/              # Bubble Tea TUI
  gui/              # GUI view-models (rendered by Tauri / SwiftUI)
  usage/            # local cost & usage ledger
  …
```

The [PRD](https://github.com/jabreeflor/conduit/blob/main/docs/PRD.md) is the
canonical product spec. Cross-cutting decisions live in
[Architecture Decision Records](./adr).
