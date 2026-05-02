# Conduit — Issue Backlog (Dependency-Ordered)

> Last updated: 2026-05-02  
> Open issues only. Closed issues are tracked in GitHub. Work top-to-bottom within each wave; parallel tracks within a wave can run simultaneously.

---

## Wave 2 — Finish Line 🟠
> 6 issues remaining. Close these before starting Wave 3 work that depends on them.

### P0 — Must close first (unblocks Wave 3 workflow engine)
| # | Issue |
|---|-------|
| [#26](../../issues/26) | Hook system — shell subprocess + JSON wire protocol |

### P1 — Run in parallel
| # | Issue |
|---|-------|
| [#11](../../issues/11) | Structured reply tag parser |
| [#12](../../issues/12) | Session JSONL tree (fork / replay / inspect) |
| [#13](../../issues/13) | Configurable keybindings system |
| [#31](../../issues/31) | Memory inspector in TUI |

### P2
| # | Issue |
|---|-------|
| [#25](../../issues/25) | SQLiteProvider — structured memory with FTS |

---

## Wave 3 — Platform 🟡
> 35 issues. Four parallel tracks once Wave 2 is closed.

### Track A — Workflow Engine
> Depends on #26 (hook system). Start immediately after it closes.

| # | Priority | Issue |
|---|----------|-------|
| [#32](../../issues/32) | P0 | Workflow engine — schema, parser, validator |
| [#33](../../issues/33) | P0 | Workflow checkpointing & resume |
| [#34](../../issues/34) | P1 | Workflow conditional branching |
| [#35](../../issues/35) | P1 | Workflow scheduler (cron) |
| [#36](../../issues/36) | P1 | Workflow status display in TUI |

### Track B — GUI
> #43 is a zero-code decision that unblocks all GUI work. Make the call first.

| # | Priority | Issue |
|---|----------|-------|
| [#43](../../issues/43) | P1 | **Decide GUI stack (SwiftUI vs Tauri vs Electron)** ← decision |
| [#44](../../issues/44) | P1 | Multi-fidelity GUI mockups |
| [#45](../../issues/45) | P1 | GUI: three-column layout (Sidebar / Main / Agent Panel) |
| [#46](../../issues/46) | P1 | GUI: computer-use screenshot stream view |
| [#47](../../issues/47) | P1 | GUI: Canvas panel (WKWebView for `canvas:html`) |
| [#48](../../issues/48) | P1 | GUI: workflow DAG visualization |
| [#49](../../issues/49) | P1 | GUI: session tree browser (fork / replay) |
| [#50](../../issues/50) | P1 | Conduit Spotlight UI — Raycast-style overlay |

### Track C — Skills & Coding Agent
> Independent of workflow and GUI tracks. Can run from day one of Wave 3.

| # | Priority | Issue |
|---|----------|-------|
| [#51](../../issues/51) | P1 | Skills registry — schema, storage, hierarchy |
| [#52](../../issues/52) | P1 | Universal Skill Adapter — import from any source |
| [#57](../../issues/57) | P1 | Multimodal input — images, PDFs, screenshots |
| [#59](../../issues/59) | P1 | `conduit code` REPL — coding agent entry point |
| [#60](../../issues/60) | P1 | Coding-agent context management |
| [#61](../../issues/61) | P1 | Core coding tool set with permission tiers |
| [#62](../../issues/62) | P1 | Plugin runtime — manifest, lifecycle hooks, virtual tools |
| [#63](../../issues/63) | P1 | Nested agent delegation |
| [#64](../../issues/64) | P1 | Custom agent profiles |
| [#65](../../issues/65) | P1 | Fine-grained budget control for coding sessions |
| [#66](../../issues/66) | P1 | LSP code intelligence runtime |
| [#69](../../issues/69) | P1 | Code diff visualization (TUI + GUI side-by-side) |
| [#70](../../issues/70) | P1 | Git worktree-aware sessions |
| [#67](../../issues/67) | P2 | Background & daemon coding sessions |
| [#68](../../issues/68) | P2 | Coding-agent GUI tabs |
| [#71](../../issues/71) | P2 | Mini IDE — syntax highlighting + AI assists |

### Track D — Advanced Sandbox & Performance
> Sandbox depends on Wave 1 sandbox foundation (closed). Performance is independent.

| # | Priority | Issue |
|---|----------|-------|
| [#127](../../issues/127) | P1 | Persistent workspace per sandbox |
| [#128](../../issues/128) | P1 | Snapshot & rollback (copy-on-write filesystem diffs) |
| [#129](../../issues/129) | P1 | Multiple sandboxes — list / create / switch / clone / destroy |
| [#130](../../issues/130) | P1 | Pre-installed package management & custom base images |
| [#131](../../issues/131) | P1 | Context assembler — first-class component for prompt optimization |
| [#132](../../issues/132) | P1 | Caching strategies — prefix / KV / semantic / tool result / response |
| [#141](../../issues/141) | P1 | Web search tool (cached / live / disabled) |

---

## Wave 4 — Extensions 🟢
> 49 issues. Unlocks after Wave 3. Five parallel tracks.

### Track A — Voice
| # | Priority | Issue |
|---|----------|-------|
| [#93](../../issues/93) | P1 | Local STT via Whisper.cpp (Metal/Core ML) |
| [#94](../../issues/94) | P1 | Voice commands — command vs conversation classification |
| [#96](../../issues/96) | P1 | Local TTS — Piper / Bark / Coqui / MLX |
| [#99](../../issues/99) | P1 | Voice accessibility compliance |
| [#95](../../issues/95) | P2 | Conversational voice mode with barge-in support |
| [#97](../../issues/97) | P2 | Wake word detection (`Hey Conduit`) |
| [#98](../../issues/98) | P2 | Voice profiles — speaker identification |

### Track B — Mobile
| # | Priority | Issue |
|---|----------|-------|
| [#83](../../issues/83) | P1 | Mobile screen mirroring + interaction primitives |
| [#84](../../issues/84) | P1 | AI-driven mobile automation loop |
| [#85](../../issues/85) | P1 | Mobile connection methods (USB / wireless, iOS / Android) |
| [#86](../../issues/86) | P1 | Mobile safety & confirmation tiers |

### Track C — Widgets & Integrations
| # | Priority | Issue |
|---|----------|-------|
| [#89](../../issues/89) | P2 | Widget connection — local network REST API + mDNS pairing |
| [#87](../../issues/87) | P2 | iOS widget (small / medium / large + lock-screen) |
| [#88](../../issues/88) | P2 | Quick actions configuration |
| [#90](../../issues/90) | P2 | Live Activities + Dynamic Island |
| [#91](../../issues/91) | P2 | Apple Watch complication + minimal app |
| [#92](../../issues/92) | P2 | Siri Shortcuts integration |

### Track D — Advanced Features
| # | Priority | Issue |
|---|----------|-------|
| [#76](../../issues/76) | P1 | Remote access & pairing (`conduit serve`) |
| [#81](../../issues/81) | P1 | Model management UI (browse, download, swap, remove) |
| [#93](../../issues/93) | P1 | Local STT via Whisper.cpp |
| [#107](../../issues/107) | P1 | SVG illustration generation (LLM as image generator) |
| [#116](../../issues/116) | P1 | Usage dashboard (GUI + TUI + exportable HTML) |
| [#133](../../issues/133) | P1 | Cost-aware cascading inference |
| [#144](../../issues/144) | P1 | Codex-aligned slash commands |
| [#145](../../issues/145) | P1 | `conduit exec` — non-interactive scripted execution |
| [#54](../../issues/54) | P1 | Subagent spawning from workflow steps |
| [#24](../../issues/24) | P2 | LanceDBProvider — vector-search memory |
| [#53](../../issues/53) | P2 | Skill auto-generation from novel tasks |
| [#55](../../issues/55) | P2 | Smart Consensus Mode (multi-model deliberation) |
| [#56](../../issues/56) | P2 | Passive observational learning (consent-gated) |
| [#58](../../issues/58) | P2 | Command palette + $skills inline composer |
| [#118](../../issues/118) | P2 | Performance comparison view |
| [#133](../../issues/133) | P1 | Cost-aware cascading inference |
| [#135](../../issues/135) | P2 | Agent-level efficiency — batched tool calls, plan-then-execute |
| [#142](../../issues/142) | P2 | Approval network — reviewer agent for side-effecting actions |
| [#143](../../issues/143) | P2 | IDE integration — VS Code + JetBrains + Cursor/Windsurf |

### Track E — Collaboration
| # | Priority | Issue |
|---|----------|-------|
| [#72](../../issues/72) | P2 | Read-only shared sessions via pairing token |
| [#73](../../issues/73) | P2 | Active co-sessions (multi-author messages) |
| [#74](../../issues/74) | P2 | Discord channel relay |
| [#75](../../issues/75) | P2 | Slack channel relay |

### Track F — Design System Advanced
| # | Priority | Issue |
|---|----------|-------|
| [#102](../../issues/102) | P2 | Iconography — SF Symbols + Conduit-specific icons |
| [#103](../../issues/103) | P2 | Motion & animation guidelines |
| [#104](../../issues/104) | P2 | Open-source design package (`@conduit/design`) |
| [#105](../../issues/105) | P2 | Figma source of truth |
| [#106](../../issues/106) | P2 | AI mockup generation from natural language |

### Track G — Video
| # | Priority | Issue |
|---|----------|-------|
| [#109](../../issues/109) | P2 | Demo recording — auto-zoom, click highlight, scroll smoothing |
| [#108](../../issues/108) | P2 | AI-powered video editor (natural-language editing) |
| [#110](../../issues/110) | P2 | Video export & platform presets |
| [#111](../../issues/111) | P2 | Mobile demo recording (with device frames + touch viz) |
| [#112](../../issues/112) | P2 | Video plugin API |
| [#113](../../issues/113) | P2 | Video import from URLs (YouTube / Twitch / Vimeo / Twitter) |

---

## Wave 5 — Polish 🔵
> 8 issues. Requires a complete product. Start only once all other waves are in good shape.

| # | Priority | Issue |
|---|----------|-------|
| [#137](../../issues/137) | P1 | Documentation site (VitePress / Docusaurus / Astro) |
| [#139](../../issues/139) | P1 | CONTRIBUTING.md and changelog automation |
| [#138](../../issues/138) | P2 | In-app documentation, tooltips, `/help` command |
| [#119](../../issues/119) | P2 | Plugin usage tracking |
| [#121](../../issues/121) | P2 | Historical trends & automated insights |
| [#134](../../issues/134) | P2 | Speculative decoding + batched inference |
| [#136](../../issues/136) | P2 | Efficiency monitoring + auto-tuning feedback loop |
| [#146](../../issues/146) | P2 | Admin / managed configuration (`requirements.toml`) |

---

## Critical Path

```
#26 (hooks) → #32 (workflow engine) → #33 (checkpointing) → #37 (computer use)
                                                            → #59 (conduit code)
                                                              → #93 (voice STT)
                                                                → #137 (docs)
```

## Parallel Tracks Summary (Wave 3)

```
Wave 2 closes
     │
     ├── Track A: #32 → #33 → #34, #35, #36
     ├── Track B: #43 (decision) → #44 → #45-50
     ├── Track C: #51 → #52   and   #59 → #60-71
     └── Track D: #127-130 (sandbox)   #131-132, #141 (perf)
```
