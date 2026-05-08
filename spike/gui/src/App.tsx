import { useEffect, useState } from "react";
import {
  Brain,
  Gauge,
  History,
  MessagesSquare,
  Sparkles,
  Workflow,
  type LucideIcon,
} from "lucide-react";
import { Spotlight } from "./Spotlight";
import { ChatPanel } from "./components/ChatPanel";
import { SessionList } from "./components/SessionList";
import { MemoryView } from "./components/MemoryView";
import logoUrl from "../../../assets/logo.svg";

type SidebarTab = "chat" | "sessions" | "workflows" | "memory" | "skills" | "evals";

type MainView = "chat" | "sessions" | "workflow" | "memory" | "evals";

const TABS: { id: SidebarTab; label: string; view: MainView; icon: LucideIcon }[] = [
  { id: "chat", label: "Chat", view: "chat", icon: MessagesSquare },
  { id: "sessions", label: "Sessions", view: "sessions", icon: History },
  { id: "workflows", label: "Workflows", view: "workflow", icon: Workflow },
  { id: "memory", label: "Memory", view: "memory", icon: Brain },
  { id: "skills", label: "Skills", view: "chat", icon: Sparkles },
  { id: "evals", label: "Evals", view: "evals", icon: Gauge },
];

export function App() {
  const [activeTab, setActiveTab] = useState<SidebarTab>("chat");
  const [mainView, setMainView] = useState<MainView>("chat");
  const [spotlightOpen, setSpotlightOpen] = useState(false);

  // Global hotkey: ⌥Space (the production binding) AND ⌘K (dev convenience
  // because most browsers swallow ⌥Space for IME composition).
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const isSpotlight =
        (e.altKey && e.code === "Space") || (e.metaKey && e.key === "k");
      if (isSpotlight) {
        e.preventDefault();
        setSpotlightOpen((v) => !v);
      } else if (e.key === "Escape") {
        setSpotlightOpen(false);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  function selectTab(id: SidebarTab) {
    setActiveTab(id);
    const tab = TABS.find((t) => t.id === id);
    if (tab) setMainView(tab.view);
  }

  return (
    <div className="app">
      <Sidebar activeTab={activeTab} onSelect={selectTab} />
      <Main view={mainView} activeTab={activeTab} />
      {spotlightOpen && (
        <Spotlight onClose={() => setSpotlightOpen(false)} />
      )}
    </div>
  );
}

function Sidebar({
  activeTab,
  onSelect,
}: {
  activeTab: SidebarTab;
  onSelect: (id: SidebarTab) => void;
}) {
  return (
    <aside className="sidebar" aria-label="Navigation">
      <div className="sidebar-header">
        <img src={logoUrl} alt="" className="sidebar-logo" />
        <span>Conduit</span>
      </div>
      <nav>
        {TABS.map((t) => {
          const Icon = t.icon;
          return (
            <button
              key={t.id}
              className={`sidebar-tab ${activeTab === t.id ? "active" : ""}`}
              onClick={() => onSelect(t.id)}
            >
              <Icon size={16} className="sidebar-tab-icon" aria-hidden />
              <span>{t.label}</span>
            </button>
          );
        })}
      </nav>
      {activeTab === "sessions" && (
        <div className="sidebar-section">
          <SessionList />
        </div>
      )}
      <div className="sidebar-footer">
        <kbd>⌥Space</kbd> Spotlight · <kbd>⌘K</kbd> Palette
      </div>
    </aside>
  );
}

function Main({ view, activeTab }: { view: MainView; activeTab: SidebarTab }) {
  // Chat is the primary surface — render it edge-to-edge in main without a
  // header, since ChatPanel has its own header with provider/model badge.
  if (view === "chat") {
    return (
      <main className="main main-chat" aria-label="Chat">
        <ChatPanel />
      </main>
    );
  }
  return (
    <main className="main" aria-label="Main content">
      <div className="main-header">
        {view === "memory" && (
          <Brain size={16} className="main-header-icon" aria-hidden />
        )}
        <span>{viewTitle(view)}</span>
      </div>
      <div className="main-body">{viewBody(view, activeTab)}</div>
    </main>
  );
}

function viewTitle(v: MainView): string {
  switch (v) {
    case "chat":
      return "Chat";
    case "sessions":
      return "Sessions";
    case "workflow":
      return "Workflow DAG";
    case "memory":
      return "Memory — SOUL.md / USER.md";
    case "evals":
      return "Evals — scorecards";
  }
}

function viewBody(v: MainView, _activeTab: SidebarTab) {
  if (v === "memory") {
    return <MemoryView />;
  }
  if (v === "evals") {
    return (
      <div className="placeholder">
        <p>Per-model scorecards + trend lines.</p>
        <p>View-model: <code>internal/gui/evals_view.go</code></p>
      </div>
    );
  }
  if (v === "workflow") {
    return (
      <div className="placeholder">
        <p>Workflow DAG renderer — already implemented in <code>internal/gui/workflow_dag.go</code>.</p>
      </div>
    );
  }
  if (v === "sessions") {
    return (
      <div className="placeholder">
        <p>Pick a session in the sidebar to view its turns.</p>
        <p>View-model: <code>internal/gui/session_tree.go</code></p>
      </div>
    );
  }
  return null;
}
