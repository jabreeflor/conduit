# Workflows

A workflow is a YAML-defined graph of steps. Each step is an agent turn, a tool
call, or a sub-workflow. The engine checkpoints between steps so a workflow
survives provider failures and can be resumed or re-routed.

## Example

```yaml
name: weekly-digest
schedule: "0 9 * * MON"
steps:
  - id: fetch
    tool: web_search
    input: { query: "AI safety news this week" }
  - id: summarize
    agent: writer
    input: "{{ steps.fetch.output }}"
  - id: deliver
    tool: email_send
    input:
      to: me@example.com
      body: "{{ steps.summarize.output }}"
```

Save it under `~/.conduit/workflows/`, then:

```sh
conduit workflow run weekly-digest
conduit workflow schedule weekly-digest
```

See [configuration reference](../reference/configuration#workflows) for the
full schema.
