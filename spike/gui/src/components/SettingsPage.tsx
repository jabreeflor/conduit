import { useEffect, useRef, useState } from "react";
import type { Theme } from "../useTheme";
import { THEME_META } from "../useTheme";
import { getInfo, patchSettings, type Info } from "../api";
import { Icon } from "./Icon";
import "./SettingsPage.css";

const THEMES: Theme[] = ["light", "dark", "hc"];

const PROVIDER_MODELS: Record<string, { value: string; label: string }[]> = {
  anthropic: [
    { value: "claude-opus-4-7",           label: "Claude Opus 4.7" },
    { value: "claude-opus-4-5",           label: "Claude Opus 4.5" },
    { value: "claude-sonnet-4-6",         label: "Claude Sonnet 4.6" },
    { value: "claude-haiku-4-5-20251001", label: "Claude Haiku 4.5" },
  ],
  codex: [
    { value: "gpt-5.5",  label: "GPT-5.5" },
    { value: "gpt-4o",   label: "GPT-4o" },
    { value: "o3",       label: "o3" },
    { value: "o4-mini",  label: "o4-mini" },
  ],
  echo: [
    { value: "(none)", label: "(none — echo only)" },
  ],
};

const DEFAULT_MODEL: Record<string, string> = {
  anthropic: "claude-opus-4-5",
  codex:     "gpt-5.5",
  echo:      "(none)",
};

type InfoState =
  | { status: "loading" }
  | { status: "ready"; info: Info }
  | { status: "error" };

type SaveState = "idle" | "saving" | "saved" | "error";

