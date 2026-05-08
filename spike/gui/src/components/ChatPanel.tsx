import { useEffect, useRef, useState } from "react";
import { ChevronDown, ChevronRight, SendHorizontal } from "lucide-react";
import {
  connectAgent,
  getInfo,
  type ConnectionState,
  type Info,
  type ServerMessage,
} from "../api";

// A single rendered chat block. Tool blocks are derived from tool_use frames
// and updated in place when the matching tool_result arrives.
type ToolBlock = {
  kind: "tool";
  id: string;
  name: string;
  input: string;
  output: string | null;
  isError: boolean;
};

type AssistantBlock = {
  kind: "assistant";
  text: string;
};

type UserBlock = {
  kind: "user";
  text: string;
};

type Block = UserBlock | AssistantBlock | ToolBlock;

// A turn groups a user prompt with the resulting assistant + tool blocks so
// the UI can render them as a single card.
type Turn = {
  id: number;
  blocks: Block[];
  done: boolean;
};

type ProviderTone = "claude" | "openai" | "litellm" | "openrouter" | "local";

// Map raw provider strings (as emitted by the core session frame) onto the
// per-provider model-badge tokens published in design/dist/web/tokens.css.
function providerTone(provider: string | undefined): ProviderTone {
  switch ((provider ?? "").toLowerCase()) {
    case "anthropic":
    case "claude":
      return "claude";
    case "openai":
    case "codex":
      return "openai";
    case "litellm":
      return "litellm";
    case "openrouter":
      return "openrouter";
    case "auto":
    case "":
      return "local";
    default:
      return "local";
  }
}

export function ChatPanel({
  initialPrompt = null,
}: {
  initialPrompt?: string | null;
}) {
  const [info, setInfo] = useState<Info | null>(null);
  const [state, setState] = useState<ConnectionState>("connecting");
  const [turns, setTurns] = useState<Turn[]>([]);
  const [draft, setDraft] = useState("");
  const clientRef = useRef<ReturnType<typeof connectAgent> | null>(null);
  const streamRef = useRef<HTMLDivElement | null>(null);
  const turnIdRef = useRef(0);
  // Track whether the welcome composer's prompt was already auto-sent so
  // reconnects after a network blip don't replay it.
  const initialSentRef = useRef(false);

  useEffect(() => {
    getInfo().then(setInfo).catch(() => {
      // Info is non-essential for the chat surface; the websocket session
      // frame carries the same provider/model.
    });
  }, []);

  useEffect(() => {
    const client = connectAgent({
      onState: setState,
      onMessage: (msg) => applyMessage(msg, setTurns, setInfo),
    });
    clientRef.current = client;
    return () => client.close();
  }, []);

  // Auto-send the welcome-screen prompt once the WS is connected. Renders
  // the user turn locally so the chat surface mirrors a real send.
  useEffect(() => {
    if (state !== "connected" || initialSentRef.current) return;
    const text = initialPrompt?.trim();
    if (!text) return;
    initialSentRef.current = true;
    const id = ++turnIdRef.current;
    setTurns((prev) => [
      ...prev,
      { id, blocks: [{ kind: "user", text }], done: false },
    ]);
    clientRef.current?.send({ type: "prompt", text });
  }, [state, initialPrompt]);

  // Pin the stream to the bottom whenever it grows.
  useEffect(() => {
    const el = streamRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [turns]);

  function send() {
    const text = draft.trim();
    if (!text || state !== "connected") return;
    const id = ++turnIdRef.current;
    setTurns((prev) => [
      ...prev,
      { id, blocks: [{ kind: "user", text }], done: false },
    ]);
    clientRef.current?.send({ type: "prompt", text });
    setDraft("");
  }

  function onKey(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      send();
    }
  }

  const tone = providerTone(info?.provider);
  const sendDisabled = state !== "connected" || draft.trim().length === 0;

  return (
    <section className="agent-panel" aria-label="Agent">
      <div className="agent-header">
        <span className="agent-header-title">Chat</span>
        {info ? (
          <span
            className={`model-badge model-badge--${tone}`}
            title={`${info.provider} / ${info.model}`}
          >
            {info.provider}/{info.model}
          </span>
        ) : (
          <span className="model-badge model-badge--local">offline</span>
        )}
      </div>
      <div className="agent-stream" ref={streamRef}>
        {turns.length === 0 && (
          <div className="placeholder" style={{ margin: 0 }}>
            <p>No messages yet — say hello.</p>
          </div>
        )}
        {turns.map((t) => (
          <TurnCard key={t.id} turn={t} />
        ))}
      </div>
      <div className="agent-input-wrap">
        <textarea
          className="agent-input"
          placeholder={
            state === "connected"
              ? "Message Conduit…"
              : `Reconnecting… (${state})`
          }
          rows={3}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={onKey}
          disabled={state !== "connected"}
        />
        <button
          type="button"
          className="agent-send"
          aria-label="Send message"
          onClick={send}
          disabled={sendDisabled}
        >
          <SendHorizontal size={20} aria-hidden />
        </button>
      </div>
      <div className="agent-status">
        <span className="agent-status-left">
          <span className={`state-dot state-dot--${state}`} aria-hidden />
          <span>{stateLabel(state)}</span>
        </span>
        <span>{info?.version ? `v${info.version}` : ""}</span>
      </div>
    </section>
  );
}

