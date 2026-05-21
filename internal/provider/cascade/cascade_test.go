package cascade

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

func TestClassifyTrivialAndComplex(t *testing.T) {
	trivial := Classify("define dependency injection", Signals{})
	if trivial.Complexity != ComplexityTrivial {
		t.Fatalf("trivial classification = %s (signals=%v score=%d), want trivial", trivial.Complexity, trivial.Signals, trivial.Score)
	}

	complex := Classify("Refactor internal/router to support speculative decoding and analyze the trade-offs across cmd/conduit/main.go and internal/router/router.go step by step.", Signals{TurnCount: 12})
	if complex.Complexity != ComplexityComplex {
		t.Fatalf("complex classification = %s (signals=%v score=%d), want complex", complex.Complexity, complex.Signals, complex.Score)
	}
}

func TestClassifyCodeBlockBumpsToModerate(t *testing.T) {
	prompt := "Here is the snippet:\n```go\nfunc add(a, b int) int { return a + b }\n```\nWhat does it do?"
	c := Classify(prompt, Signals{})
	if c.Complexity != ComplexityModerate && c.Complexity != ComplexitySimple {
		t.Fatalf("classification = %s, want moderate or simple", c.Complexity)
	}
	hasCode := false
	for _, s := range c.Signals {
		if s == "code_block" {
			hasCode = true
		}
	}
	if !hasCode {
		t.Fatalf("expected code_block signal, got %v", c.Signals)
	}
}

func TestCascadeStartsAtCheapestHandlingTier(t *testing.T) {
	tiers := []Tier{
		{Name: "tiny", Handles: []Complexity{ComplexityTrivial}, Cost: 0.1, MinConfidence: 0.5},
		{Name: "small", Handles: []Complexity{ComplexitySimple}, Cost: 0.5, MinConfidence: 0.5},
		{Name: "big", Handles: []Complexity{ComplexityModerate, ComplexityComplex}, Cost: 5, MinConfidence: 0.5},
	}
	c, err := New(tiers)
	if err != nil {
		t.Fatal(err)
	}

	called := []string{}
	infer := func(_ context.Context, t Tier) (Result, error) {
		called = append(called, t.Name)
		return Result{Text: strings.Repeat("good answer ", 5)}, nil
	}

	out, err := c.Run(context.Background(), "define cascading inference", Signals{}, infer)
	if err != nil {
		t.Fatal(err)
	}
	if out.Tier.Name != "tiny" {
		t.Fatalf("Tier = %s, want tiny", out.Tier.Name)
	}
	if len(called) != 1 || called[0] != "tiny" {
		t.Fatalf("called = %v, want [tiny]", called)
	}
	if out.Escalated {
		t.Fatalf("Escalated = true, want false")
	}
}

func TestCascadeEscalatesOnLowQuality(t *testing.T) {
	tiers := []Tier{
		{Name: "tiny", Handles: nil, Cost: 0.1, MinConfidence: 0.6},
		{Name: "big", Handles: nil, Cost: 5, MinConfidence: 0.6},
	}
	c, err := New(tiers, WithQualityFn(func(t Tier, r Result) float64 {
		if t.Name == "tiny" {
			return 0.2
		}
		return 0.9
	}))
	if err != nil {
		t.Fatal(err)
	}

	called := []string{}
	infer := func(_ context.Context, t Tier) (Result, error) {
		called = append(called, t.Name)
		return Result{Text: "ok"}, nil
	}

	out, err := c.Run(context.Background(), "anything", Signals{}, infer)
	if err != nil {
		t.Fatal(err)
	}
	if out.Tier.Name != "big" {
		t.Fatalf("Tier = %s, want big", out.Tier.Name)
	}
	if !out.Escalated {
		t.Fatalf("Escalated = false, want true")
	}
	if len(called) != 2 {
		t.Fatalf("called = %v, want [tiny big]", called)
	}
	if len(out.Attempts) != 2 || !out.Attempts[0].Escalated {
		t.Fatalf("Attempts = %#v, want first escalated", out.Attempts)
	}
}

