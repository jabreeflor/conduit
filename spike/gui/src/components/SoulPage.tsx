import { useEffect, useState } from "react";
import { getMemory, saveMemory, type Memory } from "../api";
import { Icon } from "./Icon";
import "./SoulPage.css";

// Which of the two files an action targets. Each card edits independently so a
// single Save touches only one document, but both fields are always sent to the
// backend so the SOUL.md / USER.md pair stays consistent on disk.
type FileKey = "soul" | "user";

type LoadState =
  | { status: "loading" }
  | { status: "error" }
  | { status: "loaded"; memory: Memory };

// Light-touch markdown render — no library. `#`/`##` lines become headings,
// `-`/`*` lines collect into a bullet list, blank lines flush the current list,
// everything else is a paragraph. Mirrors the approach in SoulPanel.tsx so the
// two views stay visually consistent.
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
      <ul className="sp-list" key={`ul-${key++}`}>
        {items.map((text, i) => (
          <li className="sp-list-item" key={i}>
            <span className="sp-bullet" aria-hidden>
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
    const h1 = line.match(/^#\s+(.*)$/);
    if (h1) {
      flushBullets();
      blocks.push(
        <h3 className="sp-md-h1" key={`h1-${key++}`}>
          {h1[1]}
        </h3>,
      );
      continue;
    }
    const h2 = line.match(/^#{2,6}\s+(.*)$/);
    if (h2) {
      flushBullets();
      blocks.push(
        <h4 className="sp-md-h2" key={`h2-${key++}`}>
          {h2[1]}
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
      <p className="sp-md-p" key={`p-${key++}`}>
        {line}
      </p>,
    );
  }
  flushBullets();
  return blocks;
}

type CardMeta = {
  key: FileKey;
  filename: string;
  icon: string;
  caption: string;
  placeholder: string;
};

const CARDS: CardMeta[] = [
  {
    key: "soul",
    filename: "SOUL.md",
    icon: "psychology",
    caption: "Identity — who your agent is.",
    placeholder: "# Core Identity\n\n- …",
  },
  {
    key: "user",
    filename: "USER.md",
    icon: "person",
    caption: "About you — what your agent should know.",
    placeholder: "# User\n\n- …",
  },
];

// SoulPage is the full-page editor for the agent's persistent memory. It renders
// inside the app content column (the sidebar + top bar live outside). On mount
// it fetches /api/memory (raw SOUL.md + USER.md markdown). Each file gets its own
// card with a rendered read view and a per-card edit mode (raw-markdown textarea).
// Saving POSTs both fields back to /api/memory so the pair stays in sync. When
// the backend is offline the fetch throws and we show a calm hint.
export function SoulPage(): JSX.Element {
  const [state, setState] = useState<LoadState>({ status: "loading" });
  const [editingKey, setEditingKey] = useState<FileKey | null>(null);
  const [draft, setDraft] = useState("");
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

  function startEdit(key: FileKey) {
    if (state.status !== "loaded") return;
    setDraft(state.memory[key]);
    setEditingKey(key);
    setSaveError(null);
  }

  function cancelEdit() {
    setEditingKey(null);
    setSaveError(null);
  }

  function save(key: FileKey) {
    if (state.status !== "loaded") return;
    const next: Memory = { ...state.memory, [key]: draft };
    setSaving(true);
    setSaveError(null);
    saveMemory(next)
      .then((memory) => {
        setState({ status: "loaded", memory });
        setEditingKey(null);
      })
      .catch(() => setSaveError("Couldn't save — is `conduit serve` running?"))
      .finally(() => setSaving(false));
  }

  function renderCard(meta: CardMeta): JSX.Element {
    const loaded = state.status === "loaded";
    const value = loaded ? state.memory[meta.key] : "";
    const isEditing = editingKey === meta.key;
    const trimmed = value.trim();

    return (
      <section className="sp-card" key={meta.key} aria-label={meta.filename}>
        <header className="sp-card-head">
          <span className="sp-card-title">
            <span className="sp-card-icon" aria-hidden>
              <Icon name={meta.icon} size={20} fill />
            </span>
            <span>
              <span className="sp-card-name">{meta.filename}</span>
              <span className="sp-card-caption">{meta.caption}</span>
            </span>
          </span>
          {loaded && !isEditing && (
            <button
              type="button"
              className="sp-icon-btn"
              onClick={() => startEdit(meta.key)}
              aria-label={`Edit ${meta.filename}`}
            >
              <Icon name="edit" size={18} />
            </button>
          )}
        </header>

        <div className="sp-card-body">
          {isEditing ? (
            <div className="sp-edit">
              <textarea
                className="sp-textarea"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                placeholder={meta.placeholder}
                spellCheck={false}
                disabled={saving}
                aria-label={`${meta.filename} source`}
              />
              {saveError && <p className="sp-save-error">{saveError}</p>}
              <div className="sp-actions">
                <button
                  type="button"
                  className="sp-btn sp-btn-ghost"
                  onClick={cancelEdit}
                  disabled={saving}
                >
                  <Icon name="close" size={16} />
                  Cancel
                </button>
                <button
                  type="button"
                  className="sp-btn sp-btn-primary"
                  onClick={() => save(meta.key)}
                  disabled={saving}
                >
                  <Icon name="save" size={16} fill />
                  {saving ? "Saving…" : "Save"}
                </button>
              </div>
            </div>
          ) : trimmed === "" ? (
            <p className="sp-empty">
              Nothing written yet. Use the pencil to seed {meta.filename}.
            </p>
          ) : (
            <div className="sp-md">{renderMarkdown(value)}</div>
          )}
        </div>
      </section>
    );
  }

  let body: JSX.Element;
  if (state.status === "loading") {
    body = <p className="sp-status">Loading memory…</p>;
  } else if (state.status === "error") {
    body = (
      <p className="sp-status sp-status-muted">
        Memory unavailable — start <code className="sp-code">conduit serve</code>.
      </p>
    );
  } else {
    body = <div className="sp-cards">{CARDS.map(renderCard)}</div>;
  }

  return (
    <div className="sp-root">
      <div className="sp-content">
        <header className="sp-header">
          <span className="sp-eyebrow-row">
            <span className="sp-eyebrow-icon" aria-hidden>
              <Icon name="psychology" size={20} fill />
            </span>
            <span className="sp-eyebrow">Memory</span>
          </span>
          <h1 className="sp-title">Soul</h1>
          <p className="sp-subtitle">
            Your agent's persistent memory — SOUL.md (identity) and USER.md
            (about you).
          </p>
        </header>
        {body}
      </div>
    </div>
  );
}
