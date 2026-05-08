package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ── /api/info ────────────────────────────────────────────────────────

type infoResponse struct {
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Version   string `json:"version"`
	SessionID string `json:"sessionId"`
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, infoResponse{
		Provider:  s.provider,
		Model:     s.model,
		Version:   s.version,
		SessionID: "", // filled per-connection in the agent ws "session" frame
	})
}

// ── /api/sessions ────────────────────────────────────────────────────

type sessionInfo struct {
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
	Summary   string `json:"summary"`
}

// handleSessions tails up to 20 most-recent coding-session journals.
// Each .jsonl under sessionsDir is treated as one session; ordering is
// by file modtime descending to match the GUI expectation that the
// most recent run sits at the top. A missing directory returns an
// empty list rather than a 500 — the GUI handles empty.
func (s *Server) handleSessions(w http.ResponseWriter, _ *http.Request) {
	out := make([]sessionInfo, 0)
	if s.sessionsDir == "" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	entries, err := os.ReadDir(s.sessionsDir)
	if err != nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	type entry struct {
		name string
		mod  time.Time
	}
	var ents []entry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		ents = append(ents, entry{name: e.Name(), mod: fi.ModTime()})
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].mod.After(ents[j].mod) })
	if len(ents) > 20 {
		ents = ents[:20]
	}
	for _, e := range ents {
		id := strings.TrimSuffix(e.name, ".jsonl")
		out = append(out, sessionInfo{
			ID:        id,
			CreatedAt: e.mod.UTC().Format(time.RFC3339),
			Summary:   summarizeJournal(filepath.Join(s.sessionsDir, e.name)),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// summarizeJournal pulls the first prompt/content field from a JSONL
// session journal so the sidebar has something to display. Peeks at up
// to 8 lines and bails; older or unrelated files return empty.
func summarizeJournal(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	for i := 0; i < 8; i++ {
		var raw map[string]any
		if err := dec.Decode(&raw); err != nil {
			return ""
		}
		for _, k := range []string{"prompt", "content"} {
			if v, ok := raw[k].(string); ok && v != "" {
				v = strings.TrimSpace(v)
				if len(v) > 80 {
					v = v[:79] + "…"
				}
				return v
			}
		}
	}
	return ""
}

// ── /api/memory ──────────────────────────────────────────────────────

type memoryResponse struct {
	Soul string `json:"soul"`
	User string `json:"user"`
}

// handleMemory reads SOUL.md and USER.md from ~/.conduit/. Missing
// files return empty strings — the GUI's editor seeds them on first
// save.
func (s *Server) handleMemory(w http.ResponseWriter, _ *http.Request) {
	out := memoryResponse{}
	if s.homeDir != "" {
		out.Soul = readFileOrEmpty(filepath.Join(s.homeDir, ".conduit", "SOUL.md"))
		out.User = readFileOrEmpty(filepath.Join(s.homeDir, ".conduit", "USER.md"))
	}
	writeJSON(w, http.StatusOK, out)
}

func readFileOrEmpty(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}
