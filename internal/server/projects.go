package server

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ── /api/projects ────────────────────────────────────────────────────
//
// Groups coding-session journals by their RepositoryRoot so the GUI's
// project sidebar can render each repo as a parent row with its chats
// nested underneath. Sessions whose journals carry no RepositoryRoot
// (e.g. ad-hoc invocations from /tmp) fall into the orphans bucket so
// they don't disappear from the sidebar.
//
// The wire shape is locked — the frontend agent is building to the
// exact field names below. Do not rename without coordinating.

// ChatSummary is one entry under a project (or in the orphans list).
type ChatSummary struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
	TurnCount int    `json:"turnCount"`
}

// ProjectSummary groups chats by their RepositoryRoot.
type ProjectSummary struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Path         string        `json:"path"`
	AbsolutePath string        `json:"absolutePath"`
	Branch       string        `json:"branch"`
	SessionCount int           `json:"sessionCount"`
	LastActivity string        `json:"lastActivity"`
	Chats        []ChatSummary `json:"chats"`
}

type projectsResponse struct {
	Projects []ProjectSummary `json:"projects"`
	Orphans  []ChatSummary    `json:"orphans"`
}

// readSessionJournal opens a single .jsonl file and pulls out the
// values needed to slot the session into the projects response. It
// reads the file once with a line-oriented scanner: the first line
// gives createdAt + the title (first non-empty user `Content`), every
// line bumps the turn count, and the LAST line's `At` becomes
// lastTurnAt. RepositoryRoot/GitBranch are taken from the FIRST
// non-empty value seen so an early failed turn that didn't capture
// them doesn't shadow a later one.
func readSessionJournal(path string) (chat ChatSummary, repoRoot, branch string, lastTurnAt time.Time, err error) {
	f, openErr := os.Open(path)
	if openErr != nil {
		err = openErr
		return
	}
	defer f.Close()

	id := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	chat.ID = id

	scanner := bufio.NewScanner(f)
	// JSONL turns can be large (tool output, file diffs); allow up to 4 MiB per line.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	turnCount := 0
	titleSet := false
	createdSet := false
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		turnCount++

		var turn struct {
			At             string `json:"At"`
			Role           string `json:"Role"`
			Content        string `json:"Content"`
			RepositoryRoot string `json:"RepositoryRoot"`
			GitBranch      string `json:"GitBranch"`
		}
		if jerr := json.Unmarshal(line, &turn); jerr != nil {
			// Skip malformed lines but keep counting — partial journals
			// shouldn't make the whole project disappear.
			continue
		}

		if !createdSet && turn.At != "" {
			chat.CreatedAt = normalizeTime(turn.At)
			createdSet = true
		}
		if turn.At != "" {
			if t, perr := time.Parse(time.RFC3339Nano, turn.At); perr == nil {
				lastTurnAt = t
			}
		}
		if !titleSet && turn.Role == "user" {
			if title := truncateTitle(turn.Content); title != "" {
				chat.Title = title
				titleSet = true
			}
		}
		if repoRoot == "" && turn.RepositoryRoot != "" {
			repoRoot = turn.RepositoryRoot
		}
		if branch == "" && turn.GitBranch != "" {
			branch = turn.GitBranch
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		err = scanErr
		return
	}
	chat.TurnCount = turnCount
	return
}

// truncateTitle trims whitespace and clamps to ~80 runes so the
// sidebar's chat row isn't blown out by a 2 KB first message.
func truncateTitle(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= 80 {
		return s
	}
	return string(runes[:77]) + "..."
}

// normalizeTime re-emits a journal timestamp in RFC3339 (second
// precision) so the GUI gets a stable shape regardless of whether the
// source had nanos.
func normalizeTime(s string) string {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return s
}

// projectIDFromPath returns the first 16 hex chars of SHA-256(absolutePath).
// Stable across restarts so the GUI can use it as a React key.
func projectIDFromPath(absolutePath string) string {
	sum := sha256.Sum256([]byte(absolutePath))
	return fmt.Sprintf("%x", sum)[:16]
}

// homeRelative collapses an absolute path under home to a `~/...` form
// for display. Anything outside home stays absolute.
func homeRelative(absolutePath, home string) string {
	if home == "" {
		return absolutePath
	}
	if absolutePath == home {
		return "~"
	}
	prefix := home + string(filepath.Separator)
	if strings.HasPrefix(absolutePath, prefix) {
		return "~" + string(filepath.Separator) + strings.TrimPrefix(absolutePath, prefix)
	}
	return absolutePath
}

