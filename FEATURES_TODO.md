# Conduit GUI — Feature Status

Status of the "Minimalist & Airy" GUI (`spike/gui`, Tauri/React) against the live
Conduit core. Design language: [`docs/design.md`](docs/design.md).

> **Backend is real.** `internal/server/` implements `/api/info`, `/api/memory`
> (GET + POST), `/api/sessions`, `/api/projects`, and the `/api/agent` WebSocket.
> Run it with **`conduit serve`** (`127.0.0.1:9876`, which `api.ts` targets). Data
> comes from `~/.conduit/SOUL.md`/`USER.md` and `~/.conduit/coding-sessions/*.jsonl`
> (projects = journals grouped by git repo).

Every item from the original gap list is now **done** or **deferred with a concrete
blocker** (below). There are no remaining open/actionable items that can be built
without external resources or a product decision.

---

## ✅ Done

**Design system & theming**
- "Minimalist & Airy" theme in `design/tokens.yaml` (source of truth) → web/Apple/TUI via `make tokens`.
- In-app **theme switcher** (light/dark/hc) — sidebar control, Settings page, and Spotlight command; persisted (`src/useTheme.ts`).

**Live-wired to the backend**
- **soul.md side panel** — read via `getMemory()`; **edit** via new `POST /api/memory` (atomic write + Go tests in `internal/server/views.go`).
- **Soul page** (`SoulPage.tsx`) — full-page SOUL.md/USER.md viewer + editor.
- **Settings page** (`SettingsPage.tsx`) — theme selector + live About (version/provider/model from `getInfo()`).
- **Sidebar** — live `getProjects()` rail (repos + nested sessions); real profile line (`provider · model` from `getInfo()`); Update pill removed (no update source).
- **Projects list** (`ProjectsList.tsx`) — live `getProjects()`; favorites (localStorage); client filter/sort; **local project create** merged in.
- **Project dashboard** (`ProjectDashboard.tsx`) — live summary + real session list by `projectId`; honest empty states.
- **Recent Activity panel** (`TeamActivity.tsx`) — live recent sessions via `getSessions()` (was a fake team feed).
- **Spotlight** — real commands dispatched to navigation/theme + live recent-session results.
- **New Project form** — functional: controlled fields, persists a **local** project (`src/localProjects.ts`) that appears (labeled "Local") until it has server sessions.

**Frontend features**
- **Global search** (top bar) → opens Spotlight with the query.
- **Top-bar back/forward** — view history navigation.
- **Sidebar nav** unified: New chat / Projects / Plugins / Automations / Soul / Settings, with section `+` actions (new project / new chat).
- **Plugins / Automations pages** — honest "coming soon" empty states (no fabricated data).
- **Welcome composer pills** — functional model/sandbox dropdowns (local state).
- **Chat — live streaming (verified)** — `connectAgent` ↔ `/api/agent` streams real
  model output end-to-end (tested with the Codex provider: prompt → `session` →
  `text_delta` → `end_turn`, assistant reply rendered in the GUI). `?demo=chat` is
  a QA seed for offline screenshots.
- Removed orphaned `SessionList.tsx` / `MemoryView.tsx`.

---

## ⏸ Deferred — blocked on external resources or a product decision

These can't be completed in this environment/scope without the noted prerequisite.
They're intentionally **not** faked.

- **Per-run model / sandbox / context selection** — the pills hold state but aren't
  sent to a run; the streamer is bound per-connection. *Blocker: provider-layer
  plumbing in `coding.Streamer` + an extended `/api/agent` frame.*
- **Projects as a first-class entity** — real create + infra provisioning, asset
  upload / URL ingestion (Figma/GitHub/Docs) + assets store, agent attachment,
  ACLs (Private/Team/Public), objectives, agent health, pinned assets, and
  project-scoped "Start New Session". *Blocker: product/architecture decision —
  today a project is only a journal grouping; New Project persists locally as an
  interim.*
- **Integration cards** (GitHub/Linear/MCP/Slack connect) — *Blocker: registered
  OAuth apps + secrets; out of scope for a local single-user spike.*
- **User account / billing / team** — *Blocker: no auth system; the app is local
  single-user (Settings says so honestly).*
- **Update-check / apply** (the removed Update pill) — *Blocker: a release/update
  server.*
- **Plugins registry & Automations engine** — pages are honest shells. *Blocker:
  the actual subsystems don't exist yet.*
- **Spotlight server-side ranking** (`internal/gui/spotlight.go` is scaffold) — the
  client-side command + live-session merge is the interim. *Blocker: a ranking
  service.*
- **Chat multimodal** — composer image/file attach + the inline reference-image
  block. *Blocker: multimodal support on `/api/agent` + provider.*
- **Top-bar Split view / Dock** — *Blocker: Tauri multi-window APIs (the spike runs
  frontend-only).*

---

## Reproducing live data for QA

```sh
go build -o /tmp/conduit ./cmd/conduit && /tmp/conduit serve   # :9876
cd spike/gui && npm run dev                                    # :1420
# projects: drop *.jsonl with a RepositoryRoot into ~/.conduit/coding-sessions/
# memory:   edit via the soul.md pencil, or curl -X POST :9876/api/memory -d '{"soul":"…","user":"…"}'
# screenshots: node spike/gui/capture.mjs   (?demo=chat|spotlight|projects|new-project|workspace|soul|settings|plugins)
```
