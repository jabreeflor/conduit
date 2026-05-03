# Coding agent

`conduit code` is a REPL that gives the agent the standard coding tool set
(`read_file`, `write_file`, `edit_file`, `bash`, `grep`, `glob`, `notebook_edit`)
inside your current working directory.

```sh
cd ~/code/myproject
conduit code
```

## Permission tiers

Tools run inside one of four tiers:

| Tier      | Allows                                           |
| --------- | ------------------------------------------------ |
| read-only | `read_file`, `grep`, `glob`                      |
| write     | + `write_file`, `edit_file`, `notebook_edit`     |
| shell     | + `bash`                                         |
| unsafe    | + tools the plugin author flagged as unsafe      |

Set the default in `~/.conduit/config.yaml` under `coding.permission_tier`.

## CLAUDE.md discovery

On startup, `conduit code` walks up from the current directory and merges every
`CLAUDE.md` it finds into the system prompt. Project-level instructions win.

## Sessions

Every REPL turn is appended to a JSONL session under `~/.conduit/sessions/`.
Use the [memory & sessions](./memory-sessions) guide to fork or replay.
