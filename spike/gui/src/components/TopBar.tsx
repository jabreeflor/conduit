import { Icon } from "./Icon";

// TopBar is the 48px chrome above the content column: history nav, a global
// search field, and an optional context label.
export function TopBar({
  label,
  searchPlaceholder = "Search conversations…",
  onSearch,
  onBack,
  onForward,
  canBack = false,
  canForward = false,
}: {
  label?: string;
  searchPlaceholder?: string;
  onSearch?: (query: string) => void;
  onBack?: () => void;
  onForward?: () => void;
  canBack?: boolean;
  canForward?: boolean;
}) {
  return (
    <header className="topbar">
      <div className="topbar-nav">
        <button
          type="button"
          className="topbar-icon topbar-navbtn"
          onClick={onBack}
          disabled={!canBack}
          aria-label="Back"
        >
          <Icon name="chevron_left" size={18} />
        </button>
        <button
          type="button"
          className="topbar-icon topbar-navbtn"
          onClick={onForward}
          disabled={!canForward}
          aria-label="Forward"
        >
          <Icon name="chevron_right" size={18} />
        </button>
      </div>
      <div className="topbar-search">
        <span className="topbar-icon" aria-hidden>
          <Icon name="search" size={18} />
        </span>
        <input
          type="text"
          placeholder={searchPlaceholder}
          aria-label="Search"
          onFocus={() => onSearch?.("")}
          onKeyDown={(e) => {
            if (e.key === "Enter") onSearch?.(e.currentTarget.value);
          }}
          readOnly={Boolean(onSearch)}
        />
      </div>
      <div className="topbar-spacer" />
      {label && <span className="topbar-label">{label}</span>}
    </header>
  );
}
