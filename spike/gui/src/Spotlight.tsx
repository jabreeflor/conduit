import { useEffect, useMemo, useRef, useState } from "react";

// SpotlightResult mirrors the Go SpotlightResult shape in
// internal/gui/spotlight.go. Real ranking happens in the core; this
// scaffold uses a static set so the overlay layout can be iterated
// against without a live backend.
type SpotlightResult = {
  id: string;
  kind: "command" | "workflow" | "memory" | "recent" | "session";
  title: string;
  subtitle?: string;
};

const STATIC_RESULTS: SpotlightResult[] = [
  { id: "cmd:new-session", kind: "command", title: "New session", subtitle: "/new" },
  { id: "wf:morning-brief", kind: "workflow", title: "Run morning-brief", subtitle: "workflow" },
  { id: "mem:soul", kind: "memory", title: "Open SOUL.md", subtitle: "~/.conduit/SOUL.md" },
  { id: "cmd:fork", kind: "command", title: "Fork session", subtitle: "/fork" },
  { id: "ses:abc123", kind: "session", title: "Resume: refactor router", subtitle: "2026-05-05" },
];

export function Spotlight({ onClose }: { onClose: () => void }) {
  const [query, setQuery] = useState("");
  const [cursor, setCursor] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const results = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return STATIC_RESULTS;
    return STATIC_RESULTS.filter((r) => r.title.toLowerCase().includes(q));
  }, [query]);

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
      // Activation is a no-op in the scaffold — real dispatch ships in #50.
      console.log("activate:", results[cursor]);
      onClose();
    }
  }

  return (
    <div className="spotlight-backdrop" onClick={onClose}>
      <div className="spotlight" onClick={(e) => e.stopPropagation()}>
        <input
          ref={inputRef}
          className="spotlight-input"
          placeholder="Ask Conduit…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={onKey}
        />
        <ul className="spotlight-results">
          {results.map((r, i) => (
            <li
              key={r.id}
              className={`spotlight-row ${i === cursor ? "active" : ""}`}
              onMouseEnter={() => setCursor(i)}
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
      </div>
    </div>
  );
}
