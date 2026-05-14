package e2e

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
	"github.com/jabreeflor/conduit/internal/sessions"
	"github.com/jabreeflor/conduit/internal/skills"
)

// TestSessionsLifecycleJourney walks a complete session-store lifecycle:
// create two parallel sessions, list/load them, find a turn by id, fork
// from a midpoint, diverge the fork, replay through a scripted responder,
// and assert the missing-turn error path.
func TestSessionsLifecycleJourney(t *testing.T) {
	home := newHome(t)

	// 1. Bootstrap.
	store, err := sessions.NewStore(home.Sessions)
	if err != nil {
		t.Fatalf("NewStore(%q): %v", home.Sessions, err)
	}

	// 2. Two parallel conversations.
	sessA, err := store.Create()
	if err != nil {
		t.Fatalf("Create sessA: %v", err)
	}
	sessB, err := store.Create()
	if err != nil {
		t.Fatalf("Create sessB: %v", err)
	}
	if sessA.ID == sessB.ID {
		t.Fatalf("Create returned duplicate session ids: %q", sessA.ID)
	}

	// Six turns into A, alternating user / assistant.
	contentsA := []struct {
		Role    string
		Content string
	}{
		{"user", "Can you help me design a session store?"},
		{"assistant", "Sure — what storage layout did you have in mind?"},
		{"user", "JSONL per session with parent links between turns."},
		{"assistant", "That gives you append-only writes and easy forks."},
		{"user", "How would I implement fork-from-turn cleanly?"},
		{"assistant", "Walk the parent chain to the fork point, rewrite ids."},
	}

	var idsA []string
	var lastIDA string
	for i, c := range contentsA {
		written, err := store.Append(sessA, sessions.Turn{
			Role:     c.Role,
			Content:  c.Content,
			ParentID: lastIDA,
		})
		if err != nil {
			t.Fatalf("Append sessA turn %d (%s): %v", i, c.Role, err)
		}
		if written.ID == "" {
			t.Fatalf("Append sessA turn %d returned empty id", i)
		}
		if written.SessionID != sessA.ID {
			t.Fatalf("Append sessA turn %d session id mismatch: got %q want %q", i, written.SessionID, sessA.ID)
		}
		idsA = append(idsA, written.ID)
		lastIDA = written.ID

		if i == 0 {
			// Verify the disk-backed journal exists after the first append.
			if info, err := os.Stat(sessA.Path); err != nil || info.Size() == 0 {
				t.Fatalf("expected non-empty journal at %s after first append; err=%v info=%+v", sessA.Path, err, info)
			}
		}
	}

	contentsB := []struct {
		Role    string
		Content string
	}{
		{"user", "Different conversation about logging."},
		{"assistant", "Sure, what level granularity are you considering?"},
		{"user", "Structured JSON, one event per line."},
		{"assistant", "That pairs well with jq-style debugging."},
	}
	var lastIDB string
	for i, c := range contentsB {
		written, err := store.Append(sessB, sessions.Turn{
			Role:     c.Role,
			Content:  c.Content,
			ParentID: lastIDB,
		})
		if err != nil {
			t.Fatalf("Append sessB turn %d (%s): %v", i, c.Role, err)
		}
		lastIDB = written.ID
	}

	if got := len(sessA.Turns); got != 6 {
		t.Fatalf("sessA in-memory turns: got %d want 6", got)
	}
	if got := len(sessB.Turns); got != 4 {
		t.Fatalf("sessB in-memory turns: got %d want 4", got)
	}

	// 3. List both, newest first, with correct titles.
	infos, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("List returned %d sessions, want 2: %+v", len(infos), infos)
	}
	// Newest first — sessB was written last, so it should come first.
	if !infos[0].UpdatedAt.After(infos[1].UpdatedAt) && !infos[0].UpdatedAt.Equal(infos[1].UpdatedAt) {
		t.Fatalf("List not sorted newest-first: %v then %v", infos[0].UpdatedAt, infos[1].UpdatedAt)
	}
	// TurnCount per id.
	turnCounts := map[string]int{}
	titles := map[string]string{}
	for _, info := range infos {
		turnCounts[info.ID] = info.TurnCount
		titles[info.ID] = info.Title
	}
	if turnCounts[sessA.ID] != 6 {
		t.Fatalf("sessA TurnCount: got %d want 6", turnCounts[sessA.ID])
	}
	if turnCounts[sessB.ID] != 4 {
		t.Fatalf("sessB TurnCount: got %d want 4", turnCounts[sessB.ID])
	}
	wantTitleA := truncateExpected(contentsA[0].Content, 60)
	if titles[sessA.ID] != wantTitleA {
		t.Fatalf("sessA Title: got %q want %q", titles[sessA.ID], wantTitleA)
	}

	// 4. Load round-trip: ids, roles, contents preserved.
	loadedA, err := store.Load(sessA.ID)
	if err != nil {
		t.Fatalf("Load %s: %v", sessA.ID, err)
	}
	if got := len(loadedA.Turns); got != len(contentsA) {
		t.Fatalf("loadedA turn count: got %d want %d", got, len(contentsA))
	}
	for i, want := range contentsA {
		got := loadedA.Turns[i]
		if got.ID != idsA[i] {
			t.Fatalf("loadedA turn %d id: got %q want %q", i, got.ID, idsA[i])
		}
		if got.Role != want.Role {
			t.Fatalf("loadedA turn %d role: got %q want %q", i, got.Role, want.Role)
		}
		if got.Content != want.Content {
			t.Fatalf("loadedA turn %d content: got %q want %q", i, got.Content, want.Content)
		}
		if got.SessionID != sessA.ID {
			t.Fatalf("loadedA turn %d session id: got %q want %q", i, got.SessionID, sessA.ID)
		}
		// Timestamp should round-trip with monotonic-clock tolerance: the
		// reloaded timestamp must not be earlier than the writer's.
		orig := sessA.Turns[i].At
		if got.At.Before(orig.Add(-1)) || got.At.After(orig.Add(1)) {
			// JSON marshalling via RFC3339Nano is lossless modulo monotonic
			// fields; assert the wall-clock instants match within a tiny window.
			t.Fatalf("loadedA turn %d timestamp drift: got %v want ~%v", i, got.At, orig)
		}
	}

	// 5. FindTurn finds a middle turn.
	midIdx := 2 // third turn in A (zero-indexed)
	midTurnID := idsA[midIdx]
	foundSess, foundTurn, err := store.FindTurn(midTurnID)
	if err != nil {
		t.Fatalf("FindTurn(%q): %v", midTurnID, err)
	}
	if foundSess.ID != sessA.ID {
		t.Fatalf("FindTurn returned session %q want %q", foundSess.ID, sessA.ID)
	}
	if foundTurn.ID != midTurnID {
		t.Fatalf("FindTurn returned turn id %q want %q", foundTurn.ID, midTurnID)
	}
	if foundTurn.Content != contentsA[midIdx].Content {
		t.Fatalf("FindTurn content: got %q want %q", foundTurn.Content, contentsA[midIdx].Content)
	}

	// 6. Fork at the midpoint.
	forked, err := store.Fork(sessA.ID, midTurnID)
	if err != nil {
		t.Fatalf("Fork(%s, %s): %v", sessA.ID, midTurnID, err)
	}
	if forked.ID == sessA.ID {
		t.Fatalf("Fork returned same id as source: %q", forked.ID)
	}
	if forked.ForkParentID != midTurnID {
		t.Fatalf("Fork ForkParentID: got %q want %q", forked.ForkParentID, midTurnID)
	}
	if len(forked.Turns) != midIdx+1 {
		t.Fatalf("Fork prefix length: got %d want %d (turns=%+v)", len(forked.Turns), midIdx+1, forked.Turns)
	}
	// Each turn in the fork has a fresh id and parent chain points internally,
	// except the root which carries the source turn id.
	if forked.Turns[0].ParentID != midTurnID {
		t.Fatalf("forked root ParentID: got %q want %q (cross-session link)", forked.Turns[0].ParentID, midTurnID)
	}
	for i, ft := range forked.Turns {
		// New id, not equal to any original id from sessA.
		for j, origID := range idsA {
			if ft.ID == origID {
				t.Fatalf("forked turn %d reused source id %q (source idx %d); fork must rewrite ids", i, origID, j)
			}
		}
		if i == 0 {
			continue
		}
		if ft.ParentID != forked.Turns[i-1].ID {
			t.Fatalf("forked turn %d ParentID: got %q want %q (internal chain)", i, ft.ParentID, forked.Turns[i-1].ID)
		}
	}

	// 7. Diverge the fork: append two new turns; ensure sessA stays untouched.
	forkExtra := []struct {
		Role    string
		Content string
	}{
		{"user", "What if I add embeddings later?"},
		{"assistant", "Plug a strategy into the search method — registry-friendly."},
	}
	lastForkID := forked.Turns[len(forked.Turns)-1].ID
	for i, c := range forkExtra {
		written, err := store.Append(forked, sessions.Turn{
			Role:     c.Role,
			Content:  c.Content,
			ParentID: lastForkID,
		})
		if err != nil {
			t.Fatalf("Append forked turn %d: %v", i, err)
		}
		lastForkID = written.ID
	}

	reloadedA, err := store.Load(sessA.ID)
	if err != nil {
		t.Fatalf("reload sessA: %v", err)
	}
	if got := len(reloadedA.Turns); got != 6 {
		t.Fatalf("sessA must remain 6 turns after fork divergence, got %d", got)
	}
	reloadedFork, err := store.Load(forked.ID)
	if err != nil {
		t.Fatalf("reload forked: %v", err)
	}
	if got := len(reloadedFork.Turns); got != 5 {
		t.Fatalf("forked must have 5 turns (3 prefix + 2 new), got %d", got)
	}
	for _, path := range []string{sessA.Path, forked.Path} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected session file at %s: %v", path, err)
		}
	}

	// 8. Replay from the midpoint via a deterministic responder.
	responder := scriptedResponder{prefix: "replay-reply for "}
	replaySess, replayTurn, err := store.Replay(sessA.ID, midTurnID, sessions.ReplayOptions{
		Model:     "fake-model",
		Params:    map[string]string{"temperature": "0.0"},
		Responder: responder,
	})
	if err != nil {
		t.Fatalf("Replay(%s, %s): %v", sessA.ID, midTurnID, err)
	}
	if replaySess.ID == sessA.ID || replaySess.ID == forked.ID || replaySess.ID == sessB.ID {
		t.Fatalf("Replay reused an existing session id: %q", replaySess.ID)
	}
	wantTurns := midIdx + 1 + 1
	if got := len(replaySess.Turns); got != wantTurns {
		t.Fatalf("Replay session turn count: got %d want %d", got, wantTurns)
	}
	if replayTurn.Role != "assistant" {
		t.Fatalf("Replay turn role: got %q want assistant", replayTurn.Role)
	}
	if replayTurn.Model != "fake-model" {
		t.Fatalf("Replay turn model: got %q want fake-model", replayTurn.Model)
	}
	if got := replayTurn.Metadata["source"]; got != "replay" {
		t.Fatalf("Replay turn metadata[source]: got %q want %q", got, "replay")
	}
	if !strings.HasPrefix(replayTurn.Content, "replay-reply for") {
		t.Fatalf("Replay turn content: got %q want prefix %q", replayTurn.Content, "replay-reply for")
	}
	if got := replayTurn.Params["temperature"]; got != "0.0" {
		t.Fatalf("Replay turn params[temperature]: got %q want 0.0", got)
	}

	// 9. Final List shows all four sessions: sessA, sessB, forked, replay.
	infos, err = store.List()
	if err != nil {
		t.Fatalf("final List: %v", err)
	}
	if len(infos) != 4 {
		ids := make([]string, len(infos))
		for i, info := range infos {
			ids[i] = info.ID
		}
		t.Fatalf("final List count: got %d want 4 (ids=%v)", len(infos), ids)
	}
	// All four expected ids appear.
	seen := map[string]bool{}
	for _, info := range infos {
		seen[info.ID] = true
	}
	for _, want := range []string{sessA.ID, sessB.ID, forked.ID, replaySess.ID} {
		if !seen[want] {
			t.Fatalf("final List missing session id %q: ids=%+v", want, infos)
		}
	}

	// 10. FindTurn on a non-existent id returns wrapped os.ErrNotExist.
	_, _, err = store.FindTurn("turn-does-not-exist")
	if err == nil {
		t.Fatalf("FindTurn(missing): want error, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("FindTurn(missing) error: got %v, want errors.Is(_, os.ErrNotExist)", err)
	}
}

