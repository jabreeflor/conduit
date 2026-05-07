import { useEffect, useRef, useState } from "react";
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

export function ChatPanel() {
  const [info, setInfo] = useState<Info | null>(null);
  const [state, setState] = useState<ConnectionState>("connecting");
  const [turns, setTurns] = useState<Turn[]>([]);
  const [draft, setDraft] = useState("");
  const clientRef = useRef<ReturnType<typeof connectAgent> | null>(null);
  const streamRef = useRef<HTMLDivElement | null>(null);
  const turnIdRef = useRef(0);

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

  const header = info ? `${info.provider} / ${info.model}` : "Chat";

  return (
    <section className="agent-panel" aria-label="Agent">
      <div className="agent-header">{header}</div>
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
      <div className="agent-status">
        <span>{stateLabel(state)}</span>
        <span>{info?.version ? `v${info.version}` : ""}</span>
      </div>
    </section>
  );
}

function stateLabel(s: ConnectionState): string {
  switch (s) {
    case "connecting":
      return "● connecting";
    case "connected":
      return "● connected";
    case "disconnected":
      return "● disconnected";
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
  const summary =
    block.output === null
      ? "running…"
      : block.isError
        ? "error"
        : "ok";
  return (
    <div className={`tool-call ${block.isError ? "error" : ""}`}>
      <button
        type="button"
        className="tool-head"
        onClick={() => setOpen((v) => !v)}
      >
        <span className="tool-arrow">{open ? "▼" : "▶"}</span>
        <span className="tool-name">{block.name}</span>
        <span className="tool-input">{block.input}</span>
        <span className="tool-summary">→ {summary}</span>
      </button>
      {open && block.output !== null && (
        <pre className="tool-output">{block.output}</pre>
      )}
    </div>
  );
}
