// Backend client. Speaks the protocol described in the GUI brief: a single
// websocket at /api/agent for streaming, plus a few REST endpoints for
// snapshots. Uses native fetch + WebSocket — no third-party clients.

const BASE: string =
  (import.meta.env.VITE_CONDUIT_API as string | undefined) ??
  "http://localhost:9876";

function wsBase(): string {
  return BASE.replace(/^http/, "ws");
}

// ── Protocol types (must match internal/gui/api/protocol.go on the server). ──

export type ServerMessage =
  | { type: "session"; id: string; provider: string; model: string }
  | { type: "text_delta"; text: string }
  | { type: "tool_use"; id: string; name: string; input: string }
  | { type: "tool_result"; id: string; name: string; output: string; isError: boolean }
  | { type: "end_turn" }
  | { type: "error"; message: string };

export type ClientMessage = { type: "prompt"; text: string };

export type Info = {
  provider: string;
  model: string;
  version: string;
  sessionId: string;
};

export type SessionMeta = {
  id: string;
  createdAt: string;
  summary: string;
};

export type Memory = {
  soul: string;
  user: string;
};

// Projects + chats — wire shape that the backend agent is building to.
// Server source of truth: internal/gui/api/projects.go.

export type ChatSummary = {
  id: string;
  title: string;
  createdAt: string; // ISO
  turnCount: number;
};

export type Project = {
  id: string;
  name: string;
  path: string; // home-relative ~/...
  absolutePath: string;
  branch: string; // may be ""
  sessionCount: number;
  lastActivity: string; // ISO
  chats: ChatSummary[];
};

export type ProjectsResponse = {
  projects: Project[];
  orphans: ChatSummary[];
};

// ── REST helpers ─────────────────────────────────────────────────────────

async function getJSON<T>(path: string): Promise<T> {
  const r = await fetch(`${BASE}${path}`);
  if (!r.ok) throw new Error(`${path}: HTTP ${r.status}`);
  return (await r.json()) as T;
}

export const getInfo = (): Promise<Info> => getJSON<Info>("/api/info");

export async function patchSettings(provider: string, model: string): Promise<Info> {
  const r = await fetch(`${BASE}/api/settings`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ provider, model }),
  });
  if (!r.ok) {
    const err = (await r.json().catch(() => ({}))) as { error?: string };
    throw new Error(err.error ?? `HTTP ${r.status}`);
  }
  return (await r.json()) as Info;
}

export const getSessions = (): Promise<SessionMeta[]> =>
  getJSON<SessionMeta[]>("/api/sessions");
export const getMemory = (): Promise<Memory> => getJSON<Memory>("/api/memory");

// saveMemory writes SOUL.md + USER.md back via POST /api/memory and returns the
// persisted memory (the server echoes it). Both fields are always sent so the
// pair stays consistent on disk.
export async function saveMemory(mem: Memory): Promise<Memory> {
  const r = await fetch(`${BASE}/api/memory`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(mem),
  });
  if (!r.ok) throw new Error(`/api/memory: HTTP ${r.status}`);
  return (await r.json()) as Memory;
}
export const getProjects = (): Promise<ProjectsResponse> =>
  getJSON<ProjectsResponse>("/api/projects");

export function baseUrl(): string {
  return BASE;
}

// ── WebSocket helper with reconnection ───────────────────────────────────

export type ConnectionState = "connecting" | "connected" | "disconnected";

export type AgentClient = {
  send: (msg: ClientMessage) => void;
  close: () => void;
};

export type AgentClientHandlers = {
  onMessage: (msg: ServerMessage) => void;
  onState: (state: ConnectionState) => void;
};

export type AgentClientOptions = {
  /** When set, the server will prime the session with the named template's system prompt. */
  templateId?: string;
  /** Absolute path of the selected project. The server reads AGENTS.md and CLAUDE.md from this directory and prepends them to the first turn. */
  projectPath?: string;
};

export function connectAgent(
  handlers: AgentClientHandlers,
  options: AgentClientOptions = {},
): AgentClient {
  let ws: WebSocket | null = null;
  let closed = false;
  let retry = 0;
  let retryTimer: number | null = null;

  function open() {
    handlers.onState("connecting");
    const params = new URLSearchParams();
    if (options.templateId) params.set("template", options.templateId);
    if (options.projectPath) params.set("projectPath", options.projectPath);
    const qs = params.toString();
    const url = qs ? `${wsBase()}/api/agent?${qs}` : `${wsBase()}/api/agent`;
    ws = new WebSocket(url);

    ws.addEventListener("open", () => {
      retry = 0;
      handlers.onState("connected");
    });

    ws.addEventListener("message", (e: MessageEvent<string>) => {
      try {
        const data = JSON.parse(e.data) as ServerMessage;
        handlers.onMessage(data);
      } catch {
        // Drop frames the server promised but didn't deliver. Better to keep
        // the stream alive than tear down on a single bad frame.
      }
    });

    ws.addEventListener("close", () => {
      if (closed) return;
      handlers.onState("disconnected");
      // Capped exponential backoff: 0.5s, 1s, 2s, 4s, … 8s.
      const delay = Math.min(8000, 500 * 2 ** retry);
      retry++;
      retryTimer = window.setTimeout(open, delay);
    });

    ws.addEventListener("error", () => {
      // The close handler runs after this, so reconnect logic lives there.
    });
  }

  open();

  return {
    send(msg) {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify(msg));
      }
    },
    close() {
      closed = true;
      if (retryTimer !== null) window.clearTimeout(retryTimer);
      if (ws) ws.close();
    },
  };
}
