// Local, client-side project registry. The Conduit core derives "projects"
// from session journals grouped by git repo — it has no concept of a created,
// metadata-rich project (infra/visibility/assets/agents). Rather than fake a
// server entity, projects created via the New Project form are persisted here
// in localStorage and merged into the Projects list (clearly labeled "Local")
// until they accrue real server sessions. A real projects-as-entity backend is
// tracked in FEATURES_TODO.
export type LocalProject = {
  id: string;
  name: string;
  description: string;
  infra: "local" | "cloud";
  visibility: "private" | "team" | "public";
  createdAt: string; // ISO
};

const KEY = "conduit.localProjects";

export function loadLocalProjects(): LocalProject[] {
  try {
    const raw = window.localStorage.getItem(KEY);
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    return Array.isArray(parsed) ? (parsed as LocalProject[]) : [];
  } catch {
    return [];
  }
}

export function addLocalProject(
  draft: Omit<LocalProject, "id" | "createdAt">,
): LocalProject {
  const project: LocalProject = {
    ...draft,
    id: `local-${Date.now().toString(36)}`,
    createdAt: new Date().toISOString(),
  };
  const next = [project, ...loadLocalProjects()];
  try {
    window.localStorage.setItem(KEY, JSON.stringify(next));
  } catch {
    // Best-effort; the create still returns the in-memory project.
  }
  return project;
}
