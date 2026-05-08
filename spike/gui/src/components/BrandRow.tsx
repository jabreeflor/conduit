import { ChevronLeft, ChevronRight } from "lucide-react";

// Inlined Conduit "Channel Bracket" logo, traced from assets/logo.svg.
// Inline so the sidebar can size it freely (22px) and the welcome screen can
// reuse it at 44px without an extra <img>/<svg> roundtrip.
export function ConduitMark({
  size,
  className,
  idSuffix = "sm",
}: {
  size: number;
  className?: string;
  idSuffix?: string;
}) {
  const bgId = `conduit-bg-${idSuffix}`;
  const gradId = `conduit-grad-${idSuffix}`;
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 512 512"
      className={className}
      aria-hidden
    >
      <defs>
        <linearGradient id={bgId} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0%" stopColor="#0b0d12" />
          <stop offset="100%" stopColor="#0e1320" />
        </linearGradient>
        <linearGradient id={gradId} x1="0" y1="0" x2="1" y2="0">
          <stop offset="0%" stopColor="#fbbf24" />
          <stop offset="100%" stopColor="#b45309" />
        </linearGradient>
      </defs>
      <rect width="512" height="512" rx="112" fill={`url(#${bgId})`} />
      <path
        d="M 184 128 L 128 128 L 128 384 L 184 384"
        fill="none"
        stroke="#f5f5f4"
        strokeWidth="36"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M 328 128 L 384 128 L 384 384 L 328 384"
        fill="none"
        stroke="#f5f5f4"
        strokeWidth="36"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M 220 192 L 296 256 L 220 320"
        fill="none"
        stroke={`url(#${gradId})`}
        strokeWidth="40"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <circle cx="178" cy="256" r="10" fill="#fbbf24" opacity="0.6" />
      <circle cx="156" cy="256" r="6" fill="#fbbf24" opacity="0.3" />
    </svg>
  );
}

// BrandRow renders the macOS chrome row (traffic lights + back/fwd + Update
// pill) and below it the brand row (logo + Conduit + version). It sits at
// the top of the sidebar.
export function BrandRow({ version }: { version: string }) {
  return (
    <>
      <div className="chrome">
        <div className="traffic" aria-hidden>
          <span className="dot dot-r" />
          <span className="dot dot-y" />
          <span className="dot dot-g" />
        </div>
        <div className="nav-arrows" aria-hidden>
          <ChevronLeft size={16} />
          <ChevronRight size={16} />
        </div>
        <div className="chrome-spacer" />
        <div className="update-pill">Update</div>
      </div>
      <div className="brand-row">
        <ConduitMark size={22} className="brand-logo" idSuffix="brand" />
        <span className="wordmark">Conduit</span>
        <span className="brand-version">v{version}</span>
      </div>
    </>
  );
}
