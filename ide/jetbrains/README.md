# Conduit for JetBrains

Connects any IntelliJ-Platform IDE (IDEA, PyCharm, GoLand, WebStorm, …) to a
locally-running [Conduit] instance.

## What it does today

- Adds a "Conduit" tool window on the right-hand sidebar.
- Adds three actions:
  - **Conduit: Connect to Running Instance** (`Cmd/Ctrl+Alt+Shift+C`)
  - **Conduit: Share Current File as Context**
  - **Conduit: Share Selection as Context** (`Cmd/Ctrl+Alt+S`)
- Probes `http://127.0.0.1:8923/v1/healthz` and reports back via a
  notification.
- Persists endpoint and timeout under application-level settings
  (`conduit.xml`).

The chat surface, diff review, and inline suggestions are intentionally
out of scope for this scaffold.

## Build

```sh
./gradlew buildPlugin
```

The packaged plugin is dropped at `build/distributions/conduit-jetbrains-*.zip`,
installable via Settings → Plugins → ⚙ → Install Plugin from Disk.

## Test

```sh
./gradlew test
```

## Cross-platform

Pure JVM + standard `java.net.http`. No native dependencies. Works on
macOS, Windows, and Linux wherever the JetBrains IDE itself runs.

[Conduit]: https://github.com/jabreeflor/conduit
