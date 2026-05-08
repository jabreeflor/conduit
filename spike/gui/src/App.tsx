import { useEffect, useState } from "react";
import { Spotlight } from "./Spotlight";
import { ChatPanel } from "./components/ChatPanel";
import { Sidebar } from "./components/Sidebar";
import { WelcomeScreen } from "./components/WelcomeScreen";

// Top-level shell: sidebar + main. activeChatId === null routes to the
// welcome screen; "new" focuses the welcome composer; any other id renders
// the live ChatPanel keyed on that id (so a fresh WS connect happens per
// session — full history loading is SUP-30, out of scope here).
export function App() {
  const [activeChatId, setActiveChatId] = useState<string | null>(null);
  const [pendingPrompt, setPendingPrompt] = useState<string | null>(null);
  const [spotlightOpen, setSpotlightOpen] = useState(false);

  // Global hotkey: ⌥Space (production) + ⌘K (dev convenience because most
  // browsers swallow ⌥Space for IME composition).
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

  function handleNewChat() {
    setPendingPrompt(null);
    setActiveChatId("new");
  }

  function handleStartChat(prompt: string) {
    setPendingPrompt(prompt);
    setActiveChatId("active");
  }

  function handleSelectChat(id: string) {
    setPendingPrompt(null);
    setActiveChatId(id);
  }

  // Welcome shows whenever the user hasn't chosen a chat yet OR pressed
  // "New chat" but hasn't typed anything. ChatPanel takes over once a
  // prompt is in flight (activeChatId === "active") or a session is open.
  const showWelcome = activeChatId === null || activeChatId === "new";

  return (
    <div className="app-shell">
      <Sidebar
        activeChatId={activeChatId}
        onSelectChat={handleSelectChat}
        onNewChat={handleNewChat}
        version="0.4.2"
      />
      <main className="app-main">
        {showWelcome ? (
          <WelcomeScreen
            branch="feat/projects-shell"
            onStartChat={handleStartChat}
            autoFocus={activeChatId === "new"}
          />
        ) : (
          <ChatPanel key={activeChatId} initialPrompt={pendingPrompt} />
        )}
      </main>
      {spotlightOpen && <Spotlight onClose={() => setSpotlightOpen(false)} />}
    </div>
  );
}
