import { useEffect, useMemo, useState } from "react";
import {
  Clock,
  Filter,
  Folder,
  MessageSquarePlus,
  Plus,
  Search,
  Workflow,
  Wrench,
} from "lucide-react";
import { BrandRow } from "./BrandRow";
import { getProjects, type ChatSummary, type Project } from "../api";

// Sidebar is the permanent left rail — chrome + brand + nav + projects + chats.
// It owns the projects fetch (one shot on mount) and lifts chat selection up
// to App via onSelectChat / onNewChat.
export function Sidebar({
  activeChatId,
  onSelectChat,
  onNewChat,
  version,
}: {
  activeChatId: string | null;
  onSelectChat: (id: string) => void;
  onNewChat: () => void;
  version: string;
}) {
  const [projects, setProjects] = useState<Project[]>([]);
  const [orphans, setOrphans] = useState<ChatSummary[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getProjects()
      .then((res) => {
        setProjects(res.projects);
        setOrphans(res.orphans);
      })
      .catch((e: unknown) =>
        setError(e instanceof Error ? e.message : String(e)),
      );
  }, []);

  return (
    <aside className="conduit-sidebar" aria-label="Navigation">
      <BrandRow version={version} />

      <nav className="sb-nav">
        <button
          type="button"
          className={`sb-nav-row ${activeChatId === null || activeChatId === "new" ? "active" : ""}`}
          onClick={onNewChat}
        >
          <MessageSquarePlus size={16} aria-hidden />
          <span className="sb-nav-label">New chat</span>
        </button>
        <button type="button" className="sb-nav-row">
          <Search size={16} aria-hidden />
          <span className="sb-nav-label">Search</span>
        </button>
        <button type="button" className="sb-nav-row">
          <Wrench size={16} aria-hidden />
          <span className="sb-nav-label">Plugins</span>
        </button>
        <button type="button" className="sb-nav-row">
          <Workflow size={16} aria-hidden />
          <span className="sb-nav-label">Automations</span>
        </button>
      </nav>

      <div className="sb-scroll">
        <ProjectsSection
          projects={projects}
          activeChatId={activeChatId}
          onSelectChat={onSelectChat}
          error={error}
        />
        <OrphanChatsSection
          orphans={orphans}
          activeChatId={activeChatId}
          onSelectChat={onSelectChat}
        />
      </div>

      <div className="sb-footer">
        <kbd>⌥Space</kbd> Spotlight · <kbd>⌘K</kbd> Palette
      </div>
    </aside>
  );
}

function ProjectsSection({
  projects,
  activeChatId,
  onSelectChat,
  error,
}: {
  projects: Project[];
  activeChatId: string | null;
  onSelectChat: (id: string) => void;
  error: string | null;
}) {
  return (
    <div className="sb-section">
      <div className="sb-section-header">
        <span className="sb-section-label">Projects</span>
        <span className="sb-section-actions">
          <span className="sb-icon-btn" aria-hidden>
            <Plus size={13} />
          </span>
        </span>
      </div>

      {error && <div className="sb-section-empty">Failed: {error}</div>}
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
  // Project rows expand to show their chats by default — collapse is local-only.
  const [open, setOpen] = useState(true);
  return (
    <>
      <button
        type="button"
        className="sb-project"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
      >
        <Folder size={14} aria-hidden />
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
}: {
  orphans: ChatSummary[];
  activeChatId: string | null;
  onSelectChat: (id: string) => void;
}) {
  return (
    <div className="sb-section">
      <div className="sb-section-header">
        <span className="sb-section-label">Chats</span>
        <span className="sb-section-actions">
          <span className="sb-icon-btn" aria-hidden>
            <Filter size={13} />
          </span>
          <span className="sb-icon-btn" aria-hidden>
            <Plus size={13} />
          </span>
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
        <Clock size={10} aria-hidden /> {ts}
      </span>
    </button>
  );
}

// relativeTime collapses an ISO timestamp into the compact h/d/w/mo form the
// design uses — same vocabulary as iMessage / Slack.
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
