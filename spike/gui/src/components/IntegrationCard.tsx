import type { ReactNode } from "react";

// IntegrationCard renders one of the 4 connect-X cards under the welcome
// composer. Pure presentational — clicking the card is a no-op for now since
// integration auth flows aren't wired in this PR.
export function IntegrationCard({
  icon,
  title,
  body,
}: {
  icon: ReactNode;
  title: string;
  body: string;
}) {
  return (
    <div className="integration-card">
      <div className="integration-brand">{icon}</div>
      <div className="integration-title">{title}</div>
      <div className="integration-body">{body}</div>
    </div>
  );
}

// ── Brand SVGs — inlined so the cards don't depend on an icon library for
//    third-party logos and so each mark gets its real brand colors. ──────

export function GitHubMark() {
  return (
    <svg width="32" height="32" viewBox="0 0 24 24" fill="#E2E5EE" aria-hidden>
      <path d="M12 .5C5.65.5.5 5.65.5 12c0 5.08 3.29 9.39 7.86 10.91.58.1.79-.25.79-.56v-2c-3.2.7-3.88-1.36-3.88-1.36-.52-1.33-1.28-1.69-1.28-1.69-1.05-.71.08-.7.08-.7 1.16.08 1.77 1.19 1.77 1.19 1.03 1.77 2.71 1.26 3.37.96.1-.75.4-1.26.74-1.55-2.55-.29-5.23-1.27-5.23-5.66 0-1.25.45-2.27 1.18-3.07-.12-.29-.51-1.46.11-3.04 0 0 .96-.31 3.15 1.17a10.93 10.93 0 0 1 5.74 0c2.19-1.48 3.15-1.17 3.15-1.17.62 1.58.23 2.75.11 3.04.74.8 1.18 1.82 1.18 3.07 0 4.4-2.69 5.36-5.25 5.65.41.36.78 1.06.78 2.13v3.16c0 .31.21.67.8.56A11.5 11.5 0 0 0 23.5 12C23.5 5.65 18.35.5 12 .5z" />
    </svg>
  );
}

export function LinearMark() {
  return (
    <svg width="32" height="32" viewBox="0 0 100 100" aria-hidden>
      <defs>
        <linearGradient id="conduit-linear-grad" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0%" stopColor="#5E6AD2" />
          <stop offset="100%" stopColor="#7A8CFF" />
        </linearGradient>
      </defs>
      <path
        fill="url(#conduit-linear-grad)"
        d="M2 56c1.6 17.7 15.3 31.4 33 33L2 56zm0-9.6L45.6 90c4 .4 7.6.4 11.7 0L2 34.7v11.7zm0-19.7L65.3 88c2.8-.7 5.4-1.7 8-2.9L4.9 17.5c-1.2 2.6-2.2 5.2-2.9 8zM10.6 9C19.7 1.3 31-2 50 2c26.4 5.6 42.4 21.6 48 48 4 19-.7 30.3-8.4 39.4L10.6 9z"
      />
    </svg>
  );
}

export function McpMark() {
  return (
    <svg width="32" height="32" viewBox="0 0 32 32" fill="none" aria-hidden>
      <path
        d="M16 2 L28 9 V23 L16 30 L4 23 V9 Z"
        stroke="#F59E1F"
        strokeWidth="1.8"
        fill="rgba(245,158,31,0.06)"
      />
      <circle cx="16" cy="16" r="4" fill="#36B3CF" />
      <path
        d="M16 12 V6 M16 26 V20 M22 16 H28 M4 16 H10"
        stroke="#36B3CF"
        strokeWidth="1.6"
        strokeLinecap="round"
      />
    </svg>
  );
}

export function SlackMark() {
  return (
    <svg width="32" height="32" viewBox="0 0 32 32" aria-hidden>
      <rect x="4" y="13" width="9" height="3" rx="1.5" fill="#36C5F0" />
      <rect x="13" y="4" width="3" height="9" rx="1.5" fill="#2EB67D" />
      <rect
        x="16"
        y="13"
        width="9"
        height="3"
        rx="1.5"
        transform="rotate(180 20.5 14.5)"
        fill="#ECB22E"
      />
      <rect
        x="13"
        y="16"
        width="3"
        height="9"
        rx="1.5"
        transform="rotate(180 14.5 20.5)"
        fill="#E01E5A"
      />
      <rect x="19" y="13" width="9" height="3" rx="1.5" fill="#ECB22E" />
      <rect x="16" y="19" width="3" height="9" rx="1.5" fill="#E01E5A" />
      <rect x="4" y="16" width="9" height="3" rx="1.5" fill="#36C5F0" />
      <rect x="13" y="19" width="3" height="9" rx="1.5" fill="#2EB67D" />
    </svg>
  );
}
