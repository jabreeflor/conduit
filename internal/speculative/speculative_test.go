package speculative

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeDrafter struct {
	rounds [][]string
	calls  int
}

func (d *fakeDrafter) Draft(_ context.Context, _ string, _ []string, _ int) ([]string, error) {
	if d.calls >= len(d.rounds) {
		return nil, nil
	}
	out := d.rounds[d.calls]
	d.calls++
	return out, nil
}

type fakeVerifier struct {
	target  []string // ground truth tokens
	calls   int
	pos     int
	endless bool
}

func (v *fakeVerifier) Verify(_ context.Context, _ string, accepted []string, proposal []string) (int, string, bool, error) {
	v.calls++
	// Accept the longest matching prefix of proposal vs target[v.pos:].
	matched := 0
	for i, p := range proposal {
		if v.pos+i >= len(v.target) {
			break
		}
		if v.target[v.pos+i] != p {
			break
		}
		matched++
	}
	v.pos += matched
	// Verifier always emits the next ground-truth token (if any).
	var next string
	done := false
	if v.pos < len(v.target) {
		next = v.target[v.pos]
		v.pos++
	} else {
		done = !v.endless
	}
	_ = accepted
	return matched, next, done, nil
}

func TestDecodeAcceptsFullDraftWhenCorrect(t *testing.T) {
	drafter := &fakeDrafter{rounds: [][]string{{"hello", "world", "foo", "bar"}}}
	verifier := &fakeVerifier{target: []string{"hello", "world", "foo", "bar"}}
	out, err := Decode(context.Background(), "go", drafter, verifier, Config{Lookahead: 4})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(out.Tokens, " ")
	if got != "hello world foo bar" {
		t.Fatalf("tokens = %q", got)
	}
	if out.AcceptedTokens != 4 {
		t.Fatalf("accepted = %d, want 4", out.AcceptedTokens)
	}
	if rate := out.AcceptanceRate(); rate < 0.99 {
		t.Fatalf("AcceptanceRate = %v, want ~1.0", rate)
	}
}

func TestDecodeRejectsAndResubmits(t *testing.T) {
	drafter := &fakeDrafter{rounds: [][]string{
		{"hello", "WRONG", "tokens"},
		{"foo", "bar"},
	}}
	verifier := &fakeVerifier{target: []string{"hello", "world", "foo", "bar"}}
	out, err := Decode(context.Background(), "go", drafter, verifier, Config{Lookahead: 3})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(out.Tokens, " ")
	if got != "hello world foo bar" {
		t.Fatalf("tokens = %q", got)
	}
	if out.DraftAttempts < 2 {
		t.Fatalf("DraftAttempts = %d, want >= 2", out.DraftAttempts)
	}
}

func TestDecodeBacksOffWhenAcceptanceLow(t *testing.T) {
	// Drafter is always wrong → acceptance rate is 0 → loop should set
	// BackedOff after MinSamplesForBackoff drafted tokens.
	target := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	rounds := make([][]string, 12)
	for i := range rounds {
		rounds[i] = []string{"x", "x", "x"}
	}
	drafter := &fakeDrafter{rounds: rounds}
	verifier := &fakeVerifier{target: target}
	out, err := Decode(context.Background(), "go", drafter, verifier, Config{Lookahead: 3, MinSamplesForBackoff: 6, MinAcceptanceRate: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	if !out.BackedOff {
		t.Fatalf("BackedOff = false, want true")
	}
	if strings.Join(out.Tokens, "") != strings.Join(target, "") {
		t.Fatalf("still produced wrong tokens: %v", out.Tokens)
	}
}

func TestDecodeWithoutDrafterRunsVerifierOnly(t *testing.T) {
	verifier := &fakeVerifier{target: []string{"a", "b", "c"}}
	out, err := Decode(context.Background(), "go", nil, verifier, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(out.Tokens, "") != "abc" {
		t.Fatalf("tokens = %v", out.Tokens)
	}
	if out.DraftAttempts != 0 {
		t.Fatalf("DraftAttempts = %d, want 0", out.DraftAttempts)
	}
}

func TestDecodeRequiresVerifier(t *testing.T) {
	if _, err := Decode(context.Background(), "p", nil, nil, Config{}); err == nil {
		t.Fatal("expected error for nil verifier")
	}
}

func TestDecodeRespectsMaxTokens(t *testing.T) {
	verifier := &fakeVerifier{target: []string{"a", "b", "c", "d", "e", "f", "g", "h"}, endless: true}
	out, err := Decode(context.Background(), "p", nil, verifier, Config{MaxTokens: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Tokens) != 4 {
		t.Fatalf("len = %d, want 4", len(out.Tokens))
	}
}

func TestBatcherCoalescesParallelSubmits(t *testing.T) {
	var calls int32
	var maxBatch int32
	inferer := func(_ context.Context, jobs []BatchJob) ([]BatchOutput, error) {
		atomic.AddInt32(&calls, 1)
		if int32(len(jobs)) > atomic.LoadInt32(&maxBatch) {
			atomic.StoreInt32(&maxBatch, int32(len(jobs)))
		}
		out := make([]BatchOutput, len(jobs))
		for i, j := range jobs {
			out[i] = BatchOutput{ID: j.ID, Output: j.Prompt + "!"}
		}
		return out, nil
	}
	b, err := NewBatcher(BatcherConfig{MaxBatch: 4, MaxWait: 50 * time.Millisecond}, inferer)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make([]BatchOutput, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out, err := b.Submit(context.Background(), BatchJob{ID: string(rune('a' + i)), Prompt: string(rune('a' + i))})
			if err != nil {
				t.Errorf("submit %d: %v", i, err)
				return
			}
			results[i] = out
		}(i)
	}
	wg.Wait()

	for i, r := range results {
		want := string(rune('a'+i)) + "!"
		if r.Output != want {
			t.Errorf("results[%d] = %q, want %q", i, r.Output, want)
		}
	}
	if atomic.LoadInt32(&maxBatch) < 2 {
		t.Fatalf("maxBatch = %d, want >= 2 (coalescing failed)", maxBatch)
	}
	// We should have flushed in fewer calls than requests.
	if atomic.LoadInt32(&calls) >= 6 {
		t.Fatalf("calls = %d, expected coalescing to reduce below 6", calls)
	}
}

func TestBatcherSurfacesInfererError(t *testing.T) {
	b, err := NewBatcher(BatcherConfig{MaxBatch: 1, MaxWait: 50 * time.Millisecond}, func(_ context.Context, _ []BatchJob) ([]BatchOutput, error) {
		return nil, errors.New("oops")
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.Submit(context.Background(), BatchJob{ID: "a", Prompt: "x"})
	if err == nil || err.Error() != "oops" {
		t.Fatalf("err = %v, want oops", err)
	}
}

func TestNewBatcherRequiresInferer(t *testing.T) {
	if _, err := NewBatcher(BatcherConfig{}, nil); err == nil {
		t.Fatal("expected error for nil inferer")
	}
}
