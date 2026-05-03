# Memory & sessions

## Sessions

Each conversation is a JSONL file under `~/.conduit/sessions/`. Manage them
from the TUI's session browser (default keybinding `Ctrl+S`) or the CLI:

```sh
conduit session list
conduit session show <id>
conduit session fork <id>
conduit session replay <id>
```

Forking creates a new session that branches from a chosen turn — useful for
"what if I'd asked it differently" exploration.

## Memory

Three layers, in priority order:

1. **`SOUL.md`** — your constitution. Rarely edited.
2. **`USER.md`** — your preferences. Edit freely.
3. **Curated memory entries** — facts the agent has stored, browsable in the
   TUI memory inspector (`Ctrl+M`).

Pick the storage backend in `config.yaml`:

```yaml
memory:
  provider: sqlite   # or flatfile, lancedb
```

The SQLite provider supports FTS5 full-text search; LanceDB adds vector
similarity.
