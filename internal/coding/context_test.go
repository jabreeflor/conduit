package coding

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
)

func TestBudgetThresholdBoundary(t *testing.T) {
	b := NewBudget(1000)
	// 79% — below trigger.
	b.Observe(790, 0)
	if b.ShouldCompact() {
		t.Fatal("compaction triggered at 79% of window")
	}
	// 80% — at trigger.
	b.Observe(10, 0)
	if !b.ShouldCompact() {
		t.Fatal("compaction did not trigger at 80% of window")
	}
}

func TestBudgetReactiveFlag(t *testing.T) {
	b := NewBudget(1_000_000)
	if b.ShouldCompact() {
		t.Fatal("fresh budget should not compact")
	}
	b.MarkPromptTooLong()
	if !b.ShouldCompact() {
		t.Fatal("reactive flag should force compaction even at 0% usage")
	}
}

func TestBudgetReset(t *testing.T) {
	b := NewBudget(100)
	b.Observe(80, 50)
	b.MarkPromptTooLong()
	if !b.ShouldCompact() {
		t.Fatal("expected compact before reset")
	}
	b.Reset()
	if b.ShouldCompact() {
		t.Fatal("reset should clear compaction trigger")
	}
	snap := b.Snapshot()
	if snap.UsedInput != 0 || snap.UsedOutput != 0 || snap.PromptTooLong {
		t.Fatalf("reset did not clear state: %+v", snap)
	}
}

func TestBudgetWithThreshold(t *testing.T) {
	b := NewBudget(1000).WithThreshold(0.5)
	b.Observe(499, 0)
	if b.ShouldCompact() {
		t.Fatal("compaction triggered below custom threshold")
	}
	b.Observe(1, 0)
	if !b.ShouldCompact() {
		t.Fatal("compaction did not trigger at custom threshold")
	}
}

func TestCollapsePastesShortPassesThrough(t *testing.T) {
	in := "hello world\n\nthis is short"
	out, chips := CollapsePastes(in, 500)
	if out != in {
		t.Fatalf("short input mutated:\n got %q\nwant %q", out, in)
	}
	if len(chips) != 0 {
		t.Fatalf("expected zero chips, got %d", len(chips))
	}
}

func TestCollapsePastesLongBlockReplaced(t *testing.T) {
	long := strings.Repeat("a", 600)
	in := "preface\n\n" + long + "\n\nepilogue"
	out, chips := CollapsePastes(in, 500)
	if len(chips) != 1 {
		t.Fatalf("expected 1 chip, got %d", len(chips))
	}
	if !strings.Contains(out, "[paste:"+chips[0].ID+"]") {
		t.Fatalf("output missing chip placeholder: %q", out)
	}
	if strings.Contains(out, long) {
		t.Fatal("long block was not removed from output")
	}
	if !strings.Contains(out, "preface") || !strings.Contains(out, "epilogue") {
		t.Fatalf("surrounding context lost: %q", out)
	}
	if chips[0].Length != len(long) {
		t.Errorf("chip length: got %d want %d", chips[0].Length, len(long))
	}
	if len(chips[0].Sha256Prefix8) != 8 {
		t.Errorf("expected 8-char hash prefix, got %q", chips[0].Sha256Prefix8)
	}
}

func TestCollapsePastesMultipleBlocksDistinctIDs(t *testing.T) {
	a := strings.Repeat("a", 600)
	b := strings.Repeat("b", 600)
	in := a + "\n\n" + b
	_, chips := CollapsePastes(in, 500)
	if len(chips) != 2 {
		t.Fatalf("expected 2 chips, got %d", len(chips))
	}
	if chips[0].ID == chips[1].ID {
		t.Fatalf("expected distinct chip IDs, got %q twice", chips[0].ID)
	}
}

func TestCollapsePastesDefaultThreshold(t *testing.T) {
	long := strings.Repeat("x", 500)
	_, chips := CollapsePastes(long, 0)
	if len(chips) != 1 {
		t.Fatalf("expected default 500-char threshold to collapse, got %d chips", len(chips))
	}
}

