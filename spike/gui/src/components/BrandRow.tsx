import { Icon } from "./Icon";

// BrandHeader sits at the top of the sidebar: a near-black rounded logo tile
// with the `terminal` glyph, the Conduit wordmark + version, and the gold
// "Update" pill. Native window chrome (traffic lights) is provided by Tauri,
// so the in-app surface no longer fakes it.
export function BrandHeader({ version }: { version: string }) {
  return (
    <div className="brand-header">
      <div className="brand-logo-box">
        <Icon name="terminal" size={20} />
      </div>
      <div className="brand-meta">
        <span className="wordmark">Conduit</span>
        <span className="brand-version">v{version}</span>
      </div>
    </div>
  );
}