function stateLabel(s: ConnectionState): string {
  switch (s) {
    case "connecting":
      return "connecting";
    case "connected":
      return "connected";
    case "disconnected":
      return "disconnected";
  }
}

// applyMessage reduces a single websocket frame into the turn list. The last
// in-progress turn (the one with done === false) is the active target. The
// session frame is sent once on connect and updates the header info; it does
// not start a new turn.
function applyMessage(
  msg: ServerMessage,
  setTurns: React.Dispatch<React.SetStateAction<Turn[]>>,
  setInfo: React.Dispatch<React.SetStateAction<Info | null>>,
): void {
  if (msg.type === "session") {
    setInfo((prev) => ({
      provider: msg.provider,
      model: msg.model,
      version: prev?.version ?? "",
      sessionId: msg.id,
    }));
    return;
  }

  setTurns((prev) => {
    const turns = [...prev];
    let active = turns[turns.length - 1];
    // Errors that arrive without a turn (e.g. before the user has prompted)
    // get shown as a synthetic assistant block so they're still visible.
    if (!active || active.done) {
      active = { id: -1, blocks: [], done: false };
      turns.push(active);
    }
    const blocks = [...active.blocks];

    switch (msg.type) {
      case "text_delta": {
        const last = blocks[blocks.length - 1];
        if (last && last.kind === "assistant") {
          blocks[blocks.length - 1] = { ...last, text: last.text + msg.text };
        } else {
          blocks.push({ kind: "assistant", text: msg.text });
        }
        break;
      }
      case "tool_use":
        blocks.push({
          kind: "tool",
          id: msg.id,
          name: msg.name,
          input: msg.input,
          output: null,
          isError: false,
        });
        break;
      case "tool_result": {
        const idx = blocks.findIndex(
          (b) => b.kind === "tool" && b.id === msg.id,
        );
        if (idx >= 0) {
          const t = blocks[idx] as ToolBlock;
          blocks[idx] = {
            ...t,
            output: msg.output,
            isError: msg.isError,
          };
        }
        break;
      }
      case "end_turn":
        turns[turns.length - 1] = { ...active, blocks, done: true };
        return turns;
      case "error":
        blocks.push({
          kind: "assistant",
          text: `[error] ${msg.message}`,
        });
        break;
    }

    turns[turns.length - 1] = { ...active, blocks };
    return turns;
  });
}

function TurnCard({ turn }: { turn: Turn }) {
  return (
    <div className="turn">
      {turn.blocks.map((b, i) => {
        if (b.kind === "user") {
          return (
            <div key={i} className="message user">
              {b.text}
            </div>
          );
        }
        if (b.kind === "assistant") {
          return (
            <div key={i} className="message agent">
              {b.text}
            </div>
          );
        }
        return <ToolCall key={i} block={b} />;
      })}
    </div>
  );
}

function ToolCall({ block }: { block: ToolBlock }) {
  const [open, setOpen] = useState(false);
  const pillClass =
    block.output === null
      ? "tool-pill--running"
      : block.isError
        ? "tool-pill--error"
        : "tool-pill--ok";
  const pillLabel =
    block.output === null ? "running" : block.isError ? "error" : "ok";
  return (
    <div className={`tool-call ${block.isError ? "error" : ""}`}>
      <button
        type="button"
        className="tool-head"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
      >
        <span className="tool-arrow">
          {open ? (
            <ChevronDown size={14} aria-hidden />
          ) : (
            <ChevronRight size={14} aria-hidden />
          )}
        </span>
        <span className="tool-name">{block.name}</span>
        <span className="tool-input">{block.input}</span>
        <span className="tool-summary">
          <span className={`tool-pill ${pillClass}`}>{pillLabel}</span>
        </span>
      </button>
      {open && block.output !== null && (
        <pre className="tool-output">{block.output}</pre>
      )}
    </div>
  );
}