func TestEstimateTokens(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"abc", 1},        // 3 chars → (3+3)/4 = 1
		{"abcd", 1},       // 4 chars → (4+3)/4 = 1
		{"abcde", 2},      // 5 chars → (5+3)/4 = 2
		{strings.Repeat("a", 100), 25}, // 100 chars → 25
	}
	for _, tc := range cases {
		got := EstimateTokens(tc.input)
		if got != tc.want {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestExpandChipsRoundTrip(t *testing.T) {
	long := strings.Repeat("x", 600)
	collapsed, chips := CollapsePastes(long, 500)
	if len(chips) != 1 {
		t.Fatalf("expected 1 chip, got %d", len(chips))
	}
	expanded := ExpandChips(collapsed, chips)
	if expanded != long {
		t.Fatalf("round-trip mismatch:\n got %q\nwant %q", expanded[:min(50, len(expanded))], long[:min(50, len(long))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestExpandChipsNoMatch(t *testing.T) {
	s := "[paste:unknown-id]"
	out := ExpandChips(s, nil)
	if out != s {
		t.Fatalf("no-match chip should pass through unchanged, got %q", out)
	}
}

func TestBudgetSnip(t *testing.T) {
	// Budget with a small window so we can trigger snipping.
	b := NewBudget(40).WithThreshold(1.0) // limit = 40 tokens
	turns := []contracts.CodingTurn{
		{Role: "system", Content: strings.Repeat("a", 4)},    // 1 token (always kept)
		{Role: "user", Content: strings.Repeat("b", 80)},     // 20 tokens
		{Role: "assistant", Content: strings.Repeat("c", 80)}, // 20 tokens
		{Role: "user", Content: strings.Repeat("d", 80)},     // 20 tokens
	}
	result := b.Snip(turns, 0)
	// First turn must always be preserved.
	if result[0].Role != "system" {
		t.Fatal("first turn (system) was dropped")
	}
	// Total should now be under limit.
	total := 0
	for _, tr := range result {
		total += EstimateTokens(tr.Content)
	}
	if total >= 40 {
		t.Fatalf("snip did not reduce tokens: total=%d, limit=40", total)
	}
}

func TestBudgetSnipPreservesFirstWhenAlreadyFit(t *testing.T) {
	b := NewBudget(1000)
	turns := []contracts.CodingTurn{
		{Role: "user", Content: "short"},
		{Role: "assistant", Content: "reply"},
	}
	result := b.Snip(turns, 0)
	if len(result) != 2 {
		t.Fatalf("snip dropped turns when already under budget: got %d", len(result))
	}
}

func TestDefaultCompactor(t *testing.T) {
	c := DefaultCompactor{}
	summary, err := c.Compact(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
}

func TestRunCompactionResetsAndReplaces(t *testing.T) {
	b := NewBudget(1000)
	b.Observe(500, 100)
	turns := []contracts.CodingTurn{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	result, err := b.RunCompaction(context.Background(), turns, DefaultCompactor{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 compacted turn, got %d", len(result))
	}
	if result[0].Role != "assistant" {
		t.Fatalf("compacted turn should be assistant, got %q", result[0].Role)
	}
	snap := b.Snapshot()
	if snap.UsedInput != 0 || snap.UsedOutput != 0 {
		t.Fatalf("budget not reset after compaction: %+v", snap)
	}
}

func TestPreflight(t *testing.T) {
	b := NewBudget(1000)
	b.Observe(800, 0) // 80% used → ShouldCompact = true

	prompt := strings.Repeat("a", 40) // ~10 tokens
	result := Preflight(b, prompt)

	if !result.ShouldCompact {
		t.Fatal("expected ShouldCompact = true at 80% usage")
	}
	if result.EstimatedTokens != EstimateTokens(prompt) {
		t.Fatalf("EstimatedTokens mismatch: got %d", result.EstimatedTokens)
	}
}

func TestPreflightOverBudget(t *testing.T) {
	b := NewBudget(100)
	b.Observe(95, 0) // 95 used
	// Prompt that pushes us over the 100-token window.
	prompt := strings.Repeat("a", 24) // 6 tokens → 95+6=101 > 100
	result := Preflight(b, prompt)
	if !result.OverBudget {
		t.Fatalf("expected OverBudget=true (95+6 > 100)")
	}
}

func TestDiscoverContextFiles(t *testing.T) {
	// Build a temp tree:
	//   root/
	//     CLAUDE.md
	//     project/
	//       CLAUDE.md
	//       .conduit/rules/rule1.md
	tmp := t.TempDir()
	projectRoot := filepath.Join(tmp, "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".conduit", "rules"), 0o700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(tmp, "CLAUDE.md"), []byte("root context"), 0o600)
	os.WriteFile(filepath.Join(projectRoot, "CLAUDE.md"), []byte("project context"), 0o600)
	os.WriteFile(filepath.Join(projectRoot, ".conduit", "rules", "rule1.md"), []byte("rule1"), 0o600)

	files, err := DiscoverContextFiles(projectRoot, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) < 2 {
		t.Fatalf("expected at least 2 context files, got %d: %v", len(files), files)
	}
	// First file should be project-local CLAUDE.md.
	if files[0].Content != "project context" {
		t.Fatalf("first file should be project CLAUDE.md, got %q", files[0].Content)
	}
}
