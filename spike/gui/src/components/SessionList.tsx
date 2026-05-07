import { useEffect, useState } from "react";
import { getSessions, type SessionMeta } from "../api";

export function SessionList() {
  const [sessions, setSessions] = useState<SessionMeta[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getSessions()
      .then(setSessions)
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  }, []);

  if (error) {
    return (
      <div className="session-list-empty">
        Failed to load sessions: {error}
      </div>
    );
  }

  if (sessions === null) {
    return <div className="session-list-empty">Loading sessions…</div>;
  }

  if (sessions.length === 0) {
    return (
      <div className="session-list-empty">
        No sessions yet — start chatting on the right →
      </div>
    );
  }

  return (
    <ul className="session-list">
      {sessions.map((s) => (
        <li key={s.id} className="session-row">
          <div className="session-id">{s.id}</div>
          <div className="session-summary">{s.summary}</div>
          <div className="session-time">{formatTime(s.createdAt)}</div>
        </li>
      ))}
    </ul>
  );
}

function formatTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}
