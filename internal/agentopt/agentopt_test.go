package agentopt

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBatchExecutorRunsInParallelAndPreservesOrder(t *testing.T) {
	exec := NewBatchExecutor(3)
	var inflight, peak int32
	gate := make(chan struct{})

	requests := []BatchRequest{
		{ID: "a", Input: 1},
		{ID: "b", Input: 2},
		{ID: "c", Input: 3},
		{ID: "d", Input: 4},
		{ID: "e", Input: 5},
	}
	go func() {
		// Release workers once at least 3 are simultaneously in flight.
		for atomic.LoadInt32(&inflight) < 3 {
			time.Sleep(time.Millisecond)
		}
		close(gate)
	}()
	out := exec.Run(context.Background(), requests, func(_ context.Context, req BatchRequest) (any, error) {
		cur := atomic.AddInt32(&inflight, 1)
		for {
			p := atomic.LoadInt32(&peak)
			if cur <= p || atomic.CompareAndSwapInt32(&peak, p, cur) {
				break
			}
		}
		<-gate
		atomic.AddInt32(&inflight, -1)
		return req.Input.(int) * 10, nil
	})

	if peak < 2 {
		t.Fatalf("peak inflight = %d, want >= 2", peak)
	}
	for i, r := range out {
		if r.ID != requests[i].ID {
			t.Fatalf("out[%d] ID = %s, want %s", i, r.ID, requests[i].ID)
		}
		if r.Output.(int) != requests[i].Input.(int)*10 {
			t.Fatalf("out[%d] output = %v", i, r.Output)
		}
	}
}

func TestBatchExecutorPropagatesPerItemErrors(t *testing.T) {
	exec := NewBatchExecutor(0)
	out := exec.Run(context.Background(), []BatchRequest{{ID: "a"}, {ID: "b"}}, func(_ context.Context, req BatchRequest) (any, error) {
		if req.ID == "a" {
			return nil, errors.New("boom")
		}
		return "ok", nil
	})
	if out[0].Err == nil || out[1].Err != nil {
		t.Fatalf("unexpected errs: %v / %v", out[0].Err, out[1].Err)
	}
}

func TestEarlyTerminationStopsAtCompleteJSON(t *testing.T) {
	et := EarlyTermination{Done: JSONObjectComplete}
	stream := []string{"{\"a\":", " \"hello\"", "}", " trailing junk"}
	buf := ""
	stops := 0
	for _, tok := range stream {
		var stop bool
		buf, stop = et.Consume(buf, tok)
		if stop {
			stops++
			break
		}
	}
	if stops != 1 {
		t.Fatalf("stops = %d, want 1", stops)
	}
	if !strings.HasSuffix(buf, "}") {
		t.Fatalf("buf = %q, want to end at closing brace", buf)
	}
}

