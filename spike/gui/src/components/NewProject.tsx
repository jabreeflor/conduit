import { useState } from "react";
import { Icon } from "./Icon";
import "./NewProject.css";

// NewProject is the "Create New Project" form page for the Conduit GUI. It's a
// UI spike: all selections live in local state and aren't persisted. The
// component renders inside the app content column (the sidebar + top bar are
// owned by the shell), so it only paints the scrollable form body.

type InfraChoice = "local" | "cloud";
type Visibility = "private" | "team" | "public";

const INFRA_OPTIONS: {
  id: InfraChoice;
  icon: string;
  title: string;
  body: string;
  disabled?: boolean;
}[] = [
  {
    id: "local",
    icon: "computer",
    title: "Local-First",
    body: "Secure local storage & compute. Data never leaves your machine.",
  },
  {
    id: "cloud",
    icon: "cloud",
    title: "Cloud-Native",
    body: "High-performance archival & remote agent scaling.",
    disabled: true,
  },
];

type StagedAsset = { id: string; icon: string; name: string; meta: string };

const AGENTS: { id: string; icon: string; title: string; body: string }[] = [
  {
    id: "code-auditor",
    icon: "code",
    title: "Code Auditor",
    body: "Reviews PRs for security and style vulnerabilities.",
  },
  {
    id: "content-strategist",
    icon: "description",
    title: "Content Strategist",
    body: "Drafts and refines technical copy and documentation.",
  },
  {
    id: "system-architect",
    icon: "architecture",
    title: "System Architect",
    body: "Designs scalable infrastructure and API schemas.",
  },
  {
    id: "product-manager",
    icon: "assignment",
    title: "Product Manager",
    body: "Synthesizes market data and user feedback into actionable PRDs and roadmaps.",
  },
  {
    id: "security-researcher",
    icon: "security",
    title: "Security Researcher",
    body: "Performs deep audits of code and architecture for vulnerabilities and compliance.",
  },
  {
    id: "ui-ux-critic",
    icon: "brush",
    title: "UI/UX Critic",
    body: "Provides expert feedback on design flows, accessibility, and visual hierarchy.",
  },
];

const VISIBILITY_OPTIONS: {
  id: Visibility;
  icon: string;
  title: string;
  body: string;
}[] = [
  {
    id: "private",
    icon: "lock",
    title: "Private",
    body: "Just you and invited guests",
  },
  {
    id: "team",
    icon: "group",
    title: "Team",
    body: "Visible to entire workspace",
  },
  {
    id: "public",
    icon: "public",
    title: "Public",
    body: "Published as Open Intelligence",
  },
];

export type NewProjectDraft = {
  name: string;
  description: string;
  infra: InfraChoice;
  visibility: Visibility;
};

