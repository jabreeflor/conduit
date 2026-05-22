// SettingsPage — preferences surface for the Conduit GUI. Renders inside the
// app shell's content column (sidebar + 48px top bar live outside). The theme
// selector is real and persists via the useTheme hook (the parent owns the
// state + persistence); the About section reads live values from the backend.
// All classes are prefixed `set-` and colors come from global design tokens.

import { useEffect, useState } from "react";
import type { Theme } from "../useTheme";
import { THEME_META } from "../useTheme";
import { getInfo, type Info } from "../api";
import { Icon } from "./Icon";
import "./SettingsPage.css";

const THEMES: Theme[] = ["light", "dark", "hc"];

type InfoState =
  | { status: "loading" }
  | { status: "ready"; info: Info }
  | { status: "error" };

export function SettingsPage({
  theme,
  onSetTheme,
}: {
  theme: Theme;
  onSetTheme: (t: Theme) => void;
}): JSX.Element {
  const [info, setInfo] = useState<InfoState>({ status: "loading" });

  useEffect(() => {
    let cancelled = false;
    getInfo()
      .then((data) => {
        if (!cancelled) setInfo({ status: "ready", info: data });
      })
      .catch(() => {
        if (!cancelled) setInfo({ status: "error" });
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div className="set-root">
      <div className="set-content">
        <header className="set-header">
          <h1 className="set-title">Settings</h1>
          <p className="set-subtitle">
            Tune the look of Conduit and check what the backend is running.
          </p>
        </header>

        {/* ---- Appearance ------------------------------------------------ */}
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
              <div
                className="set-theme-grid"
                role="radiogroup"
                aria-label="Theme"
              >
                {THEMES.map((t) => {
                  const meta = THEME_META[t];
                  const active = theme === t;
                  return (
                    <button
                      key={t}
                      type="button"
                      role="radio"
                      aria-checked={active}
                      className={
                        active
                          ? "set-theme-card set-theme-card-active"
                          : "set-theme-card"
                      }
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

        {/* ---- About ----------------------------------------------------- */}
        <section className="set-section" aria-labelledby="set-about-label">
          <h2 id="set-about-label" className="set-section-label">
            About
          </h2>
          <div className="set-card">
            {info.status === "error" && (
              <p className="set-notice">
                Couldn&rsquo;t reach the backend. Start it with{" "}
                <code>conduit serve</code> to see live details.
              </p>
            )}
            <dl className="set-rows">
              <div className="set-row">
                <dt className="set-row-key">Version</dt>
                <dd className="set-row-val set-row-mono">
                  {info.status === "ready" ? info.info.version : "—"}
                </dd>
              </div>
              <div className="set-row">
                <dt className="set-row-key">Provider</dt>
                <dd className="set-row-val">
                  {info.status === "ready" ? info.info.provider : "—"}
                </dd>
              </div>
              <div className="set-row">
                <dt className="set-row-key">Model</dt>
                <dd className="set-row-val set-row-mono">
                  {info.status === "ready" ? info.info.model : "—"}
                </dd>
              </div>
            </dl>
          </div>
        </section>

        {/* ---- Coming soon ----------------------------------------------- */}
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
                there&rsquo;s no backend for them today. We&rsquo;ll surface
                them here once they ship.
              </p>
            </div>
          </div>
        </section>
      </div>
    </div>
  );
}
