import { useEffect, useState } from "react";
import { getMemory, saveMemory, type Memory } from "../api";
import { Icon } from "./Icon";

type LoadState =
  | { status: "loading" }
  | { status: "error" }
  | { status: "loaded"; memory: Memory };

// Render a chunk of SOUL.md / USER.md markdown into the panel's existing
// token-styled classes. Light-touch: `#`/`##` headings become .soul-heading,
// `-`/`*` bullets collect into a .soul-list, everything else is .soul-text.
// No markdown library — the source is simple headings + bullets + prose.
function renderMarkdown(md: string): JSX.Element[] {
  const lines = md.split("\n");
  const blocks: JSX.Element[] = [];
  let bullets: string[] = [];
  let key = 0;

  const flushBullets = () => {
    if (bullets.length === 0) return;
    const items = bullets;
    bullets = [];
    blocks.push(
      <ul className="soul-list" key={`ul-${key++}`}>
        {items.map((text, i) => (
          <li key={i}>
            <span className="soul-bullet" aria-hidden>
              •
            </span>
            <span>{text}</span>
          </li>
        ))}
      </ul>,
    );
  };

  for (const raw of lines) {
    const line = raw.trim();
    if (line === "") {
      flushBullets();
      continue;
    }
    const heading = line.match(/^#{1,6}\s+(.*)$/);
    if (heading) {
      flushBullets();
      blocks.push(
        <h4 className="soul-heading" key={`h-${key++}`}>
          {heading[1]}
        </h4>,
      );
      continue;
    }
    const bullet = line.match(/^[-*]\s+(.*)$/);
    if (bullet) {
      bullets.push(bullet[1]);
      continue;
    }
    flushBullets();
    blocks.push(
      <p className="soul-text" key={`p-${key++}`}>
        {line}
      </p>,
    );
  }
  flushBullets();
  return blocks;
}

// SoulPanel is the chat right rail: a live view of the agent's SOUL.md memory.
// On mount it fetches /api/memory (SOUL.md + USER.md raw markdown) and renders
// it. It can also edit: the pencil opens raw-markdown textareas and Save POSTs
// back to /api/memory (the write path the read handler anticipates). When the
// backend is offline the fetch throws and we show a calm hint rather than
// crashing.
export function SoulPanel({ onClose }: { onClose?: () => void }) {
  const [state, setState] = useState<LoadState>({ status: "loading" });
  const [editing, setEditing] = useState(false);
  const [draftSoul, setDraftSoul] = useState("");
  const [draftUser, setDraftUser] = useState("");
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    getMemory()
      .then((memory) => {
        if (!cancelled) setState({ status: "loaded", memory });
      })
      .catch(() => {
        if (!cancelled) setState({ status: "error" });
      });
    return () => {
      cancelled = true;
    };
  }, []);

  function startEdit() {
    if (state.status !== "loaded") return;
    setDraftSoul(state.memory.soul);
    setDraftUser(state.memory.user);
    setSaveError(null);
    setEditing(true);
  }

  function save() {
    setSaving(true);
    setSaveError(null);
    saveMemory({ soul: draftSoul, user: draftUser })
      .then((memory) => {
        setState({ status: "loaded", memory });
        setEditing(false);
      })
      .catch(() =>
        setSaveError("Couldn't save — is `conduit serve` running?"),
      )
      .finally(() => setSaving(false));
  }

  let body: JSX.Element;
  if (editing) {
    body = (
      <div className="soul-edit">
        <label className="soul-heading" htmlFor="soul-edit-soul">
          SOUL.md
        </label>
        <textarea
          id="soul-edit-soul"
          className="soul-edit-area"
          value={draftSoul}
          onChange={(e) => setDraftSoul(e.target.value)}
          placeholder="# Core Identity&#10;…"
          spellCheck={false}
        />
        <label className="soul-heading" htmlFor="soul-edit-user">
          USER.md
        </label>
        <textarea
          id="soul-edit-user"
          className="soul-edit-area"
          value={draftUser}
          onChange={(e) => setDraftUser(e.target.value)}
          placeholder="# User&#10;…"
          spellCheck={false}
        />
        {saveError && <p className="soul-edit-error">{saveError}</p>}
        <div className="soul-edit-actions">
          <button
            type="button"
            className="soul-edit-btn"
            onClick={() => setEditing(false)}
            disabled={saving}
          >
            Cancel
          </button>
          <button
            type="button"
            className="soul-edit-btn soul-edit-btn-primary"
            onClick={save}
            disabled={saving}
          >
            {saving ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    );
  } else if (state.status === "loading") {
    body = <p className="soul-text">Loading memory…</p>;
  } else if (state.status === "error") {
    body = (
      <p className="soul-text">
        Memory unavailable — start <code>conduit serve</code> to load SOUL.md.
      </p>
    );
  } else {
    const soul = state.memory.soul.trim();
    const user = state.memory.user.trim();
    if (soul === "" && user === "") {
      body = (
        <p className="soul-text">
          No memory written yet. Use the pencil to seed SOUL.md.
        </p>
      );
    } else {
      body = (
        <>
          {soul !== "" && (
            <div className="soul-section">{renderMarkdown(soul)}</div>
          )}
          {user !== "" && (
            <div className="soul-section">
              <h4 className="soul-heading">User</h4>
              {renderMarkdown(user)}
            </div>
          )}
        </>
      );
    }
  }

  return (
    <aside className="context-panel" aria-label="soul.md">
      <div className="context-panel-header">
        <span className="context-panel-title">
          <span className="accent" aria-hidden>
            <Icon name="psychology" size={18} fill />
          </span>
          soul.md
        </span>
        <span className="context-panel-actions">
          {state.status === "loaded" && !editing && (
            <button
              type="button"
              className="topbar-icon"
              onClick={startEdit}
              aria-label="Edit memory"
            >
              <Icon name="edit" size={18} />
            </button>
          )}
          {onClose && (
            <button
              type="button"
              className="topbar-icon"
              onClick={onClose}
              aria-label="Close soul.md"
            >
              <Icon name="close" size={20} />
            </button>
          )}
        </span>
      </div>
      <div className="context-panel-body">{body}</div>
    </aside>
  );
}
