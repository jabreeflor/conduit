package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jabreeflor/conduit/internal/coding"
	"github.com/jabreeflor/conduit/internal/contracts"
	"github.com/jabreeflor/conduit/internal/tools"
	"github.com/jabreeflor/conduit/internal/tools/websearch"
)

// TestCodingAgentFullJourney walks a single `conduit code` session through
// the major user-visible behaviors: tool registration, a happy-path read
// turn, auto-continuation on truncation, a provider error, direct tool
// invocation, and journal-on-disk persistence.
func TestCodingAgentFullJourney(t *testing.T) {
	// ── Stage 1: Bootstrap ────────────────────────────────────────────
	home := newHome(t)

	session, err := coding.NewSessionInDir(home.Root, home.WorkspaceDir)
	if err != nil {
		t.Fatalf("NewSessionInDir: %v", err)
	}

	mainGoPath := filepath.Join(home.WorkspaceDir, "main.go")
	mainGoBody := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello from main\")\n}\n"
	mustWrite(t, mainGoPath, mainGoBody)

	// ── Stage 2: Tool wiring ──────────────────────────────────────────
	tiered := coding.LiveCodingToolsForSession(websearch.Config{}, nil, session)
	registered := coding.RegisterCodingTools(tiered, contracts.CodingPermissions{
		AllowWrite: true,
		AllowShell: true,
	})

	want := map[string]bool{
		"read_file":   false,
		"write_file":  false,
		"edit_file":   false,
		"bash":        false,
		"list_dir":    false,
		"grep_search": false,
	}
	for _, tl := range registered {
		if _, ok := want[tl.Name]; ok {
			want[tl.Name] = true
		}
	}
	for name, present := range want {
		if !present {
			t.Fatalf("expected tool %q to be registered (write+shell perms), got names=%v",
				name, toolNames(registered))
		}
	}

	// ── Stage 3: First turn — read-only ────────────────────────────────
	streamer := NewScripted(
		// Stage 3 reply
		Reply{
			Deltas:       []string{"Reading the file", "..."},
			FinishReason: "stop",
		},
	)

	out := &bytes.Buffer{}
	repl := &coding.REPL{
		Session:         session,
		Budget:          coding.NewBudget(20000),
		Tools:           registered,
		Streamer:        streamer,
		In:              strings.NewReader("please summarize main.go\n"),
		Out:             out,
		MaxAutoContinue: 2,
	}

	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	if err := repl.Run(ctx1); err != nil {
		cancel1()
		t.Fatalf("REPL.Run stage 3: %v", err)
	}
	cancel1()

	outStr := out.String()
	if !strings.Contains(outStr, "Reading the file") {
		t.Errorf("stage 3: expected delta %q in Out, got %q", "Reading the file", outStr)
	}
	if !strings.Contains(outStr, "...") {
		t.Errorf("stage 3: expected delta %q in Out, got %q", "...", outStr)
	}

	entries := readJSONL(t, session.Path)
	if len(entries) != 2 {
		t.Fatalf("stage 3: expected 2 turns on disk, got %d (entries=%+v)", len(entries), entries)
	}
	if got := roleAt(entries, 0); got != "user" {
		t.Errorf("stage 3: turn[0] role: want %q, got %q", "user", got)
	}
	if got := roleAt(entries, 1); got != "assistant" {
		t.Errorf("stage 3: turn[1] role: want %q, got %q", "assistant", got)
	}
	if got := contentAt(entries, 0); got != "please summarize main.go" {
		t.Errorf("stage 3: user content: want %q, got %q", "please summarize main.go", got)
	}
	if got := contentAt(entries, 1); !strings.Contains(got, "Reading the file") {
		t.Errorf("stage 3: assistant content should contain delta, got %q", got)
	}

	// ── Stage 4: Truncated reply auto-continuation ────────────────────
	streamer.mu.Lock()
	streamer.Replies = append(streamer.Replies,
		Reply{
			Deltas:       []string{"partial-chunk-A"},
			FinishReason: "length",
		},
		Reply{
			Deltas:       []string{"final-chunk-B"},
			FinishReason: "stop",
		},
	)
	priorCallCount := len(streamer.calls)
	streamer.mu.Unlock()

	// New REPL for stage 4 with fresh stdin; reuses session/streamer.
	out2 := &bytes.Buffer{}
	repl2 := &coding.REPL{
		Session:         session,
		Budget:          coding.NewBudget(20000),
		Tools:           registered,
		Streamer:        streamer,
		In:              strings.NewReader("write more about main.go\n"),
		Out:             out2,
		MaxAutoContinue: 2,
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	if err := repl2.Run(ctx2); err != nil {
		cancel2()
		t.Fatalf("REPL.Run stage 4: %v", err)
	}
	cancel2()

	out2Str := out2.String()
	if !strings.Contains(out2Str, "partial-chunk-A") {
		t.Errorf("stage 4: expected partial chunk in Out, got %q", out2Str)
	}
	if !strings.Contains(out2Str, "final-chunk-B") {
		t.Errorf("stage 4: expected final chunk in Out, got %q", out2Str)
	}

	calls := streamer.Calls()
	stage4Calls := calls[priorCallCount:]
	if len(stage4Calls) != 2 {
		t.Fatalf("stage 4: expected 2 streamer calls (initial + 1 auto-continue), got %d (prompts=%v)",
			len(stage4Calls), promptsOf(stage4Calls))
	}
	if stage4Calls[0].Prompt != "write more about main.go" {
		t.Errorf("stage 4: first prompt: want %q, got %q",
			"write more about main.go", stage4Calls[0].Prompt)
	}
	if stage4Calls[1].Prompt != "continue" {
		t.Errorf("stage 4: second prompt: want %q, got %q",
			"continue", stage4Calls[1].Prompt)
	}

	entries = readJSONL(t, session.Path)
	// Stage 3 wrote 2 (user+assistant); stage 4 writes 1 user + 2 assistant (length+continuation) = 3.
	// Total expected: 5.
	if len(entries) != 5 {
		t.Fatalf("stage 4: expected 5 total turns on disk, got %d: roles=%v",
			len(entries), rolesOf(entries))
	}

	// ── Stage 5: Provider error ───────────────────────────────────────
	streamer.mu.Lock()
	streamer.Replies = append(streamer.Replies, Reply{Err: errFakeProvider})
	streamer.mu.Unlock()

	out3 := &bytes.Buffer{}
	repl3 := &coding.REPL{
		Session:         session,
		Budget:          coding.NewBudget(20000),
		Tools:           registered,
		Streamer:        streamer,
		In:              strings.NewReader("trigger error\n"),
		Out:             out3,
		MaxAutoContinue: 2,
	}
	ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
	err = repl3.Run(ctx3)
	cancel3()
	if err == nil {
		t.Fatalf("stage 5: expected provider error to propagate, got nil")
	}
	if !errors.Is(err, errFakeProvider) {
		t.Fatalf("stage 5: expected errFakeProvider, got %v", err)
	}

	// ── Stage 6: Direct tool exercise ─────────────────────────────────
	writeTool := findTool(t, registered, "write_file")
	readTool := findTool(t, registered, "read_file")

	scratchPath := filepath.Join(home.WorkspaceDir, "scratch.txt")
	scratchBody := "hello-from-direct-tool-invocation\n"
	writeArgs, _ := json.Marshal(map[string]any{
		"path":    scratchPath,
		"content": scratchBody,
	})
	ctx6a, cancel6a := context.WithTimeout(context.Background(), 5*time.Second)
	wRes, wErr := writeTool.Run(ctx6a, writeArgs)
	cancel6a()
	if wErr != nil {
		t.Fatalf("stage 6: write_file run error: %v", wErr)
	}
	if wRes.IsError {
		t.Fatalf("stage 6: write_file reported IsError: %s", wRes.Text)
	}
	// Sanity-check disk side effect.
	gotOnDisk, err := os.ReadFile(scratchPath)
	if err != nil {
		t.Fatalf("stage 6: scratch.txt not created: %v", err)
	}
	if string(gotOnDisk) != scratchBody {
		t.Errorf("stage 6: scratch.txt content: want %q, got %q", scratchBody, string(gotOnDisk))
	}

	readArgs, _ := json.Marshal(map[string]any{"path": scratchPath})
	ctx6b, cancel6b := context.WithTimeout(context.Background(), 5*time.Second)
	rRes, rErr := readTool.Run(ctx6b, readArgs)
	cancel6b()
	if rErr != nil {
		t.Fatalf("stage 6: read_file run error: %v", rErr)
	}
	if rRes.IsError {
		t.Fatalf("stage 6: read_file reported IsError: %s", rRes.Text)
	}
	if !strings.Contains(rRes.Text, "hello-from-direct-tool-invocation") {
		t.Errorf("stage 6: read_file output should contain written content, got %q", rRes.Text)
	}

	// ── Stage 7: Session restart / persistence ────────────────────────
	// After error, no new turn was appended for stage 5 (streamer returned
	// before assistant Append). But a user turn for "trigger error" WAS
	// appended before streamAssistant. So total = 5 (stage3+4) + 1 = 6.
	entries = readJSONL(t, session.Path)
	if len(entries) != len(session.Turns) {
		t.Errorf("stage 7: JSONL count %d != in-memory Turns count %d",
			len(entries), len(session.Turns))
	}
	for i, e := range entries {
		jsonRole := roleAt([]map[string]any{e}, 0)
		memRole := session.Turns[i].Role
		if jsonRole != memRole {
			t.Errorf("stage 7: turn[%d] role mismatch: jsonl=%q memory=%q",
				i, jsonRole, memRole)
		}
	}

	// Exactly one .jsonl file should exist in the coding-sessions dir.
	dirEntries, err := os.ReadDir(home.CodingSessions)
	if err != nil {
		t.Fatalf("stage 7: read coding-sessions: %v", err)
	}
	jsonlCount := 0
	var jsonlName string
	for _, de := range dirEntries {
		if strings.HasSuffix(de.Name(), ".jsonl") {
			jsonlCount++
			jsonlName = de.Name()
		}
	}
	if jsonlCount != 1 {
		t.Fatalf("stage 7: expected exactly 1 .jsonl file, got %d (entries=%v)",
			jsonlCount, dirEntries)
	}
	if !strings.HasPrefix(jsonlName, strings.TrimSuffix(filepath.Base(session.Path), ".jsonl")) {
		t.Errorf("stage 7: jsonl name %q should match session id %q",
			jsonlName, filepath.Base(session.Path))
	}
}

// ── local helpers ────────────────────────────────────────────────────────

// findTool returns the tool by name from the slice, failing the test if absent.
func findTool(t *testing.T, ts []tools.Tool, name string) tools.Tool {
	t.Helper()
	for _, tl := range ts {
		if tl.Name == name {
			return tl
		}
	}
	t.Fatalf("tool %q not found in slice %v", name, toolNames(ts))
	return tools.Tool{}
}

func toolNames(ts []tools.Tool) []string {
	out := make([]string, 0, len(ts))
	for _, tl := range ts {
		out = append(out, tl.Name)
	}
	return out
}

// roleAt extracts the Role field from a JSONL-decoded turn entry.
// CodingTurn has no json tags, so encoding/json marshals "Role" verbatim.
func roleAt(entries []map[string]any, i int) string {
	if i >= len(entries) {
		return ""
	}
	if v, ok := entries[i]["Role"].(string); ok {
		return v
	}
	if v, ok := entries[i]["role"].(string); ok {
		return v
	}
	return ""
}

// contentAt extracts the Content field similarly.
func contentAt(entries []map[string]any, i int) string {
	if i >= len(entries) {
		return ""
	}
	if v, ok := entries[i]["Content"].(string); ok {
		return v
	}
	if v, ok := entries[i]["content"].(string); ok {
		return v
	}
	return ""
}

func rolesOf(entries []map[string]any) []string {
	out := make([]string, len(entries))
	for i := range entries {
		out[i] = roleAt(entries, i)
	}
	return out
}

func promptsOf(calls []ScriptedCall) []string {
	out := make([]string, len(calls))
	for i, c := range calls {
		out[i] = c.Prompt
	}
	return out
}
