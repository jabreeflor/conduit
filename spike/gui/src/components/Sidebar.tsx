import { useEffect, useMemo, useState } from "react";
import { BrandHeader } from "./BrandRow";
import { Icon } from "./Icon";
import { THEME_META, type Theme } from "../useTheme";
import { getInfo, getProjects, type ChatSummary, type Project } from "../api";

// View ids the sidebar can navigate to (mirrors App's View union).
export type NavView =
  | "welcome"
  | "chat"
  | "projects"
  | "newProject"
  | "workspace"
  | "agents"
  | "soul"
  | "settings"
  | "plugins"
  | "automations";

const NAV_ITEMS: { view: NavView; icon: string; label: string }[] = [
  { view: "welcome", icon: "add_box", label: "New chat" },
  { view: "projects", icon: "folder_open", label: "Projects" },
  { view: "plugins", icon: "extension", label: "Plugins" },
  { view: "automations", icon: "auto_mode", label: "Automations" },
  { view: "soul", icon: "psychology", label: "Soul" },
  { view: "settings", icon: "settings", label: "Settings" },
];

// Which top-level nav row is highlighted for a given active view (project
// sub-views all light up "Projects").
function navActive(item: NavView, view: NavView): boolean {
  if (item === "projects") {
    return view === "projects" || view === "newProject" || view === "workspace";
  }
  if (item === "welcome") return view === "welcome" || view === "chat";
  return item === view;
}

