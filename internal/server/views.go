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
	provider, model := s.provider, s.model
	if s.rtCfg != nil {
		provider, model = s.rtCfg.Get()
	}
	writeJSON(w, http.StatusOK, infoResponse{
		Provider:  provider,
		Model:     model,
		Version:   s.version,
		SessionID: "", // filled per-connection in the agent ws "session" frame
	})
}

// ── /api/settings ────────────────────────────────────────────────────

var validProviders = map[string]bool{"anthropic": true, "codex": true, "echo": true}

type settingsBody struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// handleSettings accepts PATCH /api/settings to hot-swap provider and model.
// New WebSocket connections pick up the change immediately; in-flight sessions
// are unaffected.
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		w.Header().Set("Allow", "PATCH")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.rtCfg == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "runtime settings not available"})
		return
	}
	var body settingsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	body.Provider = strings.TrimSpace(body.Provider)
	body.Model = strings.TrimSpace(body.Model)
	if !validProviders[body.Provider] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown provider; must be anthropic, codex, or echo"})
		return
	}
	s.rtCfg.Set(body.Provider, body.Model)
	provider, model := s.rtCfg.Get()
	writeJSON(w, http.StatusOK, infoResponse{
		Provider:  provider,
		Model:     model,
		Version:   s.version,
		SessionID: "",
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

// handleMemory serves the agent memory files. GET reads SOUL.md and USER.md
// from ~/.conduit/ (missing files return empty strings). POST/PUT writes them
// back from a {soul, user} body — the "editor seeds them on first save" path
// the read handler anticipates. The response echoes the saved memory so the
// client can refresh from a single round-trip.
func (s *Server) handleMemory(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		out := memoryResponse{}
		if s.homeDir != "" {
			out.Soul = readFileOrEmpty(filepath.Join(s.homeDir, ".conduit", "SOUL.md"))
			out.User = readFileOrEmpty(filepath.Join(s.homeDir, ".conduit", "USER.md"))
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost, http.MethodPut:
		s.saveMemory(w, r)
	default:
		w.Header().Set("Allow", "GET, POST, PUT")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "method not allowed",
		})
	}
}

// saveMemory persists the SOUL.md / USER.md bodies under ~/.conduit/. Both
// files are always written so the pair stays consistent; the client sends the
// full memory it wants on disk. Writes are atomic (temp file + rename) so a
// crash mid-write can't leave a half-written memory file.
func (s *Server) saveMemory(w http.ResponseWriter, r *http.Request) {
	if s.homeDir == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "no home directory configured",
		})
		return
	}
	var body memoryResponse
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON body",
		})
		return
	}
	dir := filepath.Join(s.homeDir, ".conduit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "could not create memory directory",
		})
		return
	}
	for name, content := range map[string]string{
		"SOUL.md": body.Soul,
		"USER.md": body.User,
	} {
		if err := writeFileAtomic(filepath.Join(dir, name), content); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "could not write " + name,
			})
			return
		}
	}
	writeJSON(w, http.StatusOK, body)
}

// writeFileAtomic writes content to path via a sibling temp file + rename so
// readers never observe a partially written file.
func writeFileAtomic(path, content string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func readFileOrEmpty(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}