func TestJSONObjectCompleteHandlesNestedAndStrings(t *testing.T) {
	cases := map[string]bool{
		"{}":               true,
		`{"a":"}"}`:        true, // brace inside string doesn't close
		`{"a":{"b":1}}`:    true,
		`{"a":1`:           false,
		`{"a":"\"}"}`:      true, // escaped quote
		`not json {really`: false,
	}
	for in, want := range cases {
		if got := JSONObjectComplete(in); got != want {
			t.Errorf("JSONObjectComplete(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestPlanValidateRejectsCycle(t *testing.T) {
	p := Plan{Steps: []PlanStep{
		{ID: "a", DependsOn: []string{"b"}},
		{ID: "b", DependsOn: []string{"a"}},
	}}
	if err := p.Validate(); err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestPlanTopologicalOrder(t *testing.T) {
	p := Plan{Steps: []PlanStep{
		{ID: "build"},
		{ID: "test", DependsOn: []string{"build"}},
		{ID: "deploy", DependsOn: []string{"test"}},
		{ID: "lint"},
	}}
	order, err := p.TopologicalOrder()
	if err != nil {
		t.Fatal(err)
	}
	pos := map[string]int{}
	for i, id := range order {
		pos[id] = i
	}
	if pos["build"] >= pos["test"] || pos["test"] >= pos["deploy"] {
		t.Fatalf("order does not respect deps: %v", order)
	}
	if len(order) != 4 {
		t.Fatalf("order = %v, want 4 elements", order)
	}
}

func TestApplyEditsAppliesSequentially(t *testing.T) {
	original := "alpha\nbeta\ngamma\n"
	out, err := ApplyEdits(original, []Edit{
		{OldString: "alpha", NewString: "ALPHA"},
		{OldString: "gamma", NewString: "GAMMA"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != "ALPHA\nbeta\nGAMMA\n" {
		t.Fatalf("got %q", out)
	}
}

func TestApplyEditsFailsOnAmbiguity(t *testing.T) {
	_, err := ApplyEdits("ab ab ab", []Edit{{OldString: "ab", NewString: "XY"}})
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
}

func TestApplyEditsHonorsOccurrenceAndReplaceAll(t *testing.T) {
	got, err := ApplyEdits("ab ab ab", []Edit{{OldString: "ab", NewString: "XY", Occurrence: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if got != "ab XY ab" {
		t.Fatalf("Occurrence=2 got %q", got)
	}
	got, err = ApplyEdits("ab ab ab", []Edit{{OldString: "ab", NewString: "Z", ReplaceAll: true}})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Z Z Z" {
		t.Fatalf("ReplaceAll got %q", got)
	}
}

func TestApplyEditsFailsOnMissing(t *testing.T) {
	if _, err := ApplyEdits("hi", []Edit{{OldString: "missing", NewString: "x"}}); err == nil {
		t.Fatal("expected missing error")
	}
	if _, err := ApplyEdits("hi", []Edit{{OldString: "hi", NewString: "hi"}}); err == nil {
		t.Fatal("expected identical-strings error")
	}
}

func TestEditTokenSavingsPositiveForSmallEdits(t *testing.T) {
	rewritten := strings.Repeat("xxxx ", 1000)
	edits := []Edit{{OldString: "yes", NewString: "no"}}
	if savings := EditTokenSavings("orig", rewritten, edits); savings <= 0 {
		t.Fatalf("savings = %d, want > 0", savings)
	}
}

func TestCompactDropsOldestUnpinnedUntilFits(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: strings.Repeat("s", 40), Pinned: true},
		{Role: "user", Content: strings.Repeat("u", 400)},
		{Role: "assistant", Content: strings.Repeat("a", 400)},
		{Role: "user", Content: strings.Repeat("u", 400)},
		{Role: "assistant", Content: strings.Repeat("a", 400)},
		{Role: "user", Content: strings.Repeat("u", 40)},
		{Role: "assistant", Content: strings.Repeat("a", 40)},
	}
	policy := CompactionPolicy{
		MaxTokens:  120,
		KeepRecent: 2,
		SummaryFn: func(dropped []Message) Message {
			return Message{Role: "system", Content: "summary", Pinned: true}
		},
	}
	out, dropped := Compact(msgs, policy)
	if dropped == 0 {
		t.Fatalf("dropped = 0, want > 0")
	}
	// system pinned + summary + at least KeepRecent recent messages
	if out[0].Content != "summary" {
		t.Fatalf("expected summary at head, got %#v", out[0])
	}
	last := out[len(out)-1]
	if last.Content != msgs[len(msgs)-1].Content {
		t.Fatalf("recent tail not preserved: %#v", last)
	}
	// system message still present somewhere
	foundSystem := false
	for _, m := range out {
		if m.Role == "system" && len(m.Content) == 40 {
			foundSystem = true
		}
	}
	if !foundSystem {
		t.Fatalf("pinned system message was dropped")
	}
}

func TestCompactNoOpWhenUnderBudget(t *testing.T) {
	msgs := []Message{{Role: "user", Content: "hi"}}
	out, dropped := Compact(msgs, CompactionPolicy{MaxTokens: 1000})
	if dropped != 0 || len(out) != 1 {
		t.Fatalf("unexpected compaction: dropped=%d out=%v", dropped, out)
	}
}
