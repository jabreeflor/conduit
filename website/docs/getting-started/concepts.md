# Concepts

A short glossary so the rest of the docs make sense.

**Provider.** A model endpoint Conduit can call — Anthropic, OpenAI, Ollama,
vLLM, OpenRouter, LiteLLM, anything OpenAI/Anthropic-compatible.

**Router.** Picks a provider per request based on cost, latency, capability,
and your declared priorities. Failover and escalation happen here.

**Session.** A conversation. Stored as JSONL on disk. Forkable, replayable,
inspectable like a Git commit.

**Memory.** Long-term context that survives across sessions. Three layers:
`SOUL.md` (constitution), `USER.md` (your prefs), and curated memory entries.

**Hook.** A shell subprocess Conduit runs at well-defined points in the agent
loop. Hooks see a JSON event on stdin and can mutate or veto via stdout.

**Tool.** A function the agent can call — `read_file`, `bash`, `web_search`,
or anything you add via the plugin runtime.

**Plugin.** A bundle of tools, hooks, agent profiles, and config that extends
Conduit without forking it.

**Workflow.** A YAML-defined, checkpointable, schedulable graph of agent
steps. Survives provider failures by re-routing.
