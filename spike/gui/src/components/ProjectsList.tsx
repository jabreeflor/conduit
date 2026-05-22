import { useEffect, useMemo, useState } from "react";
import { Icon } from "./Icon";
import { getProjects, type Project, type ChatSummary } from "../api";
import { loadLocalProjects, type LocalProject } from "../localProjects";
import "./ProjectsList.css";

// ProjectsList is the workspace browser surface: a filterable list of project
// cards plus a "create new" affordance and a bottom row of helper bento cards.
// Project data is live — fetched from the backend's /api/projects, which groups
// real coding-session journals by git repo root. Only truthful, backend-backed
// facts are rendered (name, path, branch, session count, last activity); the
// earlier mock fields (fake "agents active", member avatars, visibility,
// descriptions) were intentionally removed since the backend provides no such
// data and showing them next to a real repo would be misleading.

type Filter = "all" | "favorites";

const FILTERS: { id: Filter; label: string }[] = [
  { id: "all", label: "All Projects" },
  { id: "favorites", label: "Favorites" },
];

type Sort = "recent" | "name";

const SORTS: { id: Sort; label: string }[] = [
  { id: "recent", label: "Recently Updated" },
  { id: "name", label: "Name" },
];

const FAVORITES_KEY = "conduit.favorites";

const HELPERS: { id: string; icon: string; label: string; desc: string }[] = [
  {
    id: "templates",
    icon: "auto_awesome",
    label: "Project Templates",
    desc: "Start from a curated blueprint and ship in minutes.",
  },
  {
    id: "deploy",
    icon: "rocket_launch",
    label: "Quick Deploy",
    desc: "Push a workspace live with a single guided flow.",
  },
  {
    id: "vault",
    icon: "shield",
    label: "Vault Integration",
    desc: "Connect encrypted secrets and credentials securely.",
  },
];

// loadFavorites reads the persisted favorite-project id set from localStorage,
// tolerating absent/corrupt values by falling back to an empty set.
function loadFavorites(): Set<string> {
  try {
    const raw = window.localStorage.getItem(FAVORITES_KEY);
    if (!raw) return new Set();
    const parsed: unknown = JSON.parse(raw);
    if (Array.isArray(parsed)) {
      return new Set(parsed.filter((v): v is string => typeof v === "string"));
    }
  } catch {
    // Corrupt or unavailable storage — start fresh rather than crash.
  }
  return new Set();
}

// relativeTime renders an ISO timestamp as a compact "Updated …" phrase.
function relativeTime(iso: string): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return "Updated recently";
  const diffSec = Math.max(0, (Date.now() - then) / 1000);
  if (diffSec < 60) return "Updated just now";
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `Updated ${diffMin}m ago`;
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) return `Updated ${diffHr}h ago`;
  const diffDay = Math.floor(diffHr / 24);
  if (diffDay < 7) return `Updated ${diffDay}d ago`;
  const diffWk = Math.floor(diffDay / 7);
  if (diffWk < 5) return `Updated ${diffWk}w ago`;
  const diffMo = Math.floor(diffDay / 30);
  if (diffMo < 12) return `Updated ${diffMo}mo ago`;
  return `Updated ${Math.floor(diffDay / 365)}y ago`;
}

