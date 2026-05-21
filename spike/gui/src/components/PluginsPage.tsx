import { Icon } from "./Icon";
import "./InfoPages.css";

// PluginsPage is an honest "coming soon" surface. There is no plugin backend
// yet, so rather than fabricate a marketplace we render an empty state that
// matches the design language: a header, a centered empty-state card, a
// disabled call-to-action, and non-interactive placeholder category chips.

const CATEGORIES = ["Tools", "Data", "Integrations"] as const;

export function PluginsPage(): JSX.Element {
  return (
    <div className="pg-root">
      <div className="pg-content">
        <header className="pg-header">
          <h1 className="pg-title">Plugins</h1>
          <p className="pg-subtitle">
            Extend Conduit with tools and integrations.
          </p>
        </header>

        <section className="pg-empty">
          <span className="pg-empty-tile">
            <Icon name="extension" size={36} />
          </span>
          <h2 className="pg-empty-heading">No plugins installed</h2>
          <p className="pg-empty-text">
            Plugin support is coming soon — you'll be able to browse and install
            extensions here.
          </p>
          <button type="button" className="pg-cta" disabled>
            <Icon name="travel_explore" size={18} />
            Browse plugins
          </button>
          <div className="pg-chips" aria-hidden="true">
            {CATEGORIES.map((category) => (
              <span key={category} className="pg-chip">
                {category}
              </span>
            ))}
          </div>
        </section>
      </div>
    </div>
  );
}
