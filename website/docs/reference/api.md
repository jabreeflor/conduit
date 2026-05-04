# API reference

Conduit exposes a local HTTP API for editor extensions and remote control.

::: tip Auto-generated
This page is the entry point for the auto-generated API reference. The full
schema is built from Go source comments via `go doc` and rendered into
`./generated/` at build time. See `website/scripts/gen-api.mjs`.
:::

## Base URL

```
http://127.0.0.1:8923
```

The port and bind address are configurable under `api.bind` in
`~/.conduit/config.yaml`.

## Endpoints (summary)

| Method | Path                        | Purpose                              |
| ------ | --------------------------- | ------------------------------------ |
| GET    | `/v1/healthz`               | Liveness probe                       |
| GET    | `/v1/sessions`              | List sessions                        |
| POST   | `/v1/sessions`              | Create a session                     |
| POST   | `/v1/sessions/:id/messages` | Send a message; stream back response |
| GET    | `/v1/providers`             | List configured providers            |
| GET    | `/v1/usage`                 | Usage / cost rollup                  |

Full request/response shapes live in the generated reference.