func TestCascadeEscalatesOnError(t *testing.T) {
	tiers := []Tier{
		{Name: "tiny", Cost: 0.1, MinConfidence: 0.5},
		{Name: "big", Cost: 5, MinConfidence: 0.5},
	}
	c, err := New(tiers)
	if err != nil {
		t.Fatal(err)
	}

	infer := func(_ context.Context, t Tier) (Result, error) {
		if t.Name == "tiny" {
			return Result{}, errors.New("provider down")
		}
		return Result{Text: strings.Repeat("answer ", 5)}, nil
	}
	out, err := c.Run(context.Background(), "anything", Signals{}, infer)
	if err != nil {
		t.Fatal(err)
	}
	if out.Tier.Name != "big" {
		t.Fatalf("Tier = %s, want big", out.Tier.Name)
	}
	if out.Attempts[0].Err == "" {
		t.Fatalf("Attempts[0].Err is empty, want provider error")
	}
}

func TestCascadeBudgetGateHaltsEscalation(t *testing.T) {
	tiers := []Tier{
		{Name: "tiny", Cost: 0.1, MinConfidence: 0.99},
		{Name: "big", Cost: 5, MinConfidence: 0.5},
	}
	c, err := New(tiers,
		WithBudgetGate(func(t Tier) bool { return t.Cost < 1 }),
	)
	if err != nil {
		t.Fatal(err)
	}

	infer := func(_ context.Context, _ Tier) (Result, error) {
		return Result{Text: strings.Repeat("ok ", 30)}, nil
	}
	out, err := c.Run(context.Background(), "anything", Signals{}, infer)
	if err != nil {
		t.Fatal(err)
	}
	if !out.BudgetStopped {
		t.Fatalf("BudgetStopped = false, want true")
	}
	// degraded — last successful was tiny
	if out.Tier.Name != "tiny" {
		t.Fatalf("Tier = %s, want tiny (degraded)", out.Tier.Name)
	}
}

func TestCascadeBatchFansOut(t *testing.T) {
	tiers := []Tier{{Name: "tiny", Cost: 0.1, MinConfidence: 0.1}}
	c, err := New(tiers)
	if err != nil {
		t.Fatal(err)
	}

	var inflight, peak int32
	infer := func(_ context.Context, _ Tier) (Result, error) {
		cur := atomic.AddInt32(&inflight, 1)
		for {
			p := atomic.LoadInt32(&peak)
			if cur <= p || atomic.CompareAndSwapInt32(&peak, p, cur) {
				break
			}
		}
		atomic.AddInt32(&inflight, -1)
		return Result{Text: "answer text long enough"}, nil
	}

	results := c.Batch(context.Background(), []string{"a", "b", "c", "d"}, nil, 2, infer)
	if len(results) != 4 {
		t.Fatalf("results = %d, want 4", len(results))
	}
	for i, r := range results {
		if r.Err != nil {
			t.Fatalf("results[%d] err = %v", i, r.Err)
		}
		if r.Outcome.Tier.Name != "tiny" {
			t.Fatalf("results[%d] tier = %s", i, r.Outcome.Tier.Name)
		}
	}
}

func TestNewRequiresAtLeastOneTier(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatal("expected error for empty tiers")
	}
	if _, err := New([]Tier{{Cost: 1}}); err == nil {
		t.Fatal("expected error for unnamed tier")
	}
}

func TestDefaultQualityRefusalScoreLow(t *testing.T) {
	q := defaultQuality(Tier{}, Result{Text: "I cannot help with that, sorry."})
	if q >= 0.5 {
		t.Fatalf("quality = %v, want low (refusal)", q)
	}
}

func TestDefaultQualityHonorsExplicitConfidence(t *testing.T) {
	q := defaultQuality(Tier{}, Result{Text: "x", Confidence: 0.42})
	if q != 0.42 {
		t.Fatalf("quality = %v, want 0.42", q)
	}
}
