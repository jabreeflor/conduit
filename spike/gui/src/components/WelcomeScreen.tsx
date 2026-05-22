import { useEffect, useRef, useState } from "react";
import { Icon } from "./Icon";
import {
  GitHubMark,
  IntegrationCard,
  LinearMark,
  SlackMark,
} from "./IntegrationCard";

// WelcomeScreen is the empty-state body of the content column. Shown when no
// chat is selected. The composer's submit fires onStartChat, which the parent
// uses to create a session and route to ChatPanel.
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
  const [sandbox, setSandbox] = useState("Sandboxed");
  const [model, setModel] = useState("gpt-5.5 Medium");
  const [menu, setMenu] = useState<null | "sandbox" | "model">(null);
  const taRef = useRef<HTMLTextAreaElement | null>(null);

  const SANDBOX_OPTS = ["Sandboxed", "Unrestricted"];
  const MODEL_OPTS = [
    "gpt-5.5 Medium",
    "gpt-5.5 High",
    "claude-opus-4-7",
    "local",
  ];

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
      <div className="welcome-glow" aria-hidden />

      <div className="welcome-center">
        <div className="welcome-mark" aria-hidden>
          <Icon name="terminal" size={40} fill />
        </div>
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
                  <Icon name="add" size={20} />
                </button>
                <span className="pill-wrap">
                  <button
                    type="button"
                    className="pill pill-sandbox"
                    aria-haspopup="listbox"
                    aria-expanded={menu === "sandbox"}
                    onClick={() =>
                      setMenu((m) => (m === "sandbox" ? null : "sandbox"))
                    }
                  >
                    <span className="pill-dot" />
                    {sandbox}
                    <Icon name="expand_more" size={14} className="pill-chev" />
                  </button>
                  {menu === "sandbox" && (
                    <ul className="composer-menu" role="listbox">
                      {SANDBOX_OPTS.map((opt) => (
                        <li key={opt} role="option" aria-selected={opt === sandbox}>
                          <button
                            type="button"
                            className={`composer-menu-item${opt === sandbox ? " active" : ""}`}
                            onClick={() => {
                              setSandbox(opt);
                              setMenu(null);
                            }}
                          >
                            {opt}
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                </span>
                <span className="pill-wrap">
                  <button
                    type="button"
                    className="pill pill-model"
                    aria-haspopup="listbox"
                    aria-expanded={menu === "model"}
                    onClick={() =>
                      setMenu((m) => (m === "model" ? null : "model"))
                    }
                  >
                    {model}
                    <Icon name="expand_more" size={14} className="pill-chev" />
                  </button>
                  {menu === "model" && (
                    <ul className="composer-menu" role="listbox">
                      {MODEL_OPTS.map((opt) => (
                        <li key={opt} role="option" aria-selected={opt === model}>
                          <button
                            type="button"
                            className={`composer-menu-item${opt === model ? " active" : ""}`}
                            onClick={() => {
                              setModel(opt);
                              setMenu(null);
                            }}
                          >
                            {opt}
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                </span>
              </div>
              <div className="composer-bar-right">
                <button
                  type="button"
                  className="composer-icon"
                  title="Voice"
                  aria-label="Voice"
                >
                  <Icon name="mic" size={18} />
                </button>
                <button
                  type="button"
                  className="composer-send"
                  onClick={submit}
                  disabled={draft.trim().length === 0}
                  aria-label="Send"
                  title="Send"
                >
                  <Icon name="arrow_upward" size={20} />
                </button>
              </div>
            </div>
          </div>

          <div className="context-row">
            <span className="ctx-pill">
              <Icon name="folder_open" size={14} />
              conduit
              <Icon name="expand_more" size={14} className="pill-chev" />
            </span>
            <span className="ctx-pill">
              <Icon name="computer" size={14} />
              Work locally
              <Icon name="expand_more" size={14} className="pill-chev" />
            </span>
            <span className="ctx-pill">
              <Icon name="account_tree" size={14} />
              {branch || "main"}
              <Icon name="expand_more" size={14} className="pill-chev" />
            </span>
          </div>
        </div>
      </div>

      <div className="integration-cards">
        <IntegrationCard
          icon={<GitHubMark />}
          title="Connect GitHub"
          body="Pull issues, branches, and PR context directly into chat."
        />
        <IntegrationCard
          icon={<LinearMark />}
          title="Connect Linear"
          body="Plan from your team's backlog and update statuses."
        />
        <IntegrationCard
          tile
          icon={<Icon name="hub" size={24} fill />}
          title="Connect MCP"
          body="Plug in external tools and data via Protocol."
        />
        <IntegrationCard
          icon={<SlackMark />}
          title="Connect Slack"
          body="Pull context from team threads and share updates."
        />
      </div>
    </div>
  );
}
