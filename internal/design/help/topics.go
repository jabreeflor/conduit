package help

// Built-in topics. New topics get added here; the docs site references the
// same IDs as deep-link slugs for the contextual "Learn more" buttons.
func init() {
	for _, t := range builtinTopics {
		Default.AddTopic(t)
	}
}

var builtinTopics = []Topic{
	{
		ID:    "providers",
		Title: "Providers",
		Body: `Providers are the model endpoints Conduit can call — Anthropic,
OpenAI, Ollama, vLLM, OpenRouter, LiteLLM, anything OpenAI- or
Anthropic-compatible.

Add one with ` + "`conduit provider add <kind>`" + ` and Conduit will prompt
for credentials. Keys are stored in the macOS Keychain — never on disk in
plaintext.`,
		Related:   []string{"router", "config"},
		LearnMore: "https://github.com/jabreeflor/conduit/blob/main/website/docs/getting-started/concepts.md",
	},
	{
		ID:    "router",
		Title: "Model router",
		Body: `The router picks a provider per request based on cost, latency,
capability, and the order you set in config.yaml. Failover and "low
confidence" escalation happen here.

Strategies: cost-cascade (default), round-robin, sticky, manual.`,
		Related:   []string{"providers", "config"},
		LearnMore: "https://github.com/jabreeflor/conduit/blob/main/website/docs/reference/configuration.md#routing",
	},
	{
		ID:    "sessions",
		Title: "Sessions",
		Body: `A session is a conversation, stored as JSONL under
~/.conduit/sessions/. Sessions are forkable, replayable, and inspectable
like git commits.

Try ` + "`/sessions list`" + ` or press Ctrl+S to open the browser.`,
		Related:   []string{"memory"},
		LearnMore: "https://github.com/jabreeflor/conduit/blob/main/website/docs/guides/memory-sessions.md",
	},
	{
		ID:    "memory",
		Title: "Memory",
		Body: `Three layers, in priority order:
  1. SOUL.md — your constitution.
  2. USER.md — your preferences.
  3. Curated entries — facts the agent has stored.

Open the inspector with Ctrl+M to browse, search, or prune entries.`,
		Related:   []string{"sessions"},
		LearnMore: "https://github.com/jabreeflor/conduit/blob/main/website/docs/guides/memory-sessions.md",
	},
	{
		ID:    "hooks",
		Title: "Hooks",
		Body: `Hooks are shell subprocesses Conduit runs at well-defined
points in the agent loop (pre_tool, post_tool, pre_model, post_model,
session_start, session_end).

A hook reads a JSON event on stdin and can mutate or veto via stdout.`,
	},
	{
		ID:    "workflows",
		Title: "Workflows",
		Body: `A workflow is a YAML-defined graph of steps that the engine
checkpoints between, so it survives provider failures and can resume.

Drop a YAML file under ~/.conduit/workflows/ and run with
` + "`conduit workflow run <name>`" + `.`,
		LearnMore: "https://github.com/jabreeflor/conduit/blob/main/website/docs/guides/workflows.md",
	},
	{
		ID:    "coding",
		Title: "Coding agent",
		Body: `` + "`conduit code`" + ` is a REPL with the standard coding
toolset (read_file, write_file, edit_file, bash, grep, glob,
notebook_edit). Permission tiers: read-only, write, shell, unsafe.

CLAUDE.md files in the repo are merged into the system prompt
automatically.`,
		LearnMore: "https://github.com/jabreeflor/conduit/blob/main/website/docs/guides/coding-agent.md",
	},
	{
		ID:    "config",
		Title: "Configuration",
		Body: `All config lives in ~/.conduit/config.yaml. Top-level keys:
providers, routing, memory, coding, hooks, workflows, keybindings.

Run ` + "`conduit config edit`" + ` to open it in your $EDITOR.`,
		LearnMore: "https://github.com/jabreeflor/conduit/blob/main/website/docs/reference/configuration.md",
	},
	{
		ID:    "keybindings",
		Title: "Keybindings",
		Body: `Defaults live in code; overrides go in
~/.conduit/keybindings.json. Common ones:
  Ctrl+S — sessions browser
  Ctrl+M — memory inspector
  Ctrl+Q — quit
  Enter  — submit message
  Shift+Enter — newline`,
		LearnMore: "https://github.com/jabreeflor/conduit/blob/main/website/docs/reference/keybindings.md",
	},
	{
		ID:    "plugins",
		Title: "Plugins",
		Body: `Plugins extend Conduit with new tools, hooks, agent profiles,
and config without forking the binary. Drop a plugin directory under
~/.conduit/plugins/ — it's discovered on startup.`,
		LearnMore: "https://github.com/jabreeflor/conduit/blob/main/website/docs/plugins/development.md",
	},
	{
		ID:    "usage",
		Title: "Usage & cost",
		Body: `Conduit keeps a local ledger at ~/.conduit/usage.db. Run
` + "`conduit usage`" + ` for the dashboard, or open the in-TUI panel from
the status bar.

Nothing leaves your machine.`,
	},
}
