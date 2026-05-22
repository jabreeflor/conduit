import { useEffect, useState } from "react";
import { Icon } from "./Icon";
import { getSessions, type SessionMeta } from "../api";

// The welcome-screen right rail. Originally a mocked "team" feed; now wired to
// the real recent coding sessions (`getSessions` → /api/sessions). There is no
// multi-user collaboration backend yet, so this shows *your* recent activity
// rather than fabricated teammates.
type LoadState =
  | { status: "loading" }
  | { status: "error" }
  | { status: "loaded"; sessions: SessionMeta[] };

function relativeTime(iso: string): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return "";
  const sec = Math.max(0, (Date.now() - then) / 1000);
  if (sec < 60) return "just now";
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ago`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h ago`;
  const day = Math.floor(hr / 24);
  if (day < 7) return `${day}d ago`;
  return `${Math.floor(day / 7)}w ago`;
}

export function TeamActivity() {
  const [state, setState] = useState<LoadState>({ status: "loading" });

  useEffect(() => {
    let cancelled = false;
    getSessions()
      .then((sessions) => {
        if (!cancelled) setState({ status: "loaded", sessions });
      })
      .catch(() => {
        if (!cancelled) setState({ status: "error" });
      });
    return () => {
      cancelled = true;
    };
  }, []);

  let body: JSX.Element;
  if (state.status === "loading") {
    body = <p className="soul-text">Loading activity…</p>;
  } else if (state.status === "error") {
    body = (
      <p className="soul-text">
        Activity unavailable — start <code>conduit serve</code>.
      </p>
    );
  } else if (state.sessions.length === 0) {
    body = <p className="soul-text">No recent activity yet.</p>;
  } else {
    body = (
      <div className="activity-list">
        {state.sessions.slice(0, 8).map((s) => (
          <div className="activity-item" key={s.id}>
            <div className="activity-icon accent">
              <Icon name="forum" size={16} />
            </div>
            <div className="activity-meta">
              <p className="activity-text">{s.summary || s.id}</p>
              <span className="activity-ts">{relativeTime(s.createdAt)}</span>
            </div>
          </div>
        ))}
      </div>
    );
  }

  return (
    <aside className="context-panel" aria-label="Recent activity">
      <div className="context-panel-header">
        <span className="context-panel-title">Recent Activity</span>
        <span className="topbar-icon" aria-hidden>
          <Icon name="history" size={20} />
        </span>
      </div>
      <div className="context-panel-body">{body}</div>
    </aside>
  );
}
