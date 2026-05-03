# Configuration reference

All configuration lives in `~/.conduit/config.yaml`. The file is created on
first launch with sensible defaults.

## Top-level keys

| Key          | Type   | Default            | Description                                |
| ------------ | ------ | ------------------ | ------------------------------------------ |
| `providers`  | list   | `[]`               | Ordered provider list — see below.         |
| `routing`    | object | see below          | Cost-aware router knobs.                   |
| `memory`     | object | `{ provider: sqlite }` | Memory backend selection.              |
| `coding`     | object | see below          | `conduit code` defaults.                   |
| `hooks`      | list   | `[]`               | Subprocess hooks to run on loop events.    |
| `workflows`  | object | `{ dir: ~/.conduit/workflows }` | Workflow loader options.       |
| `keybindings` | path  | `~/.conduit/keybindings.json` | Override TUI keybindings.         |

## Providers

```yaml
providers:
  - name: anthropic
    kind: anthropic
    models: [claude-sonnet-4-7, claude-opus-4-7]
    priority: 1
  - name: ollama
    kind: openai-compatible
    base_url: http://localhost:11434/v1
    models: [llama3:70b]
    priority: 5
```

## Routing

```yaml
routing:
  strategy: cost-cascade   # or round-robin, sticky, manual
  escalate_on:
    - low_confidence
    - first_run
    - high_stakes
```

## Coding

```yaml
coding:
  permission_tier: write   # read-only | write | shell | unsafe
  auto_continue: true
```

## Workflows

```yaml
workflows:
  dir: ~/.conduit/workflows
  checkpoint_dir: ~/.conduit/checkpoints
```