export function ProjectsList({
  onCreateNew,
  onEnter,
}: {
  onCreateNew: () => void;
  onEnter: (project: { id: string; title: string }) => void;
}): JSX.Element {
  const [filter, setFilter] = useState<Filter>("all");
  const [sort, setSort] = useState<Sort>("recent");
  const [sortOpen, setSortOpen] = useState(false);
  const [projects, setProjects] = useState<Project[]>([]);
  const [orphans, setOrphans] = useState<ChatSummary[]>([]);
  const [status, setStatus] = useState<"loading" | "error" | "ready">(
    "loading",
  );
  const [favorites, setFavorites] = useState<Set<string>>(loadFavorites);
  const [localProjects] = useState<LocalProject[]>(loadLocalProjects);

  useEffect(() => {
    let cancelled = false;
    getProjects()
      .then((res) => {
        if (cancelled) return;
        setProjects(res.projects);
        setOrphans(res.orphans);
        setStatus("ready");
      })
      .catch(() => {
        if (cancelled) return;
        setStatus("error");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  function toggleFavorite(id: string): void {
    setFavorites((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      try {
        window.localStorage.setItem(FAVORITES_KEY, JSON.stringify([...next]));
      } catch {
        // Persistence is best-effort; in-memory state still updates.
      }
      return next;
    });
  }

  const visibleProjects = useMemo(() => {
    const filtered =
      filter === "favorites"
        ? projects.filter((p) => favorites.has(p.id))
        : projects;
    const sorted = [...filtered];
    if (sort === "name") {
      sorted.sort((a, b) => a.name.localeCompare(b.name));
    } else {
      sorted.sort(
        (a, b) => Date.parse(b.lastActivity) - Date.parse(a.lastActivity),
      );
    }
    return sorted;
  }, [projects, favorites, filter, sort]);

  const visibleLocal = useMemo(
    () =>
      filter === "favorites"
        ? localProjects.filter((p) => favorites.has(p.id))
        : localProjects,
    [localProjects, filter, favorites],
  );

  const sortLabel =
    SORTS.find((s) => s.id === sort)?.label ?? SORTS[0].label;

  const isEmpty =
    status === "ready" &&
    projects.length === 0 &&
    orphans.length === 0 &&
    localProjects.length === 0;

  return (
    <div className="pl-root">
      <div className="pl-content">
        <div className="pl-filter-row">
          <div className="pl-filters" role="group" aria-label="Project filters">
            {FILTERS.map((f) => (
              <button
                key={f.id}
                type="button"
                aria-pressed={filter === f.id}
                className={`pl-pill${filter === f.id ? " pl-pill-active" : ""}`}
                onClick={() => setFilter(f.id)}
              >
                {f.label}
              </button>
            ))}
          </div>
          <div className="pl-sort">
            <span className="pl-sort-label">Sort by:</span>
            <div className="pl-sort-menu">
              <button
                type="button"
                className="pl-sort-btn"
                aria-haspopup="listbox"
                aria-expanded={sortOpen}
                onClick={() => setSortOpen((o) => !o)}
              >
                {sortLabel}
                <Icon name="expand_more" size={18} />
              </button>
              {sortOpen && (
                <ul className="pl-sort-options" role="listbox">
                  {SORTS.map((s) => (
                    <li key={s.id} role="option" aria-selected={sort === s.id}>
                      <button
                        type="button"
                        className={`pl-sort-option${
                          sort === s.id ? " pl-sort-option-active" : ""
                        }`}
                        onClick={() => {
                          setSort(s.id);
                          setSortOpen(false);
                        }}
                      >
                        {s.label}
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </div>
        </div>

        <button type="button" className="pl-create" onClick={onCreateNew}>
          <span className="pl-create-tile">
            <Icon name="add" size={28} />
          </span>
          <span className="pl-create-label">Create New Project</span>
        </button>

        {visibleLocal.length > 0 && (
          <div className="pl-projects">
            {visibleLocal.map((p) => {
              const fav = favorites.has(p.id);
              return (
                <div key={p.id} className="pl-card pl-project">
                  <div className="pl-thumb">
                    <Icon name="folder" size={30} fill />
                  </div>
                  <div className="pl-project-main">
                    <div className="pl-project-head">
                      <h3 className="pl-project-title">{p.name}</h3>
                      <span className="pl-tag pl-tag-private">Local</span>
                    </div>
                    <p className="pl-project-desc">
                      {p.description || "Local project (not yet synced)"}
                    </p>
                    <div className="pl-meta">
                      <span className="pl-status-pill">
                        <Icon name="lock" size={14} />
                        {p.visibility}
                      </span>
                      <span className="pl-private-note">
                        {relativeTime(p.createdAt)}
                      </span>
                    </div>
                  </div>
                  <div className="pl-project-actions">
                    <button
                      type="button"
                      className="pl-icon-btn"
                      title={fav ? "Unstar project" : "Star project"}
                      aria-label={fav ? "Unstar project" : "Star project"}
                      aria-pressed={fav}
                      onClick={() => toggleFavorite(p.id)}
                    >
                      <Icon name="star" size={20} fill={fav} />
                    </button>
                    <button
                      type="button"
                      className="pl-enter-btn"
                      onClick={() => onEnter({ id: p.id, title: p.name })}
                    >
                      Enter Workspace
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        )}

        {status === "loading" && (
          <p className="pl-notice">Loading projects…</p>
        )}

        {status === "error" && (
          <p className="pl-notice">
            Projects unavailable — start <code>conduit serve</code>.
          </p>
        )}

        {isEmpty && (
          <p className="pl-notice">
            No projects yet. Create one to get started.
          </p>
        )}

        {status === "ready" && (
          <>
            <div className="pl-projects">
              {visibleProjects.map((p) => {
                const fav = favorites.has(p.id);
                return (
                  <div key={p.id} className="pl-card pl-project">
                    <div className="pl-thumb">
                      <Icon name="account_tree" size={30} fill />
                    </div>
                    <div className="pl-project-main">
                      <div className="pl-project-head">
                        <h3 className="pl-project-title">{p.name}</h3>
                      </div>
                      <p className="pl-project-desc">{p.path}</p>
                      <div className="pl-meta">
                        {p.branch && (
                          <span className="pl-status-pill">
                            <Icon name="account_tree" size={14} />
                            {p.branch}
                          </span>
                        )}
                        <span className="pl-status-pill">
                          <Icon name="forum" size={14} />
                          {p.sessionCount}{" "}
                          {p.sessionCount === 1 ? "session" : "sessions"}
                        </span>
                        <span className="pl-private-note">
                          {relativeTime(p.lastActivity)}
                        </span>
                      </div>
                    </div>
                    <div className="pl-project-actions">
                      <button
                        type="button"
                        className="pl-icon-btn"
                        title={fav ? "Unstar project" : "Star project"}
                        aria-label={fav ? "Unstar project" : "Star project"}
                        aria-pressed={fav}
                        onClick={() => toggleFavorite(p.id)}
                      >
                        <Icon name="star" size={20} fill={fav} />
                      </button>
                      <button
                        type="button"
                        className="pl-enter-btn"
                        onClick={() => onEnter({ id: p.id, title: p.name })}
                      >
                        Enter Workspace
                      </button>
                    </div>
                  </div>
                );
              })}
              {filter === "favorites" &&
                projects.length > 0 &&
                visibleProjects.length === 0 && (
                  <p className="pl-notice">
                    No favorites yet. Star a project to pin it here.
                  </p>
                )}
            </div>

            {orphans.length > 0 && (
              <div className="pl-orphans">
                <h4 className="pl-orphans-title">Ungrouped chats</h4>
                <div className="pl-orphans-list">
                  {orphans.map((c) => (
                    <button
                      key={c.id}
                      type="button"
                      className="pl-orphan"
                      onClick={() => onEnter({ id: c.id, title: c.title })}
                    >
                      <Icon name="forum" size={18} />
                      <span className="pl-orphan-title">{c.title}</span>
                      <span className="pl-orphan-meta">
                        {c.turnCount}{" "}
                        {c.turnCount === 1 ? "turn" : "turns"}
                      </span>
                    </button>
                  ))}
                </div>
              </div>
            )}
          </>
        )}

        <div className="pl-bento">
          {HELPERS.map((h) => (
            <div key={h.id} className="pl-helper">
              <span className="pl-helper-tile">
                <Icon name={h.icon} size={22} />
              </span>
              <div className="pl-helper-text">
                <h4 className="pl-helper-label">{h.label}</h4>
                <p className="pl-helper-desc">{h.desc}</p>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