export function SettingsPage({
  theme,
  onSetTheme,
}: {
  theme: Theme;
  onSetTheme: (t: Theme) => void;
}): JSX.Element {
  const [info, setInfo] = useState<InfoState>({ status: "loading" });
  const [selectedProvider, setSelectedProvider] = useState("anthropic");
  const [selectedModel, setSelectedModel]       = useState(DEFAULT_MODEL.anthropic);
  const [saveState, setSaveState]               = useState<SaveState>("idle");
  const [saveError, setSaveError]               = useState("");
  const seededRef = useRef(false);

  useEffect(() => {
    let cancelled = false;
    getInfo()
      .then((data) => {
        if (cancelled) return;
        setInfo({ status: "ready", info: data });
        if (!seededRef.current) {
          seededRef.current = true;
          const p = data.provider in PROVIDER_MODELS ? data.provider : "anthropic";
          setSelectedProvider(p);
          setSelectedModel(data.model || DEFAULT_MODEL[p]);
        }
      })
      .catch(() => {
        if (!cancelled) setInfo({ status: "error" });
      });
    return () => { cancelled = true; };
  }, []);

  function handleProviderChange(p: string) {
    setSelectedProvider(p);
    setSelectedModel(DEFAULT_MODEL[p] ?? "");
    setSaveState("idle");
  }

  async function handleSave() {
    setSaveState("saving");
    setSaveError("");
    try {
      const updated = await patchSettings(selectedProvider, selectedModel);
      setInfo({ status: "ready", info: updated });
      setSaveState("saved");
      setTimeout(() => setSaveState("idle"), 2000);
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : "Unknown error");
      setSaveState("error");
    }
  }

  const modelOptions = PROVIDER_MODELS[selectedProvider] ?? [];
  const isDirty =
    info.status === "ready" &&
    (info.info.provider !== selectedProvider || info.info.model !== selectedModel);

  return (
    <div className="set-root">
      <div className="set-content">
        <header className="set-header">
          <h1 className="set-title">Settings</h1>
          <p className="set-subtitle">
            Tune the look of Conduit and configure the model provider.
          </p>
        </header>

        {/* ---- Appearance -------------------------------------------- */}
        <section className="set-section" aria-labelledby="set-appearance-label">
          <h2 id="set-appearance-label" className="set-section-label">
            Appearance
          </h2>
          <div className="set-card">
            <div className="set-field">
              <div className="set-field-head">
                <span className="set-field-title">Theme</span>
                <span className="set-field-hint">
                  Applies instantly and is remembered across launches.
                </span>
              </div>
              <div className="set-theme-grid" role="radiogroup" aria-label="Theme">
                {THEMES.map((t) => {
                  const meta = THEME_META[t];
                  const active = theme === t;
                  return (
                    <button
                      key={t}
                      type="button"
                      role="radio"
                      aria-checked={active}
                      className={active ? "set-theme-card set-theme-card-active" : "set-theme-card"}
                      onClick={() => onSetTheme(t)}
                    >
                      <span className="set-theme-icon">
                        <Icon name={meta.icon} size={22} fill={active} />
                      </span>
                      <span className="set-theme-label">{meta.label}</span>
                      {active && (
                        <span className="set-theme-check" aria-hidden="true">
                          <Icon name="check_circle" size={18} fill />
                        </span>
                      )}
                    </button>
                  );
                })}
              </div>
            </div>
          </div>
        </section>

        {/* ---- Model provider ---------------------------------------- */}
        <section className="set-section" aria-labelledby="set-model-label">
          <h2 id="set-model-label" className="set-section-label">
            Model provider
          </h2>
          <div className="set-card">
            {info.status === "error" && (
              <p className="set-notice">
                Couldn&rsquo;t reach the backend. Start it with{" "}
                <code>conduit serve</code> to change the active provider.
              </p>
            )}
            <div className="set-model-grid">
              <div className="set-select-group">
                <label className="set-select-label" htmlFor="set-provider-select">
                  Provider
                </label>
                <div className="set-select-wrap">
                  <select
                    id="set-provider-select"
                    className="set-select"
                    value={selectedProvider}
                    onChange={(e) => handleProviderChange(e.target.value)}
                    disabled={saveState === "saving"}
                  >
                    <option value="anthropic">Anthropic</option>
                    <option value="codex">OpenAI Codex</option>
                    <option value="echo">Echo (no API calls)</option>
                  </select>
                  <span className="set-select-arrow" aria-hidden="true">
                    <Icon name="expand_more" size={18} />
                  </span>
                </div>
              </div>

              <div className="set-select-group">
                <label className="set-select-label" htmlFor="set-model-select">
                  Model
                </label>
                <div className="set-select-wrap">
                  <select
                    id="set-model-select"
                    className="set-select"
                    value={selectedModel}
                    onChange={(e) => { setSelectedModel(e.target.value); setSaveState("idle"); }}
                    disabled={saveState === "saving" || selectedProvider === "echo"}
                  >
                    {modelOptions.map((o) => (
                      <option key={o.value} value={o.value}>{o.label}</option>
                    ))}
                  </select>
                  <span className="set-select-arrow" aria-hidden="true">
                    <Icon name="expand_more" size={18} />
                  </span>
                </div>
              </div>
            </div>

            <div className="set-model-footer">
              {saveState === "error" && (
                <span className="set-save-error">{saveError}</span>
              )}
              {saveState === "saved" && (
                <span className="set-save-ok">
                  <Icon name="check_circle" size={15} fill /> Saved
                </span>
              )}
              <button
                type="button"
                className={saveState === "saved" ? "set-save-btn set-save-btn-done" : "set-save-btn"}
                onClick={handleSave}
                disabled={saveState === "saving" || !isDirty}
              >
                {saveState === "saving" ? "Saving…" : "Apply"}
              </button>
            </div>
          </div>
        </section>

        {/* ---- About ------------------------------------------------- */}
        <section className="set-section" aria-labelledby="set-about-label">
          <h2 id="set-about-label" className="set-section-label">
            About
          </h2>
          <div className="set-card">
            <dl className="set-rows">
              <div className="set-row">
                <dt className="set-row-key">Version</dt>
                <dd className="set-row-val set-row-mono">
                  {info.status === "ready" ? info.info.version : "—"}
                </dd>
              </div>
              <div className="set-row">
                <dt className="set-row-key">Active provider</dt>
                <dd className="set-row-val">
                  {info.status === "ready" ? info.info.provider : "—"}
                </dd>
              </div>
              <div className="set-row">
                <dt className="set-row-key">Active model</dt>
                <dd className="set-row-val set-row-mono">
                  {info.status === "ready" ? info.info.model : "—"}
                </dd>
              </div>
            </dl>
          </div>
        </section>

        {/* ---- Coming soon ------------------------------------------- */}
        <section className="set-section" aria-labelledby="set-soon-label">
          <h2 id="set-soon-label" className="set-section-label">
            Account &amp; team
          </h2>
          <div className="set-card set-card-muted">
            <span className="set-soon-icon" aria-hidden="true">
              <Icon name="hourglass_empty" size={20} />
            </span>
            <div className="set-soon-text">
              <p className="set-soon-title">Coming soon</p>
              <p className="set-soon-desc">
                Account, billing, and team settings aren&rsquo;t available yet —
                there&rsquo;s no backend for them today.
              </p>
            </div>
          </div>
        </section>
      </div>
    </div>
  );
}
