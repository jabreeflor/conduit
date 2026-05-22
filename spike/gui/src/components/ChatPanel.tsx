import { useEffect, useRef, useState } from "react";
import { Icon } from "./Icon";
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

type AssistantBlock = { kind: "assistant"; text: string };
type UserBlock = { kind: "user"; text: string };
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
    default:
      return "local";
  }
}

// Seed turns used when ?demo=chat is set — lets the chat surface be captured
// for design QA without a live backend. Mirrors the mockup exchange.
const DEMO_TURNS: Turn[] = [
  {
    id: 1,
    done: true,
    blocks: [
      { kind: "user", text: "Give me a one-line hello for a screenshot." },
      { kind: "assistant", text: "Hello — nice to see you!" },
    ],
  },
];

export function ChatPanel({
  initialPrompt = null,
  templateId = null,
  demo = false,
}: {
  initialPrompt?: string | null;
  templateId?: string | null;
  demo?: boolean;
}) {
  const [info, setInfo] = useState<Info | null>(null);
  const [state, setState] = useState<ConnectionState>("connecting");
  const [turns, setTurns] = useState<Turn[]>(demo ? DEMO_TURNS : []);
  const [draft, setDraft] = useState("");
  const clientRef = useRef<ReturnType<typeof connectAgent> | null>(null);
  const streamRef = useRef<HTMLDivElement | null>(null);
  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const turnIdRef = useRef(demo ? DEMO_TURNS.length : 0);
  const initialSentRef = useRef(false);

  useEffect(() => {
    if (demo) return;
    getInfo()
      .then(setInfo)
      .catch(() => {
        // Info is non-essential; the session frame carries provider/model.
      });
  }, [demo]);

  useEffect(() => {
    if (demo) return;
    const client = connectAgent(
      { onState: setState, onMessage: (msg) => applyMessage(msg, setTurns, setInfo) },
      { templateId: templateId ?? undefined },
    );
    clientRef.current = client;
    return () => client.close();
  }, [demo, templateId]);

  // Auto-send the welcome-screen prompt once the WS is connected.
  useEffect(() => {
    if (demo || state !== "connected" || initialSentRef.current) return;
    const text = initialPrompt?.trim();
    if (!text) return;
    initialSentRef.current = true;
    const id = ++turnIdRef.current;
    setTurns((prev) => [
      ...prev,
      { id, blocks: [{ kind: "user", text }], done: false },
    ]);
    clientRef.current?.send({ type: "prompt", text });
  }, [state, initialPrompt, demo]);

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

  useEffect(() => {
    const el = inputRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${el.scrollHeight}px`;
  }, [draft]);

  const connected = demo || state === "connected";
  const tone = providerTone(info?.provider);
  const modelLabel = info ? `${info.provider}/${info.model}` : "codex/gpt-5.5";

  return (
    <section className="agent-panel" aria-label="Agent">
      <div className="agent-stream" ref={streamRef}>
        <div className="agent-stream-inner">
          {turns.length === 0 && (
            <div className="placeholder">No messages yet — say hello.</div>
          )}
          {turns.map((t) => (
            <TurnCard key={t.id} turn={t} />
          ))}
        </div>
      </div>

      <div className="agent-input-region">
        <div className="agent-composer">
          <textarea
            ref={inputRef}
            className="agent-input"
            placeholder={connected ? "Message Conduit…" : `Reconnecting… (${state})`}
            rows={1}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={onKey}
            disabled={!connected}
          />
          <div className="agent-composer-bar">
            <div className="agent-composer-icons">
              <button
                type="button"
                className="composer-icon"
                title="Attach"
                aria-label="Attach"
              >
                <Icon name="attach_file" size={18} />
              </button>
              <button
                type="button"
                className="composer-icon"
                title="Voice"
                aria-label="Voice"
              >
                <Icon name="mic" size={18} />
              </button>
              <button
                type="button"
                className="composer-icon"
                title="Image"
                aria-label="Image"
              >
                <Icon name="image" size={18} />
              </button>
            </div>
            <div className="agent-composer-right">
              <span className={`model-badge model-badge--${tone}`} title={modelLabel}>
                {modelLabel}
              </span>
              <button
                type="button"
                className="agent-send"
                onClick={send}
                disabled={!connected || draft.trim().length === 0}
                aria-label="Send"
              >
                <Icon name="arrow_forward" size={20} />
              </button>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

// applyMessage reduces a single websocket frame into the turn list.
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
          blocks[idx] = { ...t, output: msg.output, isError: msg.isError };
        }
        break;
      }
      case "end_turn":
        turns[turns.length - 1] = { ...active, blocks, done: true };
        return turns;
      case "error":
        blocks.push({ kind: "assistant", text: `[error] ${msg.message}` });
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
            <div key={i} className="msg-user-wrap">
              <div className="message user">{b.text}</div>
            </div>
          );
        }
        if (b.kind === "assistant") {
          return (
            <div key={i} className="msg-agent-wrap">
              <div className="msg-agent-mark" aria-hidden>
                <Icon name="terminal" size={18} />
              </div>
              <div className="message agent">{b.text}</div>
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
          <Icon name={open ? "expand_more" : "chevron_right"} size={16} />
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