export function Sidebar({
  activeChatId,
  activeView,
  onNavigate,
  onSelectChat,
  theme,
  onCycleTheme,
  version,
}: {
  activeChatId: string | null;
  activeView: NavView;
  onNavigate: (view: NavView) => void;
  onSelectChat: (id: string) => void;
  theme: Theme;
  onCycleTheme: () => void;
  version: string;
}) {
  const [projects, setProjects] = useState<Project[]>([]);
  const [orphans, setOrphans] = useState<ChatSummary[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [identity, setIdentity] = useState<string>("local session");

  useEffect(() => {
    getProjects()
      .then((res) => {
        setProjects(res.projects);
        setOrphans(res.orphans);
      })
      .catch((e: unknown) =>
        setError(e instanceof Error ? e.message : String(e)),
      );
    getInfo()
      .then((info) => setIdentity(`${info.provider} · ${info.model}`))
      .catch(() => {
        /* offline — keep the placeholder */
      });
  }, []);

  return (
    <aside className="conduit-sidebar" aria-label="Navigation">
      <BrandHeader version={version} />

      <nav className="sb-nav">
        {NAV_ITEMS.map((item) => {
          const active = navActive(item.view, activeView);
          return (
            <button
              key={item.view}
              type="button"
              className={`sb-nav-row ${active ? "active" : ""}`}
              onClick={() => onNavigate(item.view)}
            >
              <Icon name={item.icon} size={20} fill={active} />
              <span className="sb-nav-label">{item.label}</span>
            </button>
          );
        })}
      </nav>

      <div className="sb-scroll">
        <ProjectsSection
          projects={projects}
          activeChatId={activeChatId}
          onSelectChat={onSelectChat}
          onAdd={() => onNavigate("newProject")}
          error={error}
        />
        <OrphanChatsSection
          orphans={orphans}
          activeChatId={activeChatId}
          onSelectChat={onSelectChat}
          onAdd={() => onNavigate("welcome")}
        />
      </div>

      <div className="sb-profile">
        <div className="sb-avatar">YOU</div>
        <div className="sb-profile-meta">
          <div className="sb-profile-name">You</div>
          <div className="sb-profile-plan">{identity}</div>
        </div>
        <button
          type="button"
          className="sb-profile-settings"
          onClick={onCycleTheme}
          aria-label={`Theme: ${THEME_META[theme].label}. Click to switch.`}
          title={`Theme: ${THEME_META[theme].label}`}
        >
          <Icon name={THEME_META[theme].icon} size={20} />
        </button>
        <button
          type="button"
          className="sb-profile-settings"
          onClick={() => onNavigate("settings")}
          aria-label="Settings"
        >
          <Icon name="settings" size={20} />
        </button>
      </div>
    </aside>
  );
}

function ProjectsSection({
  projects,
  activeChatId,
  onSelectChat,
  onAdd,
  error,
}: {
  projects: Project[];
  activeChatId: string | null;
  onSelectChat: (id: string) => void;
  onAdd: () => void;
  error: string | null;
}) {
  return (
    <div className="sb-section">
      <div className="sb-section-header">
        <span className="sb-section-label">Projects</span>
        <span className="sb-section-actions">
          <button
            type="button"
            className="sb-icon-btn"
            onClick={onAdd}
            aria-label="New project"
            title="New project"
          >
            <Icon name="add" size={16} />
          </button>
        </span>
      </div>

      {error && (
        <div className="sb-section-empty">Start `conduit serve` to load</div>
      )}
      {!error && projects.length === 0 && (
        <div className="sb-section-empty">No projects yet</div>
      )}
      {projects.map((p) => (
        <ProjectRow
          key={p.id}
          project={p}
          activeChatId={activeChatId}
          onSelectChat={onSelectChat}
        />
      ))}
    </div>
  );
}

function ProjectRow({
  project,
  activeChatId,
  onSelectChat,
}: {
  project: Project;
  activeChatId: string | null;
  onSelectChat: (id: string) => void;
}) {
  const [open, setOpen] = useState(true);
  return (
    <>
      <button
        type="button"
        className="sb-project"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
      >
        <Icon name="folder" size={18} />
        <span className="sb-project-name">{project.name}</span>
      </button>
      {open && project.chats.length > 0 && (
        <div className="sb-chats">
          {project.chats.map((c) => (
            <ChatRow
              key={c.id}
              chat={c}
              indent={36}
              active={activeChatId === c.id}
              onSelect={onSelectChat}
            />
          ))}
        </div>
      )}
    </>
  );
}

function OrphanChatsSection({
  orphans,
  activeChatId,
  onSelectChat,
  onAdd,
}: {
  orphans: ChatSummary[];
  activeChatId: string | null;
  onSelectChat: (id: string) => void;
  onAdd: () => void;
}) {
  return (
    <div className="sb-section">
      <div className="sb-section-header">
        <span className="sb-section-label">Chats</span>
        <span className="sb-section-actions">
          <button
            type="button"
            className="sb-icon-btn"
            onClick={onAdd}
            aria-label="New chat"
            title="New chat"
          >
            <Icon name="add" size={16} />
          </button>
        </span>
      </div>
      {orphans.length === 0 && (
        <div className="sb-section-empty">No orphan chats</div>
      )}
      <div className="sb-chats">
        {orphans.map((c) => (
          <ChatRow
            key={c.id}
            chat={c}
            indent={14}
            active={activeChatId === c.id}
            onSelect={onSelectChat}
          />
        ))}
      </div>
    </div>
  );
}

function ChatRow({
  chat,
  indent,
  active,
  onSelect,
}: {
  chat: ChatSummary;
  indent: number;
  active: boolean;
  onSelect: (id: string) => void;
}) {
  const ts = useMemo(() => relativeTime(chat.createdAt), [chat.createdAt]);
  return (
    <button
      type="button"
      className={`sb-chat-row ${active ? "active" : ""}`}
      style={{ paddingLeft: indent }}
      onClick={() => onSelect(chat.id)}
      title={chat.title}
    >
      <span className="sb-chat-name">{chat.title}</span>
      <span className="sb-chat-ts">
        <Icon name="schedule" size={11} /> {ts}
      </span>
    </button>
  );
}

// relativeTime collapses an ISO timestamp into the compact h/d/w/mo form.
function relativeTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const diffMs = Date.now() - d.getTime();
  const min = Math.floor(diffMs / 60000);
  if (min < 1) return "now";
  if (min < 60) return `${min}m`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h`;
  const day = Math.floor(hr / 24);
  if (day < 7) return `${day}d`;
  const wk = Math.floor(day / 7);
  if (wk < 5) return `${wk}w`;
  const mo = Math.floor(day / 30);
  if (mo < 12) return `${mo}mo`;
  return `${Math.floor(day / 365)}y`;
}
