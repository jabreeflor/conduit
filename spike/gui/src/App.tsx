import { useEffect, useState } from "react";
import { Spotlight } from "./Spotlight";
import { AgentSessionsPage } from "./components/AgentSessionsPage";
import "./components/AgentSessionsPage.css";
import { ChatPanel } from "./components/ChatPanel";
import { NewProject, type NewProjectDraft } from "./components/NewProject";
import { ProjectDashboard } from "./components/ProjectDashboard";
import { ProjectsList } from "./components/ProjectsList";
import { Sidebar, type NavView } from "./components/Sidebar";
import { SoulPage } from "./components/SoulPage";
import { SoulPanel } from "./components/SoulPanel";
import { SettingsPage } from "./components/SettingsPage";
import { PluginsPage } from "./components/PluginsPage";
import { AutomationsPage } from "./components/AutomationsPage";
import { TeamActivity } from "./components/TeamActivity";
import { TopBar } from "./components/TopBar";
import { WelcomeScreen } from "./components/WelcomeScreen";
import { useTheme } from "./useTheme";
import { addLocalProject } from "./localProjects";

type View = NavView;

// `?demo=` seeds a view for design QA without a live backend.
const demoParam =
  typeof window !== "undefined"
    ? new URLSearchParams(window.location.search).get("demo")
    : null;

function initialView(): View {
  switch (demoParam) {
    case "chat":
    case "spotlight":
      return "chat";
    case "projects":
      return "projects";
    case "new-project":
      return "newProject";
    case "workspace":
      return "workspace";
    case "soul":
      return "soul";
    case "settings":
      return "settings";
    case "plugins":
      return "plugins";
    case "automations":
      return "automations";
    case "agents":
      return "agents";
    default:
      return "welcome";
  }
}

const TOPBAR_LABEL: Record<View, string> = {
  welcome: "Conduit",
  chat: "Chat",
  projects: "Projects",
  newProject: "New Project",
  workspace: "Workspace",
  agents: "Agent Sessions",
  soul: "Soul",
  settings: "Settings",
  plugins: "Plugins",
  automations: "Automations",
};

export function App() {
  // A simple back/forward history stack over top-level views.
  const [stack, setStack] = useState<View[]>([initialView()]);
  const [pos, setPos] = useState(0);
  const view = stack[pos];

  const [activeChatId, setActiveChatId] = useState<string | null>(
    demoParam === "chat" || demoParam === "spotlight" ? "demo" : null,
  );
  const [pendingPrompt, setPendingPrompt] = useState<string | null>(null);
  const [pendingTemplateId, setPendingTemplateId] = useState<string | null>(null);
  const [spotlightOpen, setSpotlightOpen] = useState(demoParam === "spotlight");
  const [spotlightQuery, setSpotlightQuery] = useState("");
  const [activeProject, setActiveProject] = useState<{
    id: string;
    title: string;
  } | null>(null);
  const { theme, setTheme, cycleTheme } = useTheme();

  function navigate(v: View) {
    if (v === stack[pos]) return;
    setStack((s) => [...s.slice(0, pos + 1), v]);
    setPos((p) => p + 1);
  }
  const canBack = pos > 0;
  const canForward = pos < stack.length - 1;

  // Sidebar/Spotlight entry to a view, resetting chat state for "welcome".
  function go(v: View) {
    if (v === "welcome") {
      setPendingPrompt(null);
      setActiveChatId("new");
    }
    navigate(v);
  }

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

  function handleStartChat(prompt: string) {
    setPendingPrompt(prompt);
    setActiveChatId("active");
    navigate("chat");
  }

  // Called from AgentSessionsPage when the user picks a template and clicks
  // "Start Session". templateId null means a bare chat with no persona.
  function handleStartAgentSession(templateId: string | null) {
    setPendingTemplateId(templateId);
    setPendingPrompt(null);
    setActiveChatId("active");
    navigate("chat");
  }

  function handleSelectChat(id: string) {
    setPendingPrompt(null);
    setActiveChatId(id);
    navigate("chat");
  }

  function handleCreateProject(draft: NewProjectDraft) {
    addLocalProject(draft);
    navigate("projects");
  }

  function handleSpotlightAction(id: string) {
    if (id.startsWith("ses:")) {
      go("welcome");
      return;
    }
    switch (id) {
      case "nav:new-chat":
        go("welcome");
        break;
      case "nav:projects":
        navigate("projects");
        break;
      case "nav:new-project":
        navigate("newProject");
        break;
      case "nav:soul":
        navigate("soul");
        break;
      case "nav:settings":
        navigate("settings");
        break;
      case "act:cycle-theme":
        cycleTheme();
        break;
    }
  }

  function openSearch(query: string) {
    setSpotlightQuery(query);
    setSpotlightOpen(true);
  }

  const projectsActive =
    view === "projects" || view === "newProject" || view === "workspace";

  return (
    <div className="app-shell">
      <Sidebar
        activeChatId={activeChatId}
        activeView={view}
        onNavigate={go}
        onSelectChat={handleSelectChat}
        theme={theme}
        onCycleTheme={cycleTheme}
        version="0.4.2"
      />
      <main className="app-main">
        <div className="content-column">
          <TopBar
            label={TOPBAR_LABEL[view]}
            searchPlaceholder={
              projectsActive
                ? "Search systems or workspaces…"
                : view === "welcome"
                  ? "Search resources or actions…"
                  : "Search conversations…"
            }
            onSearch={openSearch}
            onBack={() => setPos((p) => Math.max(0, p - 1))}
            onForward={() => setPos((p) => Math.min(stack.length - 1, p + 1))}
            canBack={canBack}
            canForward={canForward}
          />
          {view === "welcome" && (
            <WelcomeScreen
              branch="feat/projects-shell"
              onStartChat={handleStartChat}
              onBrowseProjects={() => navigate("projects")}
              autoFocus={activeChatId === "new"}
            />
          )}
          {view === "chat" && (
            <ChatPanel
              key={activeChatId}
              initialPrompt={pendingPrompt}
              templateId={pendingTemplateId}
              demo={activeChatId === "demo"}
            />
          )}
          {view === "projects" && (
            <ProjectsList
              onCreateNew={() => navigate("newProject")}
              onEnter={(project) => {
                setActiveProject(project);
                navigate("workspace");
              }}
            />
          )}
          {view === "newProject" && (
            <NewProject
              onCancel={() => navigate("projects")}
              onCreate={handleCreateProject}
            />
          )}
          {view === "workspace" && (
            <ProjectDashboard
              projectId={activeProject?.id}
              projectName={activeProject?.title}
              onStartSession={() => go("welcome")}
              onBack={() => navigate("projects")}
            />
          )}
          {view === "agents" && (
            <AgentSessionsPage
              onStartSession={handleStartAgentSession}
              onOpenAutomations={() => navigate("automations")}
            />
          )}
          {view === "soul" && <SoulPage />}
          {view === "settings" && (
            <SettingsPage theme={theme} onSetTheme={setTheme} />
          )}
          {view === "plugins" && <PluginsPage />}
          {view === "automations" && <AutomationsPage />}
        </div>
        {view === "welcome" && <TeamActivity />}
        {view === "chat" && <SoulPanel />}
      </main>
      {spotlightOpen && (
        <Spotlight
          initialQuery={spotlightQuery}
          onAction={handleSpotlightAction}
          onClose={() => setSpotlightOpen(false)}
        />
      )}
    </div>
  );
}
