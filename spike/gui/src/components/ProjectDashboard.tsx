import { useEffect, useState } from "react";
import { Icon } from "./Icon";
import { getProjects, type Project } from "../api";
import "./ProjectDashboard.css";

// ProjectDashboard is the per-project workspace overview surface: a left rail
// of project summary metadata + (honest, currently-empty) active-agents card, a
// wider right column of the project's real coding sessions and a (currently-
// empty) pinned-assets card, and a prominent "Start New Session" CTA at the
// bottom. Data is fetched live from `conduit serve` via getProjects(); the
// matched project's real chats drive the Recent Sessions feed. Cards with no
// backend (agents, pinned assets) render honest empty states rather than mock
// data.

type LoadState =
  | { kind: "loading" }
  | { kind: "error" }
  | { kind: "ready"; project: Project | null };

export function ProjectDashboard({
  projectId,
  projectName,
  onStartSession,
  onBack,
}: {
  projectId?: string;
  projectName?: string;
  onStartSession: () => void;
  onBack?: () => void;
}): JSX.Element {
  const [state, setState] = useState<LoadState>({ kind: "loading" });

  useEffect(() => {
    let cancelled = false;
    setState({ kind: "loading" });
    getProjects()
      .then(({ projects }) => {
        if (cancelled) return;
        const match =
          projects.find((p) => p.id === projectId) ?? projects[0] ?? null;
        setState({ kind: "ready", project: match });
      })
      .catch(() => {
        if (!cancelled) setState({ kind: "error" });
      });
    return () => {
      cancelled = true;
    };
  }, [projectId]);

  const project = state.kind === "ready" ? state.project : null;
  const title = projectName ?? project?.name ?? "Workspace";

  return (
    <div className="pw-root">
      <div className="pw-content">
        <header className="pw-header">
          <div className="pw-eyebrow-row">
            {onBack && (
              <button
                type="button"
                className="pw-back"
                onClick={onBack}
                aria-label="Back"
              >
                <Icon name="arrow_back" size={18} />
              </button>
            )}
            <span className="pw-eyebrow">{title}</span>
          </div>
          <h1 className="pw-title">Project Workspace</h1>
          {state.kind === "error" ? (
            <p className="pw-subtitle pw-subtitle-muted">
              Workspace data unavailable — start{" "}
              <span className="pw-code">conduit serve</span>.
            </p>
          ) : project ? (
            <p className="pw-subtitle">
              {project.sessionCount}{" "}
              {project.sessionCount === 1 ? "session" : "sessions"} at{" "}
              <span className="pw-code">{project.path}</span>
            </p>
          ) : state.kind === "loading" ? (
            <p className="pw-subtitle pw-subtitle-muted">Loading workspace…</p>
          ) : (
            <p className="pw-subtitle pw-subtitle-muted">
              Project not found.
            </p>
          )}
        </header>

        {state.kind === "ready" && project && (
          <div className="pw-grid">
            <div className="pw-col-left">
              <section
                className="pw-card pw-summary"
                aria-label="Project Summary"
              >
                <h2 className="pw-card-title">Project Summary</h2>

                <div className="pw-block">
                  <span className="pw-block-label">Path</span>
                  <p className="pw-block-text pw-block-mono">{project.path}</p>
                </div>

                <div className="pw-block">
                  <span className="pw-block-label">Branch</span>
                  <p className="pw-block-text pw-block-mono">
                    {project.branch || "—"}
                  </p>
                </div>

                <div className="pw-block">
                  <span className="pw-block-label">Sessions</span>
                  <p className="pw-block-text">
                    {project.sessionCount}{" "}
                    {project.sessionCount === 1 ? "session" : "sessions"}
                  </p>
                </div>
              </section>

              <section className="pw-card pw-agents" aria-label="Active Agents">
                <div className="pw-card-head">
                  <h2 className="pw-card-title">Active Agents</h2>
                </div>
                <p className="pw-empty">No agents attached yet.</p>
              </section>
            </div>

            <div className="pw-col-right">
              <section className="pw-section" aria-label="Recent Sessions">
                <div className="pw-section-head">
                  <h2 className="pw-section-title">Recent Sessions</h2>
                </div>

                <div className="pw-card pw-sessions">
                  {project.chats.length === 0 ? (
                    <p className="pw-empty pw-empty-inset">
                      No sessions yet in this project.
                    </p>
                  ) : (
                    <ul className="pw-session-list">
                      {project.chats.map((c) => (
                        <li key={c.id} className="pw-session">
                          <span className="pw-session-icon" aria-hidden>
                            <Icon name="forum" size={20} />
                          </span>
                          <div className="pw-session-main">
                            <div className="pw-session-head">
                              <span className="pw-session-title">
                                {c.title || "Untitled session"}
                              </span>
                              <span className="pw-session-time">
                                {relativeTime(c.createdAt)}
                              </span>
                            </div>
                            <p className="pw-session-preview">
                              {c.turnCount}{" "}
                              {c.turnCount === 1 ? "turn" : "turns"}
                            </p>
                          </div>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              </section>

              <section className="pw-card pw-pinned" aria-label="Pinned Assets">
                <div className="pw-card-head">
                  <h2 className="pw-card-title">Pinned Assets</h2>
                  <Icon
                    name="push_pin"
                    size={18}
                    fill
                    className="pw-pin-icon"
                  />
                </div>
                <p className="pw-empty">No pinned assets.</p>
              </section>
            </div>
          </div>
        )}

        <div className="pw-cta-row">
          <button
            type="button"
            className="pw-start-btn"
            onClick={onStartSession}
          >
            <Icon name="add_circle" size={22} fill />
            Start New Session
          </button>
        </div>
      </div>
    </div>
  );
}

// relativeTime collapses an ISO timestamp into the compact h/d/w/mo form used
// across the GUI (mirrors the Sidebar helper).
function relativeTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const diffMs = Date.now() - d.getTime();
  const min = Math.floor(diffMs / 60000);
  if (min < 1) return "now";
  if (min < 60) return `${min}m ago`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h ago`;
  const day = Math.floor(hr / 24);
  if (day < 7) return `${day}d ago`;
  const wk = Math.floor(day / 7);
  if (wk < 5) return `${wk}w ago`;
  const mo = Math.floor(day / 30);
  if (mo < 12) return `${mo}mo ago`;
  return `${Math.floor(day / 365)}y ago`;
}
