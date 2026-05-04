# Plugin development guide

A plugin is a directory with a `plugin.yaml` manifest and any number of
sibling files referenced from it.

## Minimal plugin

```
~/.conduit/plugins/hello/
├── plugin.yaml
└── greet.sh
```

```yaml
# plugin.yaml
name: hello
version: 0.1.0
description: Adds a greet tool.

tools:
  - name: greet
    description: Say hello.
    command: ./greet.sh
    permission: read-only
    schema:
      type: object
      properties:
        name: { type: string }
      required: [name]
```

```sh
#!/usr/bin/env bash
# greet.sh
input=$(cat)
name=$(jq -r .name <<< "$input")
echo "{\"output\": \"Hello, $name!\"}"
```

Restart Conduit and the agent will see `greet` in its tool list.

## Hooks

Plugins can register hooks at every loop point — `pre_tool`, `post_tool`,
`pre_model`, `post_model`, `session_start`, `session_end`. Add them under
`hooks:` in the manifest.

## Agent profiles

Drop markdown files under a `profiles/` subdirectory. Each becomes a
selectable agent persona.

## Permissions

Tools must declare a tier (`read-only`, `write`, `shell`, `unsafe`). Conduit
enforces the tier at runtime against the user's coding permission setting.

## Distribution

Push the directory to GitHub. Users `git clone` it into
`~/.conduit/plugins/`. A package registry is on the roadmap.