export function NewProject({
  onCancel,
  onCreate,
}: {
  onCancel: () => void;
  onCreate: (draft: NewProjectDraft) => void;
}): JSX.Element {
  const [name, setName] = useState("");
  const [nameTouched, setNameTouched] = useState(false);
  const [description, setDescription] = useState("");
  const [infra, setInfra] = useState<InfraChoice>("local");
  const [visibility, setVisibility] = useState<Visibility>("private");
  const [selectedAgents, setSelectedAgents] = useState<Set<string>>(
    new Set(["code-auditor"]),
  );
  const [stagedAssets, setStagedAssets] = useState<StagedAsset[]>([]);

  const nameError = nameTouched && name.trim() === "";

  const removeAsset = (id: string): void =>
    setStagedAssets((prev) => prev.filter((a) => a.id !== id));

  const toggleAgent = (id: string): void => {
    setSelectedAgents((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  return (
    <div className="np-root">
      <div className="np-content">
        {/* 1. Header */}
        <header className="np-header">
          <h1 className="np-title">Create New Project</h1>
          <p className="np-subtitle">
            Initialize a structured workspace for your next breakthrough.
          </p>
        </header>

        {/* 2. Project Core */}
        <section className="np-section">
          <div className="np-section-label">Project Core</div>
          <div className="np-card">
            <div className="np-field">
              <label className="np-field-label" htmlFor="np-name">
                Project Name
                <span className="np-required" aria-hidden="true">*</span>
              </label>
              <input
                id="np-name"
                className={`np-input${nameError ? " np-input-error" : ""}`}
                type="text"
                placeholder="e.g. Project 'Aether' - Q4 Infrastructure"
                value={name}
                required
                aria-required="true"
                aria-invalid={nameError}
                aria-describedby={nameError ? "np-name-error" : undefined}
                onChange={(e) => setName(e.target.value)}
                onBlur={() => setNameTouched(true)}
              />
              {nameError && (
                <span id="np-name-error" className="np-field-error" role="alert">
                  Project name is required.
                </span>
              )}
            </div>
            <div className="np-field">
              <label className="np-field-label" htmlFor="np-description">
                Description
                <span className="np-optional">Optional</span>
              </label>
              <textarea
                id="np-description"
                className="np-textarea"
                rows={3}
                placeholder="Define the objective and scope of this orchestration..."
                value={description}
                onChange={(e) => setDescription(e.target.value)}
              />
            </div>
          </div>
        </section>

        {/* 3. Infrastructure */}
        <section className="np-section">
          <div className="np-section-label">Infrastructure</div>
          <div className="np-infra-grid">
            {INFRA_OPTIONS.map((option) => {
              const active = infra === option.id;
              return (
                <button
                  key={option.id}
                  type="button"
                  className={`np-toggle-card${active ? " np-toggle-card-active" : ""}${option.disabled ? " np-toggle-card-disabled" : ""}`}
                  aria-pressed={active}
                  disabled={option.disabled}
                  onClick={() => !option.disabled && setInfra(option.id)}
                >
                  <span className="np-toggle-icon">
                    <Icon name={option.icon} size={24} />
                  </span>
                  <span className="np-toggle-title">{option.title}</span>
                  <span className="np-toggle-body">{option.body}</span>
                </button>
              );
            })}
          </div>
        </section>

        {/* 4. Contextual Assets — single unified panel */}
        <section className="np-section">
          <div className="np-section-head">
            <div className="np-section-label">Contextual Assets</div>
            <span className="np-chip">Project Knowledge</span>
          </div>
          <div className="np-card">
            <p className="np-assets-hint">
              <Icon name="draw" size={14} />
              Designs&ensp;·&ensp;
              <Icon name="description" size={14} />
              Documents&ensp;·&ensp;
              <Icon name="folder_managed" size={14} />
              Repositories
            </p>

            <div className="np-dropzone">
              <Icon name="cloud_upload" size={32} />
              <p className="np-dropzone-text">
                Drag & drop files or{" "}
                <button type="button" className="np-dropzone-browse">
                  browse local files
                </button>
              </p>
            </div>

            <div className="np-url-row">
              <div className="np-url-input">
                <Icon name="link" size={18} />
                <input
                  className="np-url-field"
                  type="text"
                  aria-label="Asset URL"
                  placeholder="Paste a link — Figma, GitHub repo, Google Doc…"
                />
              </div>
              <button
                type="button"
                className="np-btn-primary np-btn-add"
                aria-label="Add asset URL"
              >
                Add
              </button>
            </div>

            {stagedAssets.length > 0 && (
              <div className="np-staged">
                <div className="np-staged-head">
                  Staged Assets{" "}
                  <span className="np-count-chip">{stagedAssets.length}</span>
                </div>
                <ul className="np-staged-list">
                  {stagedAssets.map((asset) => (
                    <li key={asset.id} className="np-staged-item">
                      <span className="np-staged-tile">
                        <Icon name={asset.icon} size={20} />
                      </span>
                      <span className="np-staged-meta">
                        <span className="np-staged-name">{asset.name}</span>
                        <span className="np-staged-sub">{asset.meta}</span>
                      </span>
                      <button
                        type="button"
                        className="np-icon-btn"
                        aria-label={`Remove ${asset.name}`}
                        onClick={() => removeAsset(asset.id)}
                      >
                        <Icon name="close" size={18} />
                      </button>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </div>
        </section>

        {/* 5. Agent Sessions */}
        <section className="np-section">
          <div className="np-agents-head">
            <div className="np-agents-title">
              <Icon name="smart_toy" size={20} />
              <span>Agent Sessions</span>
            </div>
            <div className="np-agents-actions">
              <button type="button" className="np-btn-outline">
                <Icon name="person_add" size={18} />
                Custom Agent
              </button>
              <span className="np-chip">Automation</span>
            </div>
          </div>
          <div className="np-agents-grid">
            {AGENTS.map((agent) => {
              const active = selectedAgents.has(agent.id);
              return (
                <button
                  key={agent.id}
                  type="button"
                  className={`np-agent-card${active ? " np-agent-card-active" : ""}`}
                  aria-pressed={active}
                  onClick={() => toggleAgent(agent.id)}
                >
                  <span
                    className={`np-radio${active ? " np-radio-active" : ""}`}
                    aria-hidden="true"
                  >
                    {active ? <Icon name="check" size={14} /> : null}
                  </span>
                  <span className="np-agent-icon">
                    <Icon name={agent.icon} size={24} />
                  </span>
                  <span className="np-agent-name">{agent.title}</span>
                  <span className="np-agent-body">{agent.body}</span>
                </button>
              );
            })}
          </div>
        </section>

        {/* 6. Visibility */}
        <section className="np-section">
          <div className="np-section-label">Visibility</div>
          <div className="np-visibility-grid">
            {VISIBILITY_OPTIONS.map((option) => {
              const active = visibility === option.id;
              const disabled = option.id === "public";
              return (
                <button
                  key={option.id}
                  type="button"
                  className={`np-toggle-card${active ? " np-toggle-card-active" : ""}${disabled ? " np-toggle-card-disabled" : ""}`}
                  aria-pressed={active}
                  aria-disabled={disabled}
                  onClick={() => !disabled && setVisibility(option.id)}
                >
                  <span className="np-toggle-icon">
                    <Icon name={option.icon} size={24} />
                  </span>
                  <span className="np-toggle-title">{option.title}</span>
                  <span className="np-toggle-body">{option.body}</span>
                </button>
              );
            })}
          </div>
        </section>

        {/* 7. Footer */}
        <footer className="np-footer">
          <p className="np-disclaimer">
            By clicking "Create Project", you agree to Conduit's automated
            archival policies and compute quotas.
          </p>
          <div className="np-footer-actions">
            <button type="button" className="np-btn-secondary" onClick={onCancel}>
              Cancel
            </button>
            <button
              type="button"
              className="np-btn-primary"
              onClick={() => {
                if (name.trim() === "") {
                  setNameTouched(true);
                  return;
                }
                onCreate({
                  name: name.trim(),
                  description: description.trim(),
                  infra,
                  visibility,
                });
              }}
            >
              Create Project
            </button>
          </div>
        </footer>
      </div>
    </div>
  );
}
