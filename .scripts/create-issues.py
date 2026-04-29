#!/usr/bin/env python3
"""Bulk-create GitHub issues for Conduit from PRD sections."""
import subprocess
import sys
import time

# (title, body, labels[], milestone_or_None)
ISSUES = [
    # ───────────────────────────────────────────────────────────────────
    # PHASE 1 — CORE ENGINE + TUI (P0 foundational)
    # ───────────────────────────────────────────────────────────────────
    (
        "Decide core language stack (Python vs Go vs Rust)",
        "**Source:** PRD §19 Phase 1\n\n"
        "Finalize the core language for the Conduit engine. Trade-offs:\n"
        "- **Python:** fastest iteration, strong AI ecosystem, slower runtime\n"
        "- **Go:** great concurrency, fast cold start, weaker AI tooling\n"
        "- **Rust:** strongest performance and safety, slowest dev velocity\n\n"
        "**Deliverable:** ADR documenting the choice, rationale, and how it interacts with the TUI stack decision (#TUI-stack).",
        ["Needs Review", "P0", "core", "infra"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "Decide TUI stack (Textual vs Bubble Tea)",
        "**Source:** PRD §11.1, §19 Phase 1\n\n"
        "Choose between Textual (Python) and Bubble Tea (Go). Decision must align with the core language ADR.\n\n"
        "**Deliverable:** ADR + a small spike (chat panel + status bar) demonstrating the chosen stack handles the three-panel layout, key bindings, and tool-call blocks.",
        ["Needs Review", "P0", "tui", "design-task"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "Project scaffolding and monorepo setup",
        "**Source:** PRD §19 Phase 1\n\n"
        "Scaffold the repo with the agreed core/contracts/TUI/GUI package structure. Wire CI for lint, type-check, unit tests, and a release build.\n\n"
        "Depends on the core language ADR.",
        ["Needs Review", "P0", "infra", "core"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "Logo design — Conduit wordmark and icon",
        "**Source:** PRD §19 Phase 1 (explicit GitHub Issue callout)\n\n"
        "Create the Conduit wordmark and icon. The icon must work at:\n"
        "- App icon (512×512)\n"
        "- Menu bar (22×22)\n"
        "- TUI status bar glyph\n"
        "- Favicon\n\n"
        "Concepts to explore: connectivity / signal flow, terminal aesthetics, minimal geometric forms.\n\n"
        "**Deliverables:** SVG source + PNG exports at all required sizes. Must be finalized before any public-facing surface ships.",
        ["Needs Review", "P0", "design-task", "design-system"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "Multi-fidelity TUI mockups",
        "**Source:** PRD §19 Phase 1 (explicit GitHub Issue callout)\n\n"
        "Generate mockups (wireframe → grayscale → color) covering: three-panel layout, status bar, tool call blocks, context panel views (workflow / memory / hooks / session tree), diff view, command palette overlay.\n\n"
        "**Must be reviewed and approved before TUI build begins.**",
        ["Needs Review", "P0", "tui", "design-task"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "Model Router — Claude + OpenAI integration with failover",
        "**Source:** PRD §6.1\n\n"
        "The model router is the nervous system of Conduit. Implement:\n"
        "- Prioritized provider list (primary + fallback chain)\n"
        "- Per-task routing rules (code → Claude, browser → Codex, image → vision-capable model)\n"
        "- Configurable timeouts per provider\n"
        "- Automatic failover with last-checkpoint resume\n"
        "- Unified inference API consumed by the rest of the engine\n"
        "- Per-provider cost/token tracking\n\n"
        "**Config example:** YAML at `~/.conduit/config.yaml` (see PRD §6.1).",
        ["Needs Review", "P0", "router", "core"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "Model Escalation — cheap → expensive routing",
        "**Source:** PRD §6.2\n\n"
        "Auto-escalate from a default cheap/fast model to a more capable one when:\n"
        "- Confidence score < threshold\n"
        "- First time a workflow type runs\n"
        "- Task tagged destructive/publish/financial\n"
        "- Cheap model self-signals uncertainty\n\n"
        "Escalation must be transparent in the session log and TUI status bar.",
        ["Needs Review", "P0", "router", "core"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "Three-layer agent identity (SOUL.md + USER.md + memory)",
        "**Source:** PRD §6.3\n\n"
        "Implement the three-layer identity system:\n"
        "- **SOUL.md** — agent constitution, injected last so it overrides\n"
        "- **USER.md** — user preferences and constraints\n"
        "- **Memory** — short-term, long-term episodic, skill memory\n\n"
        "Long-term memory entries are flat markdown files in `~/.conduit/memory/` (Spotlight-indexed).",
        ["Needs Review", "P0", "identity", "memory", "core"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "Prompt injection detection layer",
        "**Source:** PRD §6.5\n\n"
        "Scan every piece of injected content (memory, files, web fetches, tool outputs) for injection patterns before the model call:\n"
        "- 'SYSTEM OVERRIDE', 'IGNORE INSTRUCTIONS', etc.\n"
        "- Shell injection: backticks, redirect operators, `cat`, `less`\n"
        "- File exfiltration attempts\n"
        "- Invisible Unicode (zero-width, RTL override)\n\n"
        "On detection: log, strip, notify the user.",
        ["Needs Review", "P0", "security", "core"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "Tool policy pipeline",
        "**Source:** PRD §6.12\n\n"
        "Filter every tool call through ordered stages:\n"
        "1. Base tools\n"
        "2. Agent overrides (from SOUL.md)\n"
        "3. Policy filter (`~/.conduit/policies.yaml`)\n"
        "4. Schema normalization (Anthropic vs OpenAI quirks)\n"
        "5. AbortSignal wrapping\n\n"
        "Policies support `block`, `allow`, `require_confirmation`, `allowed_domains`, and per-agent restrictions.",
        ["Needs Review", "P0", "security", "core"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "Structured reply tag parser",
        "**Source:** PRD §6.14\n\n"
        "Parse and act on agent-emitted reply tags:\n"
        "- `[[silent]]` — suppress output\n"
        "- `[[voice: text]]` — speak via TTS\n"
        "- `[[media: url]]` — render media inline\n"
        "- `[[heartbeat]]` — keep-alive for long ops\n"
        "- `[[canvas: html]]` — render to GUI Canvas panel",
        ["Needs Review", "P1", "core"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "Session JSONL tree (fork / replay / inspect)",
        "**Source:** PRD §6.13\n\n"
        "Persist sessions as JSONL with parent-child links per turn. Implement:\n"
        "- Fork from any turn\n"
        "- Replay from any checkpoint with different model/params\n"
        "- TUI session tree browser\n"
        "- Slash commands: `/sessions list|fork|replay|load`",
        ["Needs Review", "P1", "core"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "Configurable keybindings system",
        "**Source:** PRD §6.15\n\n"
        "All TUI/GUI shortcuts user-configurable via `~/.conduit/keybindings.json`. Inspired by T3 Code's keybindings system. Must support all available commands (`chat.new`, `session.fork`, `commandPalette.toggle`, `conduit.summon`, etc).",
        ["Needs Review", "P1", "tui", "core"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "Credential pooling with health-check rotation",
        "**Source:** PRD §6.11\n\n"
        "Round-robin pool of API keys per provider. On 401 / rate-limit, mark a key unhealthy for a backoff window and use the next.\n\n"
        "Credentials must be read from env vars or macOS Keychain — never plaintext config.",
        ["Needs Review", "P0", "security", "core"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "Cost tracker — per-session JSONL log",
        "**Source:** PRD §6.10\n\n"
        "Log every model call to `~/.conduit/usage.jsonl` with input/output tokens, cost, and provider. Status bar shows `[Model] [Tokens] [$X.XX]` updating after every call. Feeds Section 14 analytics.",
        ["Needs Review", "P0", "analytics", "core"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "Error classification system",
        "**Source:** PRD §6.9\n\n"
        "Classify every tool/model failure into a typed error and apply the right recovery strategy: NetworkError (3x exp backoff), TimeoutError (retry once longer, then skip), RateLimitedError (backoff or fallback), PermissionError (escalate to user), InvalidInputError (log + skip), ModelUnavailableError (next fallback), UnknownError (log + continue).\n\nFailed tool calls remain visible to the agent so it can adapt.",
        ["Needs Review", "P0", "core"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "TUI: three-panel layout",
        "**Source:** PRD §11.1\n\n"
        "Conversation panel (left), context panel (right, toggleable with `mod+p`), input field (bottom). Must support resizing and follow the Conduit Design tokens.",
        ["Needs Review", "P0", "tui"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "TUI: status bar",
        "**Source:** PRD §11.1\n\n"
        "Active model, cumulative session cost, session ID, active workflow name (if running). Updates after every model call.",
        ["Needs Review", "P0", "tui"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "TUI: collapsible tool-call blocks",
        "**Source:** PRD §11.1\n\n"
        "Render each tool call as a collapsible block with status indicator (⟳ running / ✓ done / ✗ failed). Expand to show input/output.",
        ["Needs Review", "P0", "tui"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "TUI: context panel (workflow / memory / hooks / session tree)",
        "**Source:** PRD §11.1\n\n"
        "Side panel that toggles between four views: workflow step visualization, memory browser, hook log, session tree. Toggleable via `mod+p` or slash command.",
        ["Needs Review", "P0", "tui"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "Config system (YAML at ~/.conduit/config.yaml)",
        "**Source:** PRD §6.1, §19 Phase 1\n\n"
        "Single root YAML config covering models, escalation, hooks, policies, sandbox, budgets. Project-scoped overrides at `.conduit/config.yaml`. Inheritance with explicit precedence.",
        ["Needs Review", "P0", "core", "infra"],
        "Phase 1 — Core Engine + TUI",
    ),

    # ───────────────────────────────────────────────────────────────────
    # PHASE 2 — MEMORY + HOOKS + EVALS
    # ───────────────────────────────────────────────────────────────────
    (
        "Pluggable MemoryProvider interface",
        "**Source:** PRD §6.4\n\n"
        "Abstract `MemoryProvider` class with `initialize / prefetch / write / search / compress / shutdown` and optional hooks (`on_turn_start`, `on_session_end`, `on_pre_compress`, `on_memory_write`).\n\n"
        "Only one external provider may be active at a time to prevent tool schema bloat.",
        ["Needs Review", "P0", "memory", "core"],
        "Phase 2 — Memory + Hooks + Evals",
    ),
    (
        "FlatFileProvider — Spotlight-indexed markdown memory",
        "**Source:** PRD §6.3, §6.4\n\n"
        "Default memory provider. Writes structured `.md` to `~/.conduit/memory/` so macOS Spotlight indexes them automatically. Agent curates entries after task completion.",
        ["Needs Review", "P0", "memory"],
        "Phase 2 — Memory + Hooks + Evals",
    ),
    (
        "LanceDBProvider — vector-search memory",
        "**Source:** PRD §6.4\n\n"
        "Bundled provider for semantic recall over large memory stores using LanceDB.",
        ["Needs Review", "P2", "memory"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "SQLiteProvider — structured memory with FTS",
        "**Source:** PRD §6.4\n\n"
        "Bundled provider for structured recall with full-text search.",
        ["Needs Review", "P2", "memory"],
        "Phase 2 — Memory + Hooks + Evals",
    ),
    (
        "Hook system — shell subprocess + JSON wire protocol",
        "**Source:** PRD §6.6\n\n"
        "Run hooks as subprocesses with a JSON wire protocol. Input on stdin (event, tool_name, tool_input, session_id, cwd), output on stdout (`allow` / `block` / `inject`). Timeouts enforced; crash defaults to `allow` (fail-safe).",
        ["Needs Review", "P0", "hooks"],
        "Phase 2 — Memory + Hooks + Evals",
    ),
    (
        "Wire all 7 hook points",
        "**Source:** PRD §6.6\n\n"
        "Implement: `on_session_start`, `on_session_end`, `pre_llm_call`, `post_llm_call`, `pre_tool_call`, `post_tool_call`, `on_memory_write`. Each must support optional regex matchers and per-event timeouts.",
        ["Needs Review", "P0", "hooks"],
        "Phase 2 — Memory + Hooks + Evals",
    ),
    (
        "Hook consent gate (first-use confirmation)",
        "**Source:** PRD §6.6\n\n"
        "Hook registration requires user confirmation on first use. Non-interactive callers (API, cron) require `accept_hooks: true` in config. Codex-aligned events (`PreToolUse`, `PostToolUse`, `PermissionRequest`, `UserPromptSubmit`, `Stop`, `agent-turn-complete`, `approval-requested`) must be supported (PRD §6.25.17).",
        ["Needs Review", "P0", "hooks", "security"],
        "Phase 2 — Memory + Hooks + Evals",
    ),
    (
        "Eval framework — custom suites + harness metrics",
        "**Source:** PRD §6.23\n\n"
        "Two modes:\n"
        "1. **Custom evals** — YAML suites with assertions (`tool_calls_include`, `reply_contains_tag`, `cost_max_usd`, etc)\n"
        "2. **Harness metrics** — auto-collected per-model scorecards over real sessions (instruction follow rate, tool selection accuracy, latency, etc)\n\n"
        "CLI: `conduit eval run | compare | report | replay`. Results stored at `~/.conduit/evals/results/` as JSONL.",
        ["Needs Review", "P1", "eval"],
        "Phase 2 — Memory + Hooks + Evals",
    ),
    (
        "Replay-as-eval — re-run a session with a different model",
        "**Source:** PRD §6.23\n\n"
        "`conduit eval replay <session-id> --model gpt-4o --diff` — turns any real session into a regression test. Useful for model upgrade evaluation.",
        ["Needs Review", "P2", "eval"],
        "Phase 2 — Memory + Hooks + Evals",
    ),
    (
        "Memory inspector in TUI",
        "**Source:** PRD §19 Phase 2\n\n"
        "Browse, search, and inspect memory entries from inside the TUI. Pruning and manual override controls.",
        ["Needs Review", "P1", "tui", "memory"],
        "Phase 2 — Memory + Hooks + Evals",
    ),

    # ───────────────────────────────────────────────────────────────────
    # PHASE 3 — WORKFLOW ENGINE
    # ───────────────────────────────────────────────────────────────────
    (
        "Workflow engine — schema, parser, validator",
        "**Source:** PRD §6.7\n\n"
        "YAML-defined workflows with steps, branching, subagent spawning, and scheduling. Schema must support tool calls, model invocations, conditional branches, and template variable interpolation (`{{ step.output }}`).",
        ["Needs Review", "P0", "workflow"],
        "Phase 3 — Workflow Engine",
    ),
    (
        "Workflow checkpointing & resume",
        "**Source:** PRD §6.7\n\n"
        "Persist workflow state to disk after every step. On crash or model failure, resume from the last checkpoint. Model failover mid-workflow re-routes to the next provider in the chain.",
        ["Needs Review", "P0", "workflow"],
        "Phase 3 — Workflow Engine",
    ),
    (
        "Workflow conditional branching",
        "**Source:** PRD §6.7\n\n"
        "Steps can have conditional branches based on previous step output. Support comparison operators, regex matching, and JSONPath expressions.",
        ["Needs Review", "P1", "workflow"],
        "Phase 3 — Workflow Engine",
    ),
    (
        "Workflow scheduler (cron)",
        "**Source:** PRD §6.7\n\n"
        "Trigger workflows on cron schedule. Persistent across reboots. Schedule visible in TUI/GUI.",
        ["Needs Review", "P1", "workflow"],
        "Phase 3 — Workflow Engine",
    ),
    (
        "Workflow status display in TUI",
        "**Source:** PRD §19 Phase 3\n\n"
        "Numbered step list with current step highlighted, completed steps solid, failed steps red. Cost and duration per step.",
        ["Needs Review", "P1", "workflow", "tui"],
        "Phase 3 — Workflow Engine",
    ),

    # ───────────────────────────────────────────────────────────────────
    # PHASE 4 — COMPUTER USE
    # ───────────────────────────────────────────────────────────────────
    (
        "Integrate open-codex-computer-use as MCP server",
        "**Source:** PRD §6.8\n\n"
        "Computer use on macOS via the MCP server at https://github.com/iFurySt/open-codex-computer-use. Uses Accessibility API + Screen Recording.",
        ["Needs Review", "P0", "computer-use"],
        "Phase 4 — Computer Use",
    ),
    (
        "macOS permissions flow — Screen Recording + Accessibility",
        "**Source:** PRD §6.8\n\n"
        "First-launch flow that detects missing permissions, opens System Settings to the right pane, and verifies grant before allowing computer-use sessions to start.",
        ["Needs Review", "P0", "computer-use", "infra"],
        "Phase 4 — Computer Use",
    ),
    (
        "Per-app approval system with persistent config",
        "**Source:** PRD §6.8\n\n"
        "Before Conduit can control any app, the user must explicitly approve it. Approvals persist and can be revoked anytime.",
        ["Needs Review", "P0", "computer-use", "security"],
        "Phase 4 — Computer Use",
    ),
    (
        "Capability adapters — Shell / Browser / Desktop",
        "**Source:** PRD §6.8\n\n"
        "Three modular, opt-in capability levels:\n"
        "- **Shell** — terminal commands, file ops, code execution\n"
        "- **Browser** — Chrome MCP\n"
        "- **Desktop** — full macOS GUI control",
        ["Needs Review", "P0", "computer-use"],
        "Phase 4 — Computer Use",
    ),
    (
        "Pre/post screenshots per computer-use step",
        "**Source:** PRD §6.8\n\n"
        "Capture and store a screenshot before and after every computer-use step in the session log for audit and debugging.",
        ["Needs Review", "P0", "computer-use"],
        "Phase 4 — Computer Use",
    ),
    (
        "Safety guardrails for destructive computer-use actions",
        "**Source:** PRD §6.8\n\n"
        "Always checkpoint before destructive actions. Never send email, post publicly, or execute financial transactions without explicit confirmation.",
        ["Needs Review", "P0", "computer-use", "security"],
        "Phase 4 — Computer Use",
    ),

    # ───────────────────────────────────────────────────────────────────
    # PHASE 5 — GUI + SPOTLIGHT UI
    # ───────────────────────────────────────────────────────────────────
    (
        "Decide GUI stack (SwiftUI vs Tauri vs Electron)",
        "**Source:** PRD §11.2, §19 Phase 5\n\n"
        "Trade-offs:\n"
        "- **SwiftUI** — best native macOS feel, hardest to share with iOS package\n"
        "- **Tauri** — small footprint, web stack, weaker native feel\n"
        "- **Electron** — fastest dev, heaviest runtime\n\n"
        "Decision must consider sharing components with the iOS companion app (§9) and the watchOS complication (§9.7).",
        ["Needs Review", "P1", "gui", "design-task"],
        "Phase 5 — GUI + Spotlight UI",
    ),
    (
        "Multi-fidelity GUI mockups",
        "**Source:** PRD §19 Phase 5 (explicit GitHub Issue callout)\n\n"
        "Wireframe → grayscale → color, covering: three-column layout, computer-use screenshot stream, Canvas panel, workflow DAG, Spotlight UI overlay, session tree browser, SOUL.md/USER.md editor, diff view in main content. Design language must reflect Claude Desktop + Codex + T3 Code inspiration.\n\n"
        "**Must be reviewed and approved before GUI build begins.**",
        ["Needs Review", "P1", "gui", "design-task"],
        "Phase 5 — GUI + Spotlight UI",
    ),
    (
        "GUI: three-column layout (Sidebar / Main / Agent Panel)",
        "**Source:** PRD §11.2\n\n"
        "Sidebar (left, collapsible) for Sessions/Workflows/Memory/Skills. Main content area for screenshot stream, Canvas, workflow DAG, or memory browser. Agent panel (right) always visible with chat + status.",
        ["Needs Review", "P1", "gui"],
        "Phase 5 — GUI + Spotlight UI",
    ),
    (
        "GUI: computer-use screenshot stream view",
        "**Source:** PRD §11.2\n\n"
        "Live-updating, full-width, timestamped per step — Codex style. Pinning support for persisting screenshots into context.",
        ["Needs Review", "P1", "gui", "computer-use"],
        "Phase 5 — GUI + Spotlight UI",
    ),
    (
        "GUI: Canvas panel (WKWebView for [[canvas:html]])",
        "**Source:** PRD §11.2, §6.14\n\n"
        "Side panel where the agent renders interactive HTML via `[[canvas: html]]` reply tags. Useful for dashboards, forms, rich outputs.",
        ["Needs Review", "P1", "gui"],
        "Phase 5 — GUI + Spotlight UI",
    ),
    (
        "GUI: workflow DAG visualization",
        "**Source:** PRD §11.2\n\n"
        "Nodes as rounded rectangles, current step pulsing, completed steps solid, failed steps red. Click any node to inspect its inputs/outputs.",
        ["Needs Review", "P1", "gui", "workflow"],
        "Phase 5 — GUI + Spotlight UI",
    ),
    (
        "GUI: session tree browser (fork / replay)",
        "**Source:** PRD §11.2, §6.13\n\n"
        "Visual session tree allowing fork from any turn and replay with different model/params.",
        ["Needs Review", "P1", "gui"],
        "Phase 5 — GUI + Spotlight UI",
    ),
    (
        "Conduit Spotlight UI — Raycast-style overlay",
        "**Source:** PRD §11.4\n\n"
        "Global hotkey (default `⌥Space`) summons a centered floating panel from any context. Single input field. Inline responses for simple queries; hand-off to TUI/GUI for complex tasks. Recent commands and memory search surfaced as you type. Named workflow trigger by name.",
        ["Needs Review", "P1", "spotlight"],
        "Phase 5 — GUI + Spotlight UI",
    ),

    # ───────────────────────────────────────────────────────────────────
    # PHASE 6 — SKILLS + MULTI-AGENT + ADVANCED
    # ───────────────────────────────────────────────────────────────────
    (
        "Skills registry — schema, storage, hierarchy",
        "**Source:** PRD §6.18\n\n"
        "Skills stored in `~/.conduit/skills/` and indexed at startup. Hierarchy: workspace > personal > imported > bundled. Indexed semantic search at task start.",
        ["Needs Review", "P1", "skills"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Universal Skill Adapter — import from any source",
        "**Source:** PRD §6.17\n\n"
        "Detect and normalize skills from: Claude SKILL.md, Hermes skills, OpenClaw SOUL.md, awesome-openclaw templates, Cursor rules, AGENTS.md, plain markdown.\n\n"
        "CLI: `conduit skills import | sync | list`. Supports `--from hermes`, `--from openclaw`. Deduplicates on name conflict.",
        ["Needs Review", "P1", "skills"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Skill auto-generation from novel tasks",
        "**Source:** PRD §19 Phase 6\n\n"
        "After a complex task completes, the agent reflects on what it did and writes a reusable skill. Loaded via semantic search next time a similar task starts.",
        ["Needs Review", "P2", "skills"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Subagent spawning from workflow steps",
        "**Source:** PRD §6.7, §6.25.9\n\n"
        "Workflow step can spawn a subagent with isolated SOUL scope and memory. Modes: isolated / delegated / full memory. Result aggregation back to orchestrator.",
        ["Needs Review", "P1", "workflow"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Smart Consensus Mode (multi-model deliberation)",
        "**Source:** PRD §18\n\n"
        "Harness-triggered: on high-stakes / low-confidence / first-run / repeatedly-failing tasks, spawn multiple models on the same task and surface disagreements to the user.",
        ["Needs Review", "P2", "router"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Passive observational learning (consent-gated)",
        "**Source:** PRD §18\n\n"
        "With explicit user consent, watch macOS activity in the background. Detect repeated patterns and surface automation suggestions. No data leaves the machine.",
        ["Needs Review", "P2", "core"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Multimodal input — images, PDFs, screenshots",
        "**Source:** PRD §6.22\n\n"
        "Treat images, screenshots, PDFs, and (later) audio as first-class inputs. Vision-aware routing auto-promotes to vision-capable models. TUI: `@image path`, `@pdf path`. GUI: drag-drop and clipboard paste.",
        ["Needs Review", "P1", "core", "router"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Command palette + $skills inline composer",
        "**Source:** PRD §6.21\n\n"
        "`mod+k` opens a fuzzy-search palette covering all sessions/workflows/memory/skills/models actions. `$` in the chat input invokes a skill inline (T3 Code-inspired).",
        ["Needs Review", "P2", "gui", "tui", "skills"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),

    # ───────────────────────────────────────────────────────────────────
    # PHASE 7 — CODING AGENT ENGINE (§6.24)
    # ───────────────────────────────────────────────────────────────────
    (
        "`conduit code` REPL — coding agent entry point",
        "**Source:** PRD §6.24.1\n\n"
        "Dedicated multi-turn coding REPL. Token-by-token streaming, truncated-response auto-continuation, session persistence independent of general sessions, file history journaling with snapshot IDs.",
        ["Needs Review", "P1", "coding-agent"],
        "Phase 7 — Coding Agent Engine",
    ),
    (
        "Coding-agent context management",
        "**Source:** PRD §6.24.2\n\n"
        "CLAUDE.md / `.conduit/rules/*.md` discovery, auto-snip and auto-compact at configurable thresholds, reactive compaction on prompt-too-long, preflight token-budget reporting, tokenizer-aware accounting, pasted-content collapsing (≥500 chars → chip references).",
        ["Needs Review", "P1", "coding-agent", "performance"],
        "Phase 7 — Coding Agent Engine",
    ),
    (
        "Core coding tool set with permission tiers",
        "**Source:** PRD §6.24.4\n\n"
        "Tools: `list_dir`, `read_file`, `glob_search`, `grep_search`, `web_fetch`, `web_search`, `tool_search`, `sleep` (always); `write_file`, `edit_file`, `notebook_edit` (--allow-write); `bash` (--allow-shell). Tier visible in TUI status bar and GUI agent panel.",
        ["Needs Review", "P1", "coding-agent", "security"],
        "Phase 7 — Coding Agent Engine",
    ),
    (
        "Plugin runtime — manifest, lifecycle hooks, virtual tools",
        "**Source:** PRD §6.24.5\n\n"
        "Manifest-based plugin discovery (`plugins/*/plugin.json`). Hooks: `beforePrompt`, `afterTurn`, `onResume`, `beforePersist`, `beforeDelegate`, `afterDelegate`. Tool aliases, virtual tools with response templates, tool blocking. Plugin session-state persistence.",
        ["Needs Review", "P1", "plugin", "coding-agent"],
        "Phase 7 — Coding Agent Engine",
    ),
    (
        "Nested agent delegation",
        "**Source:** PRD §6.24.6\n\n"
        "`delegate_agent` tool with dependency-aware topological batching (sequential + parallel). Agent manager with lineage tracking. Child-session save/resume from parent context. Batch summaries for nested results.",
        ["Needs Review", "P1", "coding-agent"],
        "Phase 7 — Coding Agent Engine",
    ),
    (
        "Custom agent profiles",
        "**Source:** PRD §6.24.7\n\n"
        "Markdown profiles at `.conduit/agents/*.md` (project) and `~/.conduit/agents/*.md` (user). Project overrides user. CLI: `conduit agents-create | agents-update | agents-delete`. Fields: name, description, tools, model, initialPrompt.",
        ["Needs Review", "P1", "coding-agent"],
        "Phase 7 — Coding Agent Engine",
    ),
    (
        "Fine-grained budget control for coding sessions",
        "**Source:** PRD §6.24.8\n\n"
        "Flags: `--max-total-tokens`, `--max-input-tokens`, `--max-output-tokens`, `--max-reasoning-tokens`, `--max-budget-usd`, `--max-tool-calls`, `--max-model-calls`, `--max-session-turns`, `--max-delegated-tasks`. All editable at runtime in the GUI settings panel. Feeds the eval scorecard.",
        ["Needs Review", "P1", "coding-agent", "analytics"],
        "Phase 7 — Coding Agent Engine",
    ),
    (
        "LSP code intelligence runtime",
        "**Source:** PRD §6.24.11\n\n"
        "Local LSP-style runtime: go-to-definition, find-references, hover docs, document/workspace symbols, call hierarchy, diagnostics. Surfaces errors/warnings inline in agent context.",
        ["Needs Review", "P1", "coding-agent"],
        "Phase 7 — Coding Agent Engine",
    ),
    (
        "Background & daemon coding sessions",
        "**Source:** PRD §6.24.12\n\n"
        "`conduit code-bg <prompt>`, `code-ps`, `code-logs <id>`, `code-attach <id>`, `code-kill <id>`. Daemon wrapper. Background sessions visible in GUI Background tab with live log streaming.",
        ["Needs Review", "P2", "coding-agent"],
        "Phase 7 — Coding Agent Engine",
    ),
    (
        "Coding-agent GUI tabs (Tasks/Plan/Memory/History/...)",
        "**Source:** PRD §6.24.24\n\n"
        "Tabs: Tasks, Plan, Memory, History, Background, Worktree, Skills, Accounts, Remote, MCP, Plugins, Ask queue, Workflows, Search, Triggers, Teams, Diagnostics.",
        ["Needs Review", "P2", "gui", "coding-agent"],
        "Phase 7 — Coding Agent Engine",
    ),
    (
        "Code diff visualization (TUI + GUI side-by-side)",
        "**Source:** PRD §6.19, §11.3\n\n"
        "Every file write/edit by the agent triggers a diff entry. Side-by-side and unified views. Hunk-level approve/reject. Line annotations fed back to agent. Git-aware (against last commit if tracked, else pre-session state).",
        ["Needs Review", "P1", "ide", "coding-agent"],
        "Phase 7 — Coding Agent Engine",
    ),
    (
        "Git worktree-aware sessions and `worktree_enter`/`worktree_exit`",
        "**Source:** PRD §6.20, §6.24.17\n\n"
        "Sessions record current branch and worktree. `chat.newLocal` creates a session in an isolated worktree. Mid-session cwd switching. State survives session reload. Worktree status visible in GUI Worktree tab.",
        ["Needs Review", "P1", "coding-agent", "workflow"],
        "Phase 7 — Coding Agent Engine",
    ),
    (
        "Mini IDE — code editor with syntax highlighting and AI assists",
        "**Source:** PRD §11.3\n\n"
        "Lightweight in-app editor: Tree-sitter syntax highlighting, basic autocomplete, inline AI suggestions, line numbers, soft wrap, multi-file tabs, minimap. File tree browser. Embedded terminal. External-editor launch (`Open in VS Code`, `Open in Obsidian`, configurable).",
        ["Needs Review", "P2", "ide", "gui"],
        "Phase 7 — Coding Agent Engine",
    ),

    # ───────────────────────────────────────────────────────────────────
    # PHASE 8 — COLLABORATION & CHANNEL ACCESS
    # ───────────────────────────────────────────────────────────────────
    (
        "Read-only shared sessions via pairing token",
        "**Source:** PRD §18 Phase 8\n\n"
        "Pairing token scoped to read-only view. Real-time broadcast of conversation thread, tool call status, workflow step, model/cost. Owner can revoke access instantly and view active observers.",
        ["Needs Review", "P2", "collab", "remote"],
        "Phase 8 — Collaboration & Channel Access",
    ),
    (
        "Active co-sessions (multi-author messages)",
        "**Source:** PRD §18 Phase 8\n\n"
        "Both participants can send messages. Messages attributed to author. Owner controls approval gates, SOUL.md edits, computer-use confirmation. Conflict resolution for simultaneous submission.",
        ["Needs Review", "P2", "collab"],
        "Phase 8 — Collaboration & Channel Access",
    ),
    (
        "Discord channel relay",
        "**Source:** PRD §18 Phase 8\n\n"
        "Discord bot account joins server or DM. Messages route to Conduit core; reply lands back in the channel. Bot has no filesystem, shell, or computer-use access — relay only. Prompt injection scan on inbound messages.",
        ["Needs Review", "P2", "collab"],
        "Phase 8 — Collaboration & Channel Access",
    ),
    (
        "Slack channel relay",
        "**Source:** PRD §18 Phase 8\n\n"
        "Slack app with bot account. `@conduit` mention triggers in any channel the bot is in. Same routing model as Discord.",
        ["Needs Review", "P2", "collab"],
        "Phase 8 — Collaboration & Channel Access",
    ),

    # ───────────────────────────────────────────────────────────────────
    # SECTION 6.16 — REMOTE ACCESS
    # ───────────────────────────────────────────────────────────────────
    (
        "Remote access & pairing (`conduit serve`)",
        "**Source:** PRD §6.16\n\n"
        "Headless server mode with Tailscale-friendly host config. One-time owner pairing token, persistent authenticated session afterwards. GUI/TUI can attach to a remote Conduit server via connection string. Returns connection string + pairing URL + QR code.",
        ["Needs Review", "P1", "remote"],
        "Phase 5 — GUI + Spotlight UI",
    ),

    # ───────────────────────────────────────────────────────────────────
    # SECTION 7 — INSTALLATION & LOCAL MODELS
    # ───────────────────────────────────────────────────────────────────
    (
        "Machine auto-detection (CPU, GPU, RAM, disk, macOS)",
        "**Source:** PRD §7.1\n\n"
        "First-launch profiling: CPU/GPU/VRAM, RAM, disk, macOS version. Cache results in `~/.conduit/machine-profile.json`. Must complete in under 3 seconds. Refresh on user-triggered re-scan.",
        ["Needs Review", "P0", "installation"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "Automatic local model installation pipeline",
        "**Source:** PRD §7.2\n\n"
        "Background install of inference runtime (Ollama / llama.cpp / LM Studio CLI) and recommended weights. Checksum verification. Detect and adopt existing installs. Resumable. All Conduit-managed files under `~/.conduit/models/`.",
        ["Needs Review", "P1", "installation"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "Hardware-aware model recommendations",
        "**Source:** PRD §7.3\n\n"
        "Machine class → recommended models (high-end / mid-range / entry-level / constrained). Estimated download size, disk footprint, expected tokens/sec on user's hardware. One-click accept top recommendation; manual browse available. Local heuristic, no phone-home.",
        ["Needs Review", "P1", "installation"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "One-click setup welcome flow",
        "**Source:** PRD §7.4\n\n"
        "End-to-end first-run flow: open app → machine profile → welcome screen → 'Set up local AI' → install runtime + model → ready. No terminal, no config editing. External-API option visible alongside local-setup path.",
        ["Needs Review", "P1", "installation"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "Model management UI (browse, download, swap, remove, health-check)",
        "**Source:** PRD §7.5\n\n"
        "Curated catalog filterable by size/capability/hardware. One-click download with progress. Hot-swappable active model without restart. Health checks for installed models. Updates router config automatically.",
        ["Needs Review", "P1", "installation", "gui"],
        "Phase 5 — GUI + Spotlight UI",
    ),
    (
        "Fallback to external endpoints (OpenAI/Anthropic/vLLM/etc)",
        "**Source:** PRD §7.6\n\n"
        "When local won't run well, present external-endpoint option with clear explanation. Validate connection before saving. Endpoints integrate into the model router alongside local models.",
        ["Needs Review", "P0", "installation", "router"],
        "Phase 1 — Core Engine + TUI",
    ),

    # ───────────────────────────────────────────────────────────────────
    # SECTION 8 — MOBILE DEVICE CONTROL
    # ───────────────────────────────────────────────────────────────────
    (
        "Mobile screen mirroring + interaction primitives",
        "**Source:** PRD §8.1\n\n"
        "Tap, long_press, swipe, type, scroll, back, home, screenshot, app_launch, app_close. Android via ADB; iOS via Xcode bridge / libimobiledevice. <200ms interaction latency. Connection state visible in status bar.",
        ["Needs Review", "P1", "mobile"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "AI-driven mobile automation loop",
        "**Source:** PRD §8.2\n\n"
        "Screenshot → vision model → identify UI elements → next action → execute → repeat. Action plan trace logged for debug. Multi-step tasks checkpointed per action. >90% tap-target accuracy on standard iOS/Android UIs.",
        ["Needs Review", "P1", "mobile"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Mobile connection methods (USB / wireless, iOS / Android)",
        "**Source:** PRD §8.3\n\n"
        "Saved device profiles, auto-reconnect on USB plug-in, pause-on-link-drop with resume on reconnect. ADB authorization handling for Android, trust-prompt detection for iOS.",
        ["Needs Review", "P1", "mobile"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Mobile safety & confirmation tiers",
        "**Source:** PRD §8.5\n\n"
        "Silent / Notify / Confirm / Block tiers. Confirm tier covers messaging, social posting, purchases, deletes. Block tier covers payments, factory reset — opt-in only. Stop hotkey halts all mobile automation. AI never guesses credentials. 2 actions/sec rate limit.",
        ["Needs Review", "P1", "mobile", "security"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),

    # ───────────────────────────────────────────────────────────────────
    # SECTION 9 — IPHONE WIDGET & COMPANION APP
    # ───────────────────────────────────────────────────────────────────
    (
        "iOS widget (small / medium / large + lock-screen)",
        "**Source:** PRD §9.1, §9.2\n\n"
        "WidgetKit (SwiftUI). Quick-action buttons, status line, microphone for voice input. Lock-screen circular/rectangular/inline variants. StandBy support. Minimal battery impact.",
        ["Needs Review", "P2", "widget"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Quick actions configuration",
        "**Source:** PRD §9.3\n\n"
        "User-defined shortcuts mapping a single tap to a Conduit command. Configured in companion app or `~/.conduit/widgets/quick-actions.yaml`. Each action: name, SF Symbol icon, color, command (workflow / slash / NL instruction / `__voice_input__` / `__open_dashboard__`).",
        ["Needs Review", "P2", "widget"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Widget connection — local network REST API + mDNS pairing",
        "**Source:** PRD §9.4\n\n"
        "Conduit exposes a REST API (default port 9824). Bonjour/mDNS discovery on same Wi-Fi. TLS with pinned cert. QR-pairing on first connect. Tailscale/WireGuard/Cloudflare Tunnel for remote. Graceful 'Disconnected' fallback with command queueing.",
        ["Needs Review", "P2", "widget", "remote"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Live Activities + Dynamic Island for long-running tasks",
        "**Source:** PRD §9.6\n\n"
        "Auto-start when a task is expected to run >30s. Compact / expanded / minimal Dynamic Island states. Tap to expand → recent log + Stop button. Final summary notification on completion.",
        ["Needs Review", "P2", "widget"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Apple Watch complication + minimal app",
        "**Source:** PRD §9.7\n\n"
        "Circular/rectangular/corner complications with status color. Watch app: quick actions, current task with stop button, recent activity, voice input via WatchConnectivity (Watch never talks to Mac directly).",
        ["Needs Review", "P2", "widget"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Siri Shortcuts integration",
        "**Source:** PRD §9.5\n\n"
        "Each quick action exposed as a Siri Shortcut. Works from phone, Watch, HomePod, CarPlay. Composable with iOS Automations.",
        ["Needs Review", "P2", "widget", "voice"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),

    # ───────────────────────────────────────────────────────────────────
    # SECTION 10 — VOICE
    # ───────────────────────────────────────────────────────────────────
    (
        "Local STT via Whisper.cpp (Metal/Core ML optimized)",
        "**Source:** PRD §10.1\n\n"
        "Streaming transcription with words appearing as you speak. Model size selected from machine profile (tiny/base/small/medium/large-v3). Cloud fallback (OpenAI Whisper, Deepgram, AssemblyAI) opt-in. <500ms latency target. Audio never leaves device by default.",
        ["Needs Review", "P1", "voice"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Voice commands — command vs conversation classification",
        "**Source:** PRD §10.2\n\n"
        "Classify transcribed text as command (workflow / status / system / navigation / mobile / widget) vs conversation. >95% classification accuracy target. Unrecognized commands fall through to conversational AI rather than failing.",
        ["Needs Review", "P1", "voice"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Conversational voice mode with barge-in support",
        "**Source:** PRD §10.3\n\n"
        "Speak naturally, get spoken responses. Interrupt while TTS plays to barge-in. Voice and text share session context — switching is seamless. <2s end-to-end latency for local models. Multimodal context (screenshot of screen on demand).",
        ["Needs Review", "P2", "voice"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Local TTS — Piper / Bark / Coqui / MLX",
        "**Source:** PRD §10.4\n\n"
        "Pluggable local TTS pipeline. Cloud fallback (ElevenLabs, OpenAI TTS, Google TTS) opt-in. Multiple voices, adjustable speed/pitch, per-surface voice settings. Streaming TTS — first audio chunk in <500ms. Min 22kHz sample rate.",
        ["Needs Review", "P1", "voice"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Wake word detection (`Hey Conduit`)",
        "**Source:** PRD §10.5\n\n"
        "openWakeWord / Porcupine / custom keyword spotter. <1% CPU continuous. Audio never streamed/stored before wake word. Visible listening indicator. Custom wake word training (3-5 examples). Multiple wake words supported.",
        ["Needs Review", "P2", "voice"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Voice profiles — speaker identification",
        "**Source:** PRD §10.6\n\n"
        "Recognize 1-5 enrolled speakers. Per-speaker preferences, default workflows, permission levels. Guest mode for unrecognized voices (configurable restriction). Embeddings only — no raw audio retained.",
        ["Needs Review", "P2", "voice"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Voice accessibility compliance",
        "**Source:** PRD §10.7\n\n"
        "WCAG 2.1 AA for all voice UI. Adjustable listening sensitivity. No hard time limits. Confirmation modes ('I heard X — proceed?'). Customizable command aliases. VoiceOver compatibility.",
        ["Needs Review", "P1", "voice", "accessibility"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),

    # ───────────────────────────────────────────────────────────────────
    # SECTION 12 — DESIGN SYSTEM
    # ───────────────────────────────────────────────────────────────────
    (
        "Design tokens — color/type/spacing/shadow/radius/motion",
        "**Source:** PRD §12.1\n\n"
        "Platform-agnostic JSON/YAML source compiled to: CSS custom properties (web), SwiftUI Color/Font extensions (Apple), Textual/Rich theme (TUI). Dark mode default + light + high-contrast (WCAG AAA 7:1). Model provider badge colors.",
        ["Needs Review", "P1", "design-system"],
        "Phase 5 — GUI + Spotlight UI",
    ),
    (
        "Component library across surfaces",
        "**Source:** PRD §12.2\n\n"
        "Button, Input, Card, Modal, Toast, Navigation, Status Badge, Model Badge, Agent Activity Indicator, Code Block, Tool Call Block, Progress Bar, Command Palette, Voice Waveform. Consistent prop API: variant, size, state, accessibility labels.",
        ["Needs Review", "P1", "design-system"],
        "Phase 5 — GUI + Spotlight UI",
    ),
    (
        "Iconography — SF Symbols + Conduit-specific custom icons",
        "**Source:** PRD §12.4\n\n"
        "SF Symbols as base (native Apple platforms). Custom icons (monoline, 1.5px stroke) for Conduit-specific concepts: agent thinking, model switch, workflow fork, memory write, hook fired. Export: SVG, SF Symbols catalog, PNG @1x/2x/3x.",
        ["Needs Review", "P2", "design-system"],
        "Phase 5 — GUI + Spotlight UI",
    ),
    (
        "Motion & animation guidelines",
        "**Source:** PRD §12.5\n\n"
        "Functional, fast (150-200ms), interruptible. Standard animations table: panel open, modal appear, toast, agent thinking pulse, tool-call expand, voice waveform, task progress.",
        ["Needs Review", "P2", "design-system"],
        "Phase 5 — GUI + Spotlight UI",
    ),
    (
        "Open-source design package (`@conduit/design`)",
        "**Source:** PRD §12.7\n\n"
        "npm (web), Swift Package (Apple), Python package (TUI). Tokens, generated outputs, component documentation, icon set, Figma library link, usage guidelines. Plugin developers import once and match the host app exactly.",
        ["Needs Review", "P2", "design-system", "plugin"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Figma source of truth",
        "**Source:** PRD §12.8\n\n"
        "Maintained Figma file: foundations / components / patterns / surfaces / prototypes. Token sync via Tokens Studio. Linked from repo README and design package docs.",
        ["Needs Review", "P2", "design-system", "design-task"],
        "Phase 5 — GUI + Spotlight UI",
    ),
    (
        "AI mockup generation from natural language",
        "**Source:** PRD §12.9\n\n"
        "Description → rendered mockup using Conduit Design tokens and components. Outputs: PNG / SVG / interactive HTML / React or SwiftUI scaffold. Conversational iteration. Mockup-from-diff (preview a code change visually). Plugin API: `design.mockup(description, options)`.",
        ["Needs Review", "P2", "design-system"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "SVG illustration generation (LLM as image generator)",
        "**Source:** PRD §12.10\n\n"
        "Built-in image generation that requires no image model — the LLM writes SVG XML. Style presets (flat / isometric / line art / duotone / gradient / hand-drawn / blueprint). Diagram types (architecture / flowchart / sequence / ER / network / state machine / Gantt). Animation via CSS keyframes or SMIL. Exports: SVG / PNG / PDF / Lottie / ICO / React / SwiftUI. Plugin APIs: `design.svg.generate / animate / export`, `design.diagram`, `design.icon`. SVGO optimization, accessibility labels, complexity budget.",
        ["Needs Review", "P1", "design-system"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),

    # ───────────────────────────────────────────────────────────────────
    # SECTION 13 — CONDUIT VIDEO
    # ───────────────────────────────────────────────────────────────────
    (
        "AI-powered video editor (natural-language editing)",
        "**Source:** PRD §13.1\n\n"
        "Cut / trim / split, transitions, color correction, captions (auto-Whisper), silence removal, music with auto-duck, highlights, speed ramp, zoom/pan, overlays, intro/outro. Natural-language editing: 'Remove the ums', 'Make a 60s highlight reel'. Non-destructive EDL. Template system.",
        ["Needs Review", "P2", "video"],
        None,
    ),
    (
        "Demo recording — auto-zoom, click highlight, scroll smoothing",
        "**Source:** PRD §13.2\n\n"
        "Up to 4K 60fps. Auto-zoom around cursor on click/type. Cursor smoothing, click highlighting, scroll smoothing, window-focus tracking. Webcam PiP with background blur/removal and auto-framing. Tango-style auto-annotated step walkthroughs. AI-narrated voiceover (silent-recording mode).",
        ["Needs Review", "P2", "video"],
        None,
    ),
    (
        "Video export & platform presets",
        "**Source:** PRD §13.3\n\n"
        "Formats: MP4 (H.264/H.265), WebM, MOV (ProRes), GIF, APNG, WebP. Resolutions 4K → square → vertical. Platform presets: Twitter, YouTube, Slack, LinkedIn, internal docs. Auto-thumbnail, chapter markers, SRT/VTT, companion blog post.",
        ["Needs Review", "P2", "video"],
        None,
    ),
    (
        "Mobile demo recording (with device frames + touch viz)",
        "**Source:** PRD §13.4\n\n"
        "Captures phone screen at native res. Auto-zoom/click highlights apply. Touch visualization overlay. Optional iPhone/Pixel/Samsung device frames. Gesture annotation in walkthroughs. Simultaneous desktop+phone split or PiP recording.",
        ["Needs Review", "P2", "video", "mobile"],
        None,
    ),
    (
        "Video plugin API",
        "**Source:** PRD §13.5\n\n"
        "`video.record.start/stop`, `video.screenshot`, `video.edit`, `video.export`, `video.caption`, `video.narrate`, `video.annotate`, `video.highlight`. Use cases: QA plugin records test runs with annotated bug reports; documentation plugin auto-records how-to guides; social plugin generates platform clips.",
        ["Needs Review", "P2", "video", "plugin"],
        None,
    ),
    (
        "Video import from URLs (YouTube / Twitch / Vimeo / Twitter)",
        "**Source:** PRD §13.6\n\n"
        "Download at chosen quality. Pull existing captions; Whisper fallback if none. Metadata + thumbnail extraction. Chapter import. Imported videos are first-class in the editor. Integrates with the LLM Wiki for transcript filing.",
        ["Needs Review", "P2", "video"],
        None,
    ),

    # ───────────────────────────────────────────────────────────────────
    # SECTION 14 — USAGE TRACKING & ANALYTICS
    # ───────────────────────────────────────────────────────────────────
    (
        "Per-model usage metrics + JSONL session logs",
        "**Source:** PRD §14.1, §14.7\n\n"
        "Log every request: model, provider, tokens_in/out, ttft_ms, total_ms, tokens/sec, status, error_type, feature, plugin, session_id, timestamp. Stored at `~/.conduit/logs/usage/YYYY-MM-DD.jsonl`. Daily rotation, 7-day compression, 90-day retention default.",
        ["Needs Review", "P1", "analytics"],
        "Phase 2 — Memory + Hooks + Evals",
    ),
    (
        "Cost tracking — API + local-model estimation",
        "**Source:** PRD §14.2\n\n"
        "Per-request dollar cost from local pricing tables (Anthropic, OpenAI, Google, Mistral). Local-model cost estimated from machine profile wattage × inference time × user electricity rate. Side-by-side cost comparison.",
        ["Needs Review", "P1", "analytics"],
        "Phase 2 — Memory + Hooks + Evals",
    ),
    (
        "Usage dashboard (GUI + TUI + exportable HTML)",
        "**Source:** PRD §14.3\n\n"
        "Panels: cost overview, cost by model/feature, request volume, latency percentiles, error rate, token usage, model comparison table, plugin usage, budget status. Time-range selector, multi-dimensional filters, drill-down to request log. TUI version via `/usage`.",
        ["Needs Review", "P1", "analytics", "gui"],
        "Phase 5 — GUI + Spotlight UI",
    ),
    (
        "Budgets & alerts (per model / provider / feature / plugin / overall)",
        "**Source:** PRD §14.4\n\n"
        "Configurable spending limits. Warning at 75%, critical at 90%, hard stop at 100% (refuses to over-budget; falls back to local). Alerts via macOS notifications, status bar, widget, Live Activity. Projected-overshoot dates and optimization suggestions.",
        ["Needs Review", "P1", "analytics"],
        "Phase 2 — Memory + Hooks + Evals",
    ),
    (
        "Performance comparison view",
        "**Source:** PRD §14.5\n\n"
        "Side-by-side model metrics: TTFT, total latency, tokens/sec, error rate, cost per 1K, fallback frequency. Quality signals: edit rate, retry rate, completion rate (clearly labeled as proxies). Latency-vs-cost scatter plot.",
        ["Needs Review", "P2", "analytics"],
        "Phase 5 — GUI + Spotlight UI",
    ),
    (
        "Plugin usage tracking",
        "**Source:** PRD §14.6\n\n"
        "Per-plugin requests, tokens, cost, model breakdown, error rate. Anomaly detection vs rolling average. `usage.query(plugin_id, time_range)` API for plugin authors.",
        ["Needs Review", "P2", "analytics", "plugin"],
        None,
    ),
    (
        "Privacy controls — purge / export / aggregate-only",
        "**Source:** PRD §14.8\n\n"
        "All tracking local. No telemetry. Prompt content never logged by default. `/usage purge --before / --model` commands. Export raw data as CSV/JSON, dashboard as self-contained HTML, aggregate-only reports for expense reporting.",
        ["Needs Review", "P1", "analytics", "security"],
        "Phase 2 — Memory + Hooks + Evals",
    ),
    (
        "Historical trends & automated insights",
        "**Source:** PRD §14.9\n\n"
        "Long-term views: model migration, cost trajectory with linear projection, feature adoption, efficiency gains, reliability trend. Threshold-based heuristics for insights — no model calls, no privacy impact.",
        ["Needs Review", "P2", "analytics"],
        None,
    ),

    # ───────────────────────────────────────────────────────────────────
    # SECTION 15 — SANDBOXED EXECUTION
    # ───────────────────────────────────────────────────────────────────
    (
        "Sandbox runtime selection (Apple Virtualization / OCI containers)",
        "**Source:** PRD §15.9\n\n"
        "Recommended: Apple Virtualization.framework first (no Docker dep), with OCI container support as alternative. Cold start <5s, warm start <2s, <256MB runtime overhead. Rosetta 2 for x86 Linux binaries on Apple Silicon. virtio-fs for host sharing.",
        ["Needs Review", "P0", "sandbox", "infra"],
        "Phase 4 — Computer Use",
    ),
    (
        "Sandbox architecture — full Linux userspace, isolated FS/network",
        "**Source:** PRD §15.1\n\n"
        "Ubuntu-based filesystem, standard shell, pre-installed runtimes. Prevents host FS/network/process access, no privilege escalation. <2s warm startup. Image pre-cached.",
        ["Needs Review", "P0", "sandbox", "security"],
        "Phase 4 — Computer Use",
    ),
    (
        "Filesystem isolation & mount modes (read-only / read-write / copy-in / copy-out)",
        "**Source:** PRD §15.2\n\n"
        "Default: copy-in (agent works on a copy, user reviews diffs, approved changes synced back). Dynamic mount requests surfaced for user approval. Sensitive paths blocked without explicit override (`~/.ssh`, `~/.aws`, `~/Library/Keychains`, etc).",
        ["Needs Review", "P0", "sandbox", "security"],
        "Phase 4 — Computer Use",
    ),
    (
        "Network sandboxing — allowlist + per-request approval",
        "**Source:** PRD §15.3\n\n"
        "Restricted (default) / per-request / open / offline modes. Default allowlist: pypi, npm, GitHub, model providers. Inbound blocked by default; explicit port forwarding. All DNS + traffic logged.",
        ["Needs Review", "P0", "sandbox", "security"],
        "Phase 4 — Computer Use",
    ),
    (
        "Permission model — categories, scopes, audit trail",
        "**Source:** PRD §15.4\n\n"
        "Categories: filesystem (host/sandbox), network, shell, computer use, mobile, destructive ops, external comms, credentials. Scopes: per-task / per-session / persistent. Every grant + denial logged.",
        ["Needs Review", "P0", "sandbox", "security"],
        "Phase 4 — Computer Use",
    ),
    (
        "Persistent workspace per sandbox",
        "**Source:** PRD §15.5\n\n"
        "`~/.conduit/sandboxes/<name>/` with workspace/, home/, cache/, snapshots/, logs/, config.yaml. Workspace persists across sessions; /tmp inside sandbox cleared on session end. Disk quota (default 10GB) with cleanup tooling.",
        ["Needs Review", "P1", "sandbox"],
        "Phase 4 — Computer Use",
    ),
    (
        "Snapshot & rollback (copy-on-write filesystem diffs)",
        "**Source:** PRD §15.6\n\n"
        "Auto-snapshot before risky ops. Manual via `conduit sandbox snapshot`. Workflow-integrated. List / compare / restore / delete. Auto-cleanup after configurable threshold (default 7 days), max 20 per sandbox.",
        ["Needs Review", "P1", "sandbox"],
        "Phase 4 — Computer Use",
    ),
    (
        "Multiple sandboxes — list / create / switch / clone / destroy",
        "**Source:** PRD §15.7\n\n"
        "`conduit sandbox list | create | switch | clone | destroy`. Per-sandbox disk/memory/CPU limits. Active sandbox shown in TUI/GUI status bar. Each has its own session history, permissions, workspace.",
        ["Needs Review", "P1", "sandbox"],
        "Phase 4 — Computer Use",
    ),
    (
        "Pre-installed package management & custom base images",
        "**Source:** PRD §15.8\n\n"
        "Pre-installed: Python 3.12+, Node 20+, Go 1.22+, Rust, pip/npm/yarn/pnpm/cargo, git/curl/jq/ripgrep/fd, vim/nano, SQLite. Allowlisted registries don't require approval. Custom base images shareable with teammates.",
        ["Needs Review", "P1", "sandbox"],
        "Phase 4 — Computer Use",
    ),

    # ───────────────────────────────────────────────────────────────────
    # SECTION 16 — TOKEN EFFICIENCY
    # ───────────────────────────────────────────────────────────────────
    (
        "Context assembler — first-class component for prompt optimization",
        "**Source:** PRD §16.2, §16.9\n\n"
        "Dedicated module that runs before every model call. Smart context pruning with relevance scoring, sliding window with summarization, file diffing instead of full re-sends, AST-based code extraction (Tree-sitter), per-category token budget allocation. Optimization transparency in session log.",
        ["Needs Review", "P1", "performance", "core"],
        "Phase 7 — Coding Agent Engine",
    ),
    (
        "Caching strategies — prefix / KV / semantic / tool result / response",
        "**Source:** PRD §16.3\n\n"
        "Anthropic-style prompt prefix caching. Local-model KV cache reuse with LRU eviction. Exact-match response cache. Embedding-based semantic cache (FAISS). Tool result cache (file reads, search results, web fetches) with TTL/file-change invalidation. Cache hierarchy from fastest to slowest.",
        ["Needs Review", "P1", "performance"],
        "Phase 7 — Coding Agent Engine",
    ),
    (
        "Cost-aware cascading inference",
        "**Source:** PRD §16.4\n\n"
        "Trivial / simple / moderate / complex classification by free heuristics (length, code presence, file refs, keywords, turn count). Cascading inference: try cheaper tier first, escalate on confidence/quality threshold miss. Batch routing for parallel sub-tasks. Budget-proximity-aware routing degradation.",
        ["Needs Review", "P1", "performance", "router"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Speculative decoding + batched inference",
        "**Source:** PRD §16.5\n\n"
        "Draft-verifier model pairs (e.g., Llama 3 8B Q4 + Llama 3 70B Q5) with configurable acceptance threshold. Continuous batching for parallel sub-agents. Quantization-aware prompt design. Token streaming for perceived latency reduction.",
        ["Needs Review", "P2", "performance"],
        None,
    ),
    (
        "Agent-level efficiency — batched tool calls, plan-then-execute, diff edits",
        "**Source:** PRD §16.6\n\n"
        "Batch tool variants (`read_files([...])`). Early termination when structured output is complete. Plan-first for multi-step tasks. Diff-based file updates instead of full rewrites. Aggressive conversation compaction for long automation runs.",
        ["Needs Review", "P2", "performance", "coding-agent"],
        "Phase 7 — Coding Agent Engine",
    ),
    (
        "Efficiency monitoring + auto-tuning feedback loop",
        "**Source:** PRD §16.7\n\n"
        "Metrics: tokens/successful task, cache hit rate, context utilization, waste ratio, routing efficiency, retry/escalation rate. Per-task budgets with auto-optimization on approach. Weekly efficiency reports. Auto-tune context budget / relevance threshold / cache TTL / routing thresholds; transparent and override-able.",
        ["Needs Review", "P2", "performance", "analytics"],
        None,
    ),

    # ───────────────────────────────────────────────────────────────────
    # SECTION 17 — DOCUMENTATION
    # ───────────────────────────────────────────────────────────────────
    (
        "Documentation site (VitePress / Docusaurus / Astro)",
        "**Source:** PRD §17.2\n\n"
        "Sections: Getting Started, Guides, Configuration Reference, API Reference (auto-generated), Plugin Development Guide, Architecture, FAQ. Docs live in repo, reviewed in PRs, broken-link CI.",
        ["Needs Review", "P1", "docs"],
        None,
    ),
    (
        "In-app documentation, tooltips, `/help` command",
        "**Source:** PRD §17.3\n\n"
        "First-launch guided tour. Tooltips on settings/status/UI elements. `/help <topic>` in TUI. Contextual 'Learn more' links on friction points. Searchable from command palette.",
        ["Needs Review", "P2", "docs"],
        None,
    ),
    (
        "CONTRIBUTING.md and changelog automation",
        "**Source:** PRD §17.4, §17.5\n\n"
        "One-command dev setup, code style/conventions, PR process, architecture overview, issue triage labels, code of conduct. Automated changelog from conventional commits + PR labels in Keep-a-Changelog format. Surfaced in-app on update.",
        ["Needs Review", "P1", "docs", "infra"],
        None,
    ),

    # ───────────────────────────────────────────────────────────────────
    # SECTION 6.25 — CODEX PARITY
    # ───────────────────────────────────────────────────────────────────
    (
        "MCP integration — stdio + streaming HTTP transports",
        "**Source:** PRD §6.25.5\n\n"
        "STDIO and streaming-HTTP MCP server support. User-global + project-scoped configs. `conduit mcp` CLI. Auto-discovery and exposure of MCP tools alongside built-ins. Conduit-as-MCP-server. `@ToolName` inline mention syntax. Side-effecting tool approval. Server allowlist.",
        ["Needs Review", "P0", "core"],
        "Phase 1 — Core Engine + TUI",
    ),
    (
        "Web search tool (cached / live / disabled)",
        "**Source:** PRD §6.25.7\n\n"
        "First-party built-in. Cached (default), live, disabled modes. GET/HEAD/OPTIONS only by default; POST/PUT/DELETE blocked.",
        ["Needs Review", "P1", "core"],
        "Phase 6 — Skills + Multi-Agent + Voice",
    ),
    (
        "Approval network — reviewer agent for side-effecting actions",
        "**Source:** PRD §6.25.8\n\n"
        "A reviewer agent evaluates actions requiring approval before execution. Configurable `automatic_review_policy`. Detects security violations (data exfil, credential probing, persistent weakening) and destructive ops. Typed approval prompts per action category.",
        ["Needs Review", "P2", "security", "core"],
        "Phase 7 — Coding Agent Engine",
    ),
    (
        "IDE integration — VS Code + JetBrains + Cursor/Windsurf",
        "**Source:** PRD §6.25.14\n\n"
        "Conduit in IDE sidebar. Custom keyboard shortcuts per IDE. Editor selection / file context sharing. Multi-platform (macOS/Windows/Linux for the extension itself).",
        ["Needs Review", "P2", "ide"],
        None,
    ),
    (
        "Codex-aligned slash commands",
        "**Source:** PRD §6.25.4, §6.24.25\n\n"
        "`/help`, `/review`, `/fork`, `/resume`, `/compact`, `/diff`, `/approvals`, `/agent` plus the coding-engine set: `/context`, `/token-budget`, `/mcp`, `/search`, `/remote`, `/account`, `/config`, `/plan`, `/tasks`, `/task-next`, `/prompt`, `/hooks`, `/trust`, `/permissions`, `/agents`, `/memory`, `/status`, `/clear`.",
        ["Needs Review", "P1", "core", "tui", "coding-agent"],
        "Phase 7 — Coding Agent Engine",
    ),
    (
        "`conduit exec` — non-interactive scripted execution",
        "**Source:** PRD §6.25.13, §6.25.16\n\n"
        "Sandboxed scripted runs for CI/CD. GitHub Actions integration. Pre-commit hooks, automated changelog, GitHub issue management via agent-driven scripts.",
        ["Needs Review", "P1", "infra", "coding-agent"],
        "Phase 7 — Coding Agent Engine",
    ),
    (
        "Admin / managed configuration (`requirements.toml`)",
        "**Source:** PRD §6.25.21\n\n"
        "Admin-enforced constraint definitions. Managed defaults per session. MCP server allowlist. Managed hooks. Audit logging. Fine-grained per-user permissions. Mostly relevant if/when Conduit is used in a team context.",
        ["Needs Review", "P2", "infra", "security"],
        None,
    ),
]


def run(cmd, **kwargs):
    return subprocess.run(cmd, capture_output=True, text=True, **kwargs)


def main():
    created = 0
    failed = 0
    for i, (title, body, labels, milestone) in enumerate(ISSUES, 1):
        cmd = ["gh", "issue", "create", "--title", title, "--body", body]
        for lbl in labels:
            cmd += ["--label", lbl]
        if milestone:
            cmd += ["--milestone", milestone]
        result = run(cmd)
        if result.returncode == 0:
            url = result.stdout.strip()
            print(f"[{i:3d}/{len(ISSUES)}] ✓ {title[:60]:60s} → {url}")
            created += 1
        else:
            print(f"[{i:3d}/{len(ISSUES)}] ✗ {title[:60]:60s}")
            print(f"           STDERR: {result.stderr.strip()[:200]}")
            failed += 1
        time.sleep(0.3)  # be nice to GH API

    print(f"\nCreated: {created} / {len(ISSUES)}  Failed: {failed}")
    return 0 if failed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
