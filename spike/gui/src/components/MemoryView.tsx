import { useEffect, useState } from "react";
import { getMemory, type Memory } from "../api";

export function MemoryView() {
  const [mem, setMem] = useState<Memory | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getMemory()
      .then(setMem)
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  }, []);

  if (error) {
    return <div className="placeholder">Failed to load memory: {error}</div>;
  }
  if (mem === null) {
    return <div className="placeholder">Loading memory…</div>;
  }
  if (!mem.soul && !mem.user) {
    return <div className="placeholder">No memory written yet.</div>;
  }

  return (
    <div className="memory-grid">
      <MemoryCard title="SOUL.md" body={mem.soul} />
      <MemoryCard title="USER.md" body={mem.user} />
    </div>
  );
}

function MemoryCard({ title, body }: { title: string; body: string }) {
  return (
    <div className="memory-card">
      <div className="memory-card-title">{title}</div>
      <pre className="memory-card-body">{body || "(empty)"}</pre>
    </div>
  );
}