// TestSkillsRegistryJourney loads the registry with a tier conflict and
// asserts precedence resolution, lookup, listing, and search.
func TestSkillsRegistryJourney(t *testing.T) {
	home := newHome(t)

	// Derive the canonical roots so we write to paths the registry will
	// actually scan. DefaultRoots places personal at
	// $HOME/.conduit/skills/personal, not the bare $HOME/.conduit/skills
	// directory newHome creates — so go through the API.
	roots := skills.DefaultRoots(home.Root, home.WorkspaceDir)
	workspaceRoot, ok := roots[contracts.SkillTierWorkspace]
	if !ok {
		t.Fatalf("DefaultRoots missing workspace root; got %+v", roots)
	}
	personalRoot, ok := roots[contracts.SkillTierPersonal]
	if !ok {
		t.Fatalf("DefaultRoots missing personal root; got %+v", roots)
	}

	// Sanity: workspace root should equal the helpers' scaffolded path.
	if workspaceRoot != home.SkillsWorkspace {
		t.Fatalf("workspace root mismatch: helpers %q vs DefaultRoots %q", home.SkillsWorkspace, workspaceRoot)
	}

	// Seed skills.
	workspacePath := writeSkillFile(t, workspaceRoot, "format-go.md", "---\nname: format-go\ndescription: Format Go code\ntags: [go, formatting]\n---\nUse gofmt -s.\n")
	personalConflictPath := writeSkillFile(t, personalRoot, "format-go.md", "---\nname: format-go\ndescription: Personal go-fmt rules\n---\nLocal overrides.\n")
	personalGitPath := writeSkillFile(t, personalRoot, "git-summary.md", "---\nname: git-summary\ndescription: Summarize git diff\ntags: [git]\n---\nRun git diff and summarize.\n")

	// Construct + load.
	reg := skills.NewRegistry(roots)
	if err := reg.Load([]skills.Adapter{skills.NewMarkdownAdapter()}); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Workspace wins for the conflicting name.
	got, ok := reg.Lookup("format-go")
	if !ok {
		t.Fatalf("Lookup format-go: not found")
	}
	if got.Tier != contracts.SkillTierWorkspace {
		t.Fatalf("format-go tier: got %q want %q", got.Tier, contracts.SkillTierWorkspace)
	}
	if got.Path != workspacePath {
		t.Fatalf("format-go winning path: got %q want %q", got.Path, workspacePath)
	}
	if got.Description != "Format Go code" {
		t.Fatalf("format-go description: got %q want %q (workspace should win)", got.Description, "Format Go code")
	}

	// Conflict is recorded with the personal copy as a loser.
	conflicts := reg.Conflicts()
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d: %+v", len(conflicts), conflicts)
	}
	if conflicts[0].Name != "format-go" {
		t.Fatalf("conflict name: got %q want format-go", conflicts[0].Name)
	}
	if conflicts[0].Winner != workspacePath {
		t.Fatalf("conflict winner: got %q want %q", conflicts[0].Winner, workspacePath)
	}
	foundLoser := false
	for _, loser := range conflicts[0].Losers {
		if loser == personalConflictPath {
			foundLoser = true
			break
		}
	}
	if !foundLoser {
		t.Fatalf("conflict losers missing %q: got %v", personalConflictPath, conflicts[0].Losers)
	}

	// git-summary resolves from personal (no conflict).
	gitSkill, ok := reg.Lookup("git-summary")
	if !ok {
		t.Fatalf("Lookup git-summary: not found")
	}
	if gitSkill.Tier != contracts.SkillTierPersonal {
		t.Fatalf("git-summary tier: got %q want %q", gitSkill.Tier, contracts.SkillTierPersonal)
	}
	if gitSkill.Path != personalGitPath {
		t.Fatalf("git-summary path: got %q want %q", gitSkill.Path, personalGitPath)
	}

	// Missing skill.
	if missing, ok := reg.Lookup("nonexistent"); ok {
		t.Fatalf("Lookup nonexistent: expected miss, got %+v", missing)
	}

	// List shows both names, alphabetical.
	listed := reg.List()
	if len(listed) != 2 {
		names := make([]string, len(listed))
		for i, s := range listed {
			names[i] = s.Name
		}
		t.Fatalf("List size: got %d want 2 (names=%v)", len(listed), names)
	}
	if listed[0].Name != "format-go" || listed[1].Name != "git-summary" {
		t.Fatalf("List ordering: got [%s, %s] want [format-go, git-summary]", listed[0].Name, listed[1].Name)
	}

	// Search by tag (case-insensitive) reaches the workspace winner.
	results := reg.Search("GO")
	if len(results) == 0 {
		t.Fatalf("Search GO returned no results")
	}
	sawFormat := false
	for _, r := range results {
		if r.Name == "format-go" {
			sawFormat = true
			if r.Tier != contracts.SkillTierWorkspace {
				t.Fatalf("Search returned non-workspace format-go: tier=%q", r.Tier)
			}
		}
	}
	if !sawFormat {
		names := make([]string, len(results))
		for i, r := range results {
			names[i] = r.Name
		}
		t.Fatalf("Search GO did not contain format-go (got %v)", names)
	}

	// Search by description token finds git-summary only.
	gitResults := reg.Search("summarize")
	if len(gitResults) != 1 || gitResults[0].Name != "git-summary" {
		names := make([]string, len(gitResults))
		for i, r := range gitResults {
			names[i] = r.Name
		}
		t.Fatalf("Search 'summarize': got %v want [git-summary]", names)
	}
}

// ---- helpers ----

// scriptedResponder is a deterministic sessions.ReplayResponder that
// echoes the last turn's content with a fixed prefix.
type scriptedResponder struct {
	prefix string
}

func (r scriptedResponder) Respond(history []sessions.Turn, model string, params map[string]string) (string, error) {
	if len(history) == 0 {
		return r.prefix + "<empty>", nil
	}
	last := history[len(history)-1]
	return fmt.Sprintf("%s%s", r.prefix, last.Content), nil
}

// writeSkillFile materialises a markdown skill file at dir/name.
func writeSkillFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	return mustWrite(t, path, body)
}

// truncateExpected mirrors the private truncate helper in the sessions
// package so tests can assert the Title field deterministically. Keep it
// in lock-step with sessions/store.go truncate(...).
func truncateExpected(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}
