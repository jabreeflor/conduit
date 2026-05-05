# Conduit for VS Code (Cursor, Windsurf)

This extension connects VS Code, Cursor, and Windsurf to a locally-running
[Conduit] instance. The build is the same `.vsix` for all three editors —
they share the VS Code extension API.

## What it does today

- Adds a Conduit panel to the activity bar.
- Adds a status-bar item showing connection state (click to connect).
- Probes `http://127.0.0.1:8923/v1/healthz` on activation.
- Commands:
  - **Conduit: Connect to running instance** (`Cmd+Alt+Shift+C`)
  - **Conduit: Open sidebar** (`Cmd+Alt+C`)
  - **Conduit: Share current file as context**
  - **Conduit: Share selection as context** (`Cmd+Alt+S`)

The chat panel, diff review, and inline suggestions are intentionally
out of scope for this scaffold — they land in follow-up issues.

## Configuration

| Setting             | Default                      | Description                            |
| ------------------- | ---------------------------- | -------------------------------------- |
| `conduit.endpoint`  | `http://127.0.0.1:8923`      | Base URL of the Conduit daemon.        |
| `conduit.timeoutMs` | `2000`                       | Connection probe timeout.              |

## Build

```sh
npm install
npm run compile
```

To package as `.vsix`:

```sh
npx @vscode/vsce package
```

## Test

```sh
npm test
```

[Conduit]: https://github.com/jabreeflor/conduit