// handleProjects walks sessionsDir, groups every .jsonl by its
// RepositoryRoot, and emits the projects/orphans wire shape. Empty or
// missing directory returns the empty-but-well-formed response with
// 200 OK so the GUI can render an empty state without error handling.
func (s *Server) handleProjects(w http.ResponseWriter, _ *http.Request) {
	resp := projectsResponse{
		Projects: []ProjectSummary{},
		Orphans:  []ChatSummary{},
	}
	if s.sessionsDir == "" {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	entries, err := os.ReadDir(s.sessionsDir)
	if err != nil {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Bucket per RepositoryRoot. We track last-turn metadata so the
	// project's branch + lastActivity reflect the most recent session,
	// not whichever was read first off disk.
	type bucket struct {
		absolutePath string
		chats        []ChatSummary
		// Track the journal turn-time and a per-chat creation time so
		// we can pick the most-recent session's branch.
		lastTurnAt     time.Time
		latestSession  time.Time
		latestBranch   string
		latestBranchAt time.Time
	}
	buckets := make(map[string]*bucket)
	var orphans []ChatSummary
	// Track a parallel slice of (orphan, createdAt) so we can sort.
	type orphanEntry struct {
		chat      ChatSummary
		createdAt time.Time
	}
	var orphanEntries []orphanEntry

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(s.sessionsDir, e.Name())
		chat, repoRoot, branch, lastTurnAt, jerr := readSessionJournal(path)
		if jerr != nil {
			continue
		}

		// Use createdAt for ordering chats within a project. If the
		// journal didn't carry an At field at all, fall back to the
		// file's mtime so we still produce a stable order.
		createdAt := time.Time{}
		if chat.CreatedAt != "" {
			if t, perr := time.Parse(time.RFC3339, chat.CreatedAt); perr == nil {
				createdAt = t
			}
		}
		if createdAt.IsZero() {
			if fi, ferr := e.Info(); ferr == nil {
				createdAt = fi.ModTime()
			}
		}

		if repoRoot == "" {
			orphanEntries = append(orphanEntries, orphanEntry{chat: chat, createdAt: createdAt})
			continue
		}

		b, ok := buckets[repoRoot]
		if !ok {
			b = &bucket{absolutePath: repoRoot}
			buckets[repoRoot] = b
		}
		b.chats = append(b.chats, chat)
		if lastTurnAt.After(b.lastTurnAt) {
			b.lastTurnAt = lastTurnAt
		}
		// Branch comes from the most-recent session (by createdAt).
		// Empty branches never overwrite a real one.
		if branch != "" && (b.latestBranch == "" || createdAt.After(b.latestBranchAt)) {
			b.latestBranch = branch
			b.latestBranchAt = createdAt
		}
		if createdAt.After(b.latestSession) {
			b.latestSession = createdAt
		}
	}

	// Build the projects slice. Within each project, sort chats by
	// createdAt descending.
	projects := make([]ProjectSummary, 0, len(buckets))
	for _, b := range buckets {
		// Re-derive a comparable createdAt per chat for sorting since
		// ChatSummary stores its time as a string.
		sort.SliceStable(b.chats, func(i, j int) bool {
			ti, _ := time.Parse(time.RFC3339, b.chats[i].CreatedAt)
			tj, _ := time.Parse(time.RFC3339, b.chats[j].CreatedAt)
			return ti.After(tj)
		})

		lastActivity := b.lastTurnAt
		if lastActivity.IsZero() {
			lastActivity = b.latestSession
		}

		projects = append(projects, ProjectSummary{
			ID:           projectIDFromPath(b.absolutePath),
			Name:         filepath.Base(b.absolutePath),
			Path:         homeRelative(b.absolutePath, s.homeDir),
			AbsolutePath: b.absolutePath,
			Branch:       b.latestBranch,
			SessionCount: len(b.chats),
			LastActivity: lastActivity.UTC().Format(time.RFC3339),
			Chats:        b.chats,
		})
	}
	sort.SliceStable(projects, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, projects[i].LastActivity)
		tj, _ := time.Parse(time.RFC3339, projects[j].LastActivity)
		return ti.After(tj)
	})

	// Sort orphans by createdAt descending and strip the helper struct.
	sort.SliceStable(orphanEntries, func(i, j int) bool {
		return orphanEntries[i].createdAt.After(orphanEntries[j].createdAt)
	})
	orphans = make([]ChatSummary, 0, len(orphanEntries))
	for _, oe := range orphanEntries {
		orphans = append(orphans, oe.chat)
	}

	resp.Projects = projects
	resp.Orphans = orphans
	writeJSON(w, http.StatusOK, resp)
}
