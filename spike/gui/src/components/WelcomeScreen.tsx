import { useEffect, useRef, useState } from "react";
import {
  ArrowUp,
  ChevronDown,
  Folder,
  GitBranch,
  Laptop,
  Mic,
  Plus,
  Square,
  SquareSplitHorizontal,
} from "lucide-react";
import { ConduitMark } from "./BrandRow";
import {
  GitHubMark,
  IntegrationCard,
  LinearMark,
  McpMark,
  SlackMark,
} from "./IntegrationCard";

// WelcomeScreen is the empty-state body of <main>. It's shown when no chat is
// selected (activeChatId === null or "new"). The composer's submit fires the
// onStartChat callback, which the parent uses to create a session and route
// to ChatPanel for the rest of the conversation.
export function WelcomeScreen({
  branch,
  onStartChat,
  autoFocus,
}: {
  branch: string;
  onStartChat: (prompt: string) => void;
  autoFocus: boolean;
}) {
  const [draft, setDraft] = useState("");
  const taRef = useRef<HTMLTextAreaElement | null>(null);

  useEffect(() => {
    if (autoFocus) taRef.current?.focus();
  }, [autoFocus]);

  function submit() {
    const text = draft.trim();
    if (!text) return;
    onStartChat(text);
    setDraft("");
  }

  function onKey(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      submit();
    }
  }

  return (
    <div className="welcome">
      <div className="welcome-top-actions" aria-hidden>
        <button type="button" className="ghost-icon-btn" title="Split window">
          <SquareSplitHorizontal size={16} />
        </button>
        <button type="button" className="ghost-icon-btn" title="New window">
          <Square size={16} />
        </button>
      </div>

      <div className="welcome-center">
        <ConduitMark size={44} className="welcome-mark" idSuffix="hero" />
        <h1 className="welcome-headline">What should we build in conduit?</h1>

        <div className="composer-wrap">
          <div className="composer">
            <textarea
              ref={taRef}
              className="composer-input"
              placeholder="Ask Conduit anything. @ to mention files or tools"
              rows={3}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={onKey}
            />
            <div className="composer-bar">
              <div className="composer-bar-left">
                <button
                  type="button"
                  className="composer-icon"
                  title="Attach"
                  aria-label="Attach"
                >
                  <Plus size={16} />
                </button>
                <span className="pill pill-sandbox">
                  <span className="pill-dot" />
                  Sandboxed
                  <ChevronDown size={10} className="pill-chev" />
                </span>
                <span className="pill pill-model">
                  gpt-5.5 Medium
                  <ChevronDown size={10} className="pill-chev" />
                </span>
              </div>
              <div className="composer-bar-right">
                <button
                  type="button"
                  className="composer-icon"
                  title="Voice"
                  aria-label="Voice"
                >
                  <Mic size={15} />
                </button>
                <button
                  type="button"
                  className="composer-send"
                  onClick={submit}
                  disabled={draft.trim().length === 0}
                  aria-label="Send"
                  title="Send"
                >
                  <ArrowUp size={14} strokeWidth={2.4} />
                </button>
              </div>
            </div>
          </div>

          <div className="context-row">
            <span className="ctx-pill">
              <Folder size={13} />
              conduit
              <ChevronDown size={10} className="pill-chev" />
            </span>
            <span className="ctx-pill">
              <Laptop size={13} />
              Work locally
              <ChevronDown size={10} className="pill-chev" />
            </span>
            <span className="ctx-pill">
              <GitBranch size={13} />
              {branch || "main"}
              <ChevronDown size={10} className="pill-chev" />
            </span>
          </div>
        </div>
      </div>

      <div className="integration-cards">
        <IntegrationCard
          icon={<GitHubMark />}
          title="Connect GitHub"
          body="Pull issues, branches, and PR context"
        />
        <IntegrationCard
          icon={<LinearMark />}
          title="Connect Linear"
          body="Plan from your team's backlog"
        />
        <IntegrationCard
          icon={<McpMark />}
          title="Connect MCP"
          body="Plug in external tools and data"
        />
        <IntegrationCard
          icon={<SlackMark />}
          title="Connect Slack"
          body="Pull context from team threads"
        />
      </div>
    </div>
  );
}
