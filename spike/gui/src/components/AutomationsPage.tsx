import { Icon } from "./Icon";
import "./InfoPages.css";

// AutomationsPage is an honest "coming soon" surface. There is no automation
// backend yet, so rather than fabricate scheduled workflows we render an empty
// state that matches the design language: a header, a centered empty-state
// card, and a disabled call-to-action.

export function AutomationsPage(): JSX.Element {
  return (
    <div className="pg-root">
      <div className="pg-content">
        <header className="pg-header">
          <h1 className="pg-title">Automations</h1>
          <p className="pg-subtitle">Schedule and chain agent workflows.</p>
        </header>

        <section className="pg-empty">
          <span className="pg-empty-tile">
            <Icon name="auto_mode" size={36} />
          </span>
          <h2 className="pg-empty-heading">No automations yet</h2>
          <p className="pg-empty-text">
            Automation workflows are coming soon — define triggers and
            multi-step agent runs here.
          </p>
          <button type="button" className="pg-cta" disabled>
            <Icon name="add" size={18} />
            New automation
          </button>
        </section>
      </div>
    </div>
  );
}
