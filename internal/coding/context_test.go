package coding

import (
	"strings"
	"testing"
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
