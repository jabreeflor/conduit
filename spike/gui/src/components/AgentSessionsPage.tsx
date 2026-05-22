import { useEffect, useRef, useState } from "react";
import {
  getAgents,
  createAgent,
  type AgentTemplate,
} from "../api";
import { Icon } from "./Icon";

// AgentSessionsPage shows the built-in template picker and routes the
// selected template into a new chat session via onStartSession.
export function AgentSessionsPage({
  onStartSession,
  onOpenAutomations,
}: {
  onStartSession: (templateId: string | null) => void;
  onOpenAutomations: () => void;
}) {
  const [templates, setTemplates] = useState<AgentTemplate[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [showCustomModal, setShowCustomModal] = useState(false);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    setLoading(true);
    getAgents()
      .then(setTemplates)
      .catch(() => setTemplates(builtinFallback))
      .finally(() => setLoading(false));
  }, []);

  function handleStart() {
    onStartSession(selected);
  }

  function handleCustomSaved(t: AgentTemplate) {
    setTemplates((prev) => [...prev, t]);
    setSelected(t.id);
    setShowCustomModal(false);
  }

  return (
    <div className="agent-sessions-page">
      <div className="agent-sessions-header">
        <div className="agent-sessions-title-row">
          <Icon name="smart_toy" size={22} fill />
          <h1 className="agent-sessions-title">Agent Sessions</h1>
        </div>
        <div className="agent-sessions-actions">
          <button
            type="button"
            className="agents-btn agents-btn-primary"
            onClick={() => setShowCustomModal(true)}
          >
            <Icon name="person_add" size={16} />
            Custom Agent
          </button>
          <button
            type="button"
            className="agents-btn agents-btn-secondary"
            onClick={onOpenAutomations}
          >
            Automation
          </button>
        </div>
      </div>

      {loading ? (
        <div className="agent-sessions-loading">Loading templates…</div>
      ) : (
        <div className="agent-sessions-grid">
          {templates.map((t) => (
            <TemplateCard
              key={t.id}
              template={t}
              selected={selected === t.id}
              onSelect={() => setSelected((s) => (s === t.id ? null : t.id))}
            />
          ))}
        </div>
      )}

      {selected && (
        <div className="agent-sessions-start-row">
          <button
            type="button"
            className="agents-btn agents-btn-start"
            onClick={handleStart}
          >
            Start Session
            <Icon name="arrow_forward" size={16} />
          </button>
        </div>
      )}

      {showCustomModal && (
        <CustomAgentModal
          onSave={handleCustomSaved}
          onClose={() => setShowCustomModal(false)}
        />
      )}
    </div>
  );
}

// ── Template card ──────────────────────────────────────────────────────────

function TemplateCard({
  template,
  selected,
  onSelect,
}: {
  template: AgentTemplate;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      className={`agent-card${selected ? " agent-card--selected" : ""}`}
      onClick={onSelect}
      aria-pressed={selected}
    >
      <div className="agent-card-top">
        <TemplateIcon name={template.icon} />
        <span className={`agent-card-radio${selected ? " agent-card-radio--checked" : ""}`} aria-hidden />
      </div>
      <p className="agent-card-name">{template.name}</p>
      <p className="agent-card-desc">{template.description}</p>
    </button>
  );
}

function TemplateIcon({ name }: { name: string }) {
  const iconMap: Record<string, string> = {
    code: "code",
    description: "description",
    architecture: "architecture",
    assignment: "assignment",
    security: "security",
    brush: "edit",
    person: "person",
  };
  return <Icon name={iconMap[name] ?? name} size={22} />;
}

// ── Custom agent modal ─────────────────────────────────────────────────────

function CustomAgentModal({
  onSave,
  onClose,
}: {
  onSave: (t: AgentTemplate) => void;
  onClose: () => void;
}) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [systemPrompt, setSystemPrompt] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const nameRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    nameRef.current?.focus();
  }, []);

  async function handleSave() {
    if (!name.trim()) {
      setError("Name is required.");
      return;
    }
    if (!systemPrompt.trim()) {
      setError("System prompt is required.");
      return;
    }
    setSaving(true);
    setError(null);
    try {
      const saved = await createAgent({ name: name.trim(), description: description.trim(), systemPrompt: systemPrompt.trim() });
      onSave(saved);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Save failed.");
    } finally {
      setSaving(false);
    }
  }

  function onKey(e: React.KeyboardEvent) {
    if (e.key === "Escape") onClose();
  }

  return (
    <div className="modal-overlay" role="dialog" aria-modal aria-label="Create custom agent" onKeyDown={onKey}>
      <div className="modal-panel">
        <div className="modal-header">
          <h2 className="modal-title">Custom Agent</h2>
          <button type="button" className="modal-close" onClick={onClose} aria-label="Close">
            <Icon name="close" size={18} />
          </button>
        </div>

        <div className="modal-body">
          <label className="modal-label" htmlFor="agent-name">Name</label>
          <input
            ref={nameRef}
            id="agent-name"
            className="modal-input"
            type="text"
            placeholder="e.g. Data Analyst"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />

          <label className="modal-label" htmlFor="agent-desc">Description</label>
          <input
            id="agent-desc"
            className="modal-input"
            type="text"
            placeholder="One sentence about what this agent does"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />

          <label className="modal-label" htmlFor="agent-prompt">
            System Prompt
            <span className="modal-label-hint"> — defines the agent's persona and behavior</span>
          </label>
          <textarea
            id="agent-prompt"
            className="modal-textarea"
            rows={8}
            placeholder={"You are a Data Analyst. Your job is to…\n\nBe concise. Lead with findings, not methodology."}
            value={systemPrompt}
            onChange={(e) => setSystemPrompt(e.target.value)}
          />

          {error && <p className="modal-error">{error}</p>}
        </div>

        <div className="modal-footer">
          <button type="button" className="agents-btn agents-btn-secondary" onClick={onClose}>
            Cancel
          </button>
          <button
            type="button"
            className="agents-btn agents-btn-primary"
            onClick={handleSave}
            disabled={saving}
          >
            {saving ? "Saving…" : "Save Agent"}
          </button>
        </div>
      </div>
    </div>
  );
}

// Fallback shown when the backend is unreachable — mirrors the 6 built-ins.
const builtinFallback: AgentTemplate[] = [
  { id: "code-auditor", name: "Code Auditor", description: "Reviews PRs for security and style vulnerabilities.", icon: "code", isBuiltIn: true },
  { id: "content-strategist", name: "Content Strategist", description: "Drafts and refines technical copy and documentation.", icon: "description", isBuiltIn: true },
  { id: "system-architect", name: "System Architect", description: "Designs scalable infrastructure and API schemas.", icon: "architecture", isBuiltIn: true },
  { id: "product-manager", name: "Product Manager", description: "Synthesizes market data and user feedback into actionable PRDs and roadmaps.", icon: "assignment", isBuiltIn: true },
  { id: "security-researcher", name: "Security Researcher", description: "Performs deep audits of code and architecture for vulnerabilities and compliance.", icon: "security", isBuiltIn: true },
  { id: "ui-ux-critic", name: "UI/UX Critic", description: "Provides expert feedback on design flows, accessibility, and visual hierarchy.", icon: "brush", isBuiltIn: true },
];
