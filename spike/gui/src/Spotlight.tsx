import { useEffect, useMemo, useRef, useState } from "react";
import { Icon } from "./components/Icon";
import { getSessions } from "./api";

// Static commands are local; recent sessions are pulled live from
// /api/sessions and merged in. (A server-side ranking service —
// internal/gui/spotlight.go — is still scaffold; this client-side merge is the
// interim.) `search` is the plain-text used for filtering since `title` may be
// a React node.
type SpotlightResult = {
  id: string;
  kind: "command" | "workflow" | "memory" | "recent" | "session";
  title: React.ReactNode;
  subtitle?: string;
  search: string;
};

const COMMANDS: SpotlightResult[] = [
  { id: "nav:new-chat", kind: "command", title: "New chat", subtitle: "/new", search: "new chat session" },
  { id: "nav:projects", kind: "command", title: "Go to Projects", subtitle: "/projects", search: "go to projects" },
  { id: "nav:new-project", kind: "command", title: "New Project", subtitle: "/projects new", search: "new project create" },
  { id: "nav:soul", kind: "memory", title: "Open Soul", subtitle: "/soul", search: "open soul memory page" },
  { id: "nav:settings", kind: "command", title: "Settings", subtitle: "/settings", search: "settings preferences" },
  { id: "act:cycle-theme", kind: "command", title: "Switch theme", subtitle: "light · dark · hc", search: "switch theme light dark high contrast appearance" },
];

function relativeTime(iso: string): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return "session";
  const min = Math.floor(Math.max(0, (Date.now() - then) / 60000));
  if (min < 1) return "just now";
  if (min < 60) return `${min}m ago`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h ago`;
  return `${Math.floor(hr / 24)}d ago`;
}

export function Spotlight({
  onClose,
  onAction,
  initialQuery = "",
}: {
  onClose: () => void;
  onAction?: (id: string) => void;
  initialQuery?: string;
}) {
  const [query, setQuery] = useState(initialQuery);
  const [cursor, setCursor] = useState(0);
  const [sessions, setSessions] = useState<SpotlightResult[]>([]);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
    inputRef.current?.select();
  }, []);

  // Pull recent sessions into the palette as "session" results. Best-effort:
  // if the server is offline we just show the static commands.
  useEffect(() => {
    let cancelled = false;
    getSessions()
      .then((list) => {
        if (cancelled) return;
        setSessions(
          list.slice(0, 8).map((s) => ({
            id: `ses:${s.id}`,
            kind: "session" as const,
            title: s.summary || s.id,
            subtitle: relativeTime(s.createdAt),
            search: (s.summary || s.id).toLowerCase(),
          })),
        );
      })
      .catch(() => {
        /* offline — commands only */
      });
    return () => {
      cancelled = true;
    };
  }, []);

  function activate(result: SpotlightResult) {
    onAction?.(result.id);
    onClose();
  }

  const results = useMemo(() => {
    const all = [...COMMANDS, ...sessions];
    const q = query.trim().toLowerCase();
    if (!q) return all;
    return all.filter((r) => r.search.includes(q));
  }, [query, sessions]);

  useEffect(() => {
    setCursor(0);
  }, [query]);

  function onKey(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setCursor((c) => (results.length ? (c + 1) % results.length : 0));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setCursor((c) => (results.length ? (c - 1 + results.length) % results.length : 0));
    } else if (e.key === "Enter" && results[cursor]) {
      e.preventDefault();
      activate(results[cursor]);
    }
  }

  return (
    <div className="spotlight-backdrop" onClick={onClose}>
      <div className="spotlight" onClick={(e) => e.stopPropagation()}>
        <div className="spotlight-input-row">
          <span className="spotlight-prompt-icon" aria-hidden>
            <Icon name="terminal" size={24} />
          </span>
          <input
            ref={inputRef}
            className="spotlight-input"
            placeholder="Ask Conduit…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onKey}
          />
        </div>

        <ul className="spotlight-results">
          {results.map((r, i) => (
            <li
              key={r.id}
              className={`spotlight-row ${i === cursor ? "active" : ""}`}
              onMouseEnter={() => setCursor(i)}
              onClick={() => activate(r)}
            >
              <span className={`kind kind-${r.kind}`}>{r.kind}</span>
              <span className="title">{r.title}</span>
              {r.subtitle && <span className="subtitle">{r.subtitle}</span>}
            </li>
          ))}
          {results.length === 0 && (
            <li className="spotlight-row empty">No matches</li>
          )}
        </ul>

        <div className="spotlight-footer">
          <div className="spotlight-hints">
            <span className="spotlight-hint">
              <kbd>ESC</kbd> to close
            </span>
            <span className="spotlight-hint">
              <kbd>↑↓</kbd> to navigate
            </span>
          </div>
          <div className="spotlight-model">
            codex/gpt-5.5
            <Icon name="bolt" size={14} />
          </div>
        </div>
      </div>
    </div>
  );
}
