import { useEffect, useState } from "react";
import { Spotlight } from "./Spotlight";
import { ChatPanel } from "./components/ChatPanel";
import { SessionList } from "./components/SessionList";
import { MemoryView } from "./components/MemoryView";

// SidebarTab mirrors the iota in internal/gui/layout.go.
type SidebarTab = "sessions" | "workflows" | "memory" | "skills" | "evals";

// MainView mirrors internal/gui/layout.go MainView.
type MainView = "screenshot" | "canvas" | "workflow" | "memory" | "diff" | "evals";

const TABS: { id: SidebarTab; label: string; view: MainView }[] = [
  { id: "sessions", label: "Sessions", view: "screenshot" },
  { id: "workflows", label: "Workflows", view: "workflow" },
  { id: "memory", label: "Memory", view: "memory" },
  { id: "skills", label: "Skills", view: "screenshot" },
  { id: "evals", label: "Evals", view: "evals" },
];

export function App() {
  const [activeTab, setActiveTab] = useState<SidebarTab>("sessions");
  const [mainView, setMainView] = useState<MainView>("screenshot");
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
      <ChatPanel />
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
      <div className="sidebar-header">Conduit</div>
      <nav>
        {TABS.map((t) => (
          <button
            key={t.id}
            className={`sidebar-tab ${activeTab === t.id ? "active" : ""}`}
            onClick={() => onSelect(t.id)}
          >
            {t.label}
          </button>
        ))}
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
  return (
    <main className="main" aria-label="Main content">
      <div className="main-header">{viewTitle(view)}</div>
      <div className="main-body">{viewBody(view, activeTab)}</div>
    </main>
  );
}

function viewTitle(v: MainView): string {
  switch (v) {
    case "screenshot":
      return "Computer use — live screenshots";
    case "canvas":
      return "Canvas";
    case "workflow":
      return "Workflow DAG";
    case "memory":
      return "Memory — SOUL.md / USER.md";
    case "diff":
      return "Diff review";
    case "evals":
      return "Evals — scorecards";
  }
}

function viewBody(v: MainView, activeTab: SidebarTab) {
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
  if (activeTab === "sessions") {
    return (
      <div className="placeholder">
        <p>Pick a session in the sidebar to view its screenshot stream.</p>
        <p>View-model: <code>internal/gui/screenshot_stream.go</code></p>
      </div>
    );
  }
  return (
    <div className="placeholder">
      <p>Screenshot stream — already wired in <code>internal/gui/screenshot_stream.go</code>.</p>
    </div>
  );
}
