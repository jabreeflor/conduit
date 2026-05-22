package cascade

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Tier names a model rung in the cascade. Lower Cost values are tried first.
type Tier struct {
	// Name is a router model/provider name (passed through to Inferer).
	Name string
	// Handles is the set of complexities this tier should handle without
	// further escalation. A request whose Complexity is in this set will skip
	// cheaper tiers below it on the way up; a request whose complexity is
	// strictly greater than every entry will still try this tier on the way to
	// a stronger one.
	Handles []Complexity
	// Cost is a relative cost weight (USD per 1k tokens, or any monotonically
	// increasing scalar) used to order tiers cheapest-to-most-expensive.
	Cost float64
	// MinConfidence is the minimum acceptable Quality score for a result
	// produced by this tier. Results below the threshold trigger escalation.
	MinConfidence float64
}

// Result is the outcome of a single tier attempt, supplied by the caller.
type Result struct {
	// Text is the generated content (used by default Quality scorer).
	Text string
	// Confidence is an optional caller-supplied confidence in [0,1]. When
	// non-zero it overrides the default heuristic Quality scorer.
	Confidence float64
	// Provider/Model are passed through to the final returned Outcome.
	Provider string
	Model    string
	// Extra is opaque metadata attached to the final outcome.
	Extra map[string]string
}

// Inferer runs one tier and returns its Result. Implementations should respect
// ctx and return ctx.Err() promptly.
type Inferer func(ctx context.Context, tier Tier) (Result, error)

// QualityFn scores a Result in [0,1]. Returning a value below tier.MinConfidence
// triggers escalation.
type QualityFn func(Tier, Result) float64

// BudgetGate is consulted before each tier attempt. Returning false halts
// escalation (the caller has hit a budget warning/critical threshold and wants
// to degrade rather than escalate further).
type BudgetGate func(tier Tier) bool

// Outcome is the final cascading verdict returned to the caller.
type Outcome struct {
	Result        Result
	Tier          Tier
	Complexity    Complexity
	Attempts      []Attempt
	Escalated     bool
	BudgetStopped bool
}

// Attempt records one tier invocation for transparency in session logs.
type Attempt struct {
	Tier       string
	Quality    float64
	Err        string
	Escalated  bool
	BudgetStop bool
}

// Cascader orchestrates the try-cheap-first / escalate-on-miss loop.
type Cascader struct {
	tiers      []Tier
	quality    QualityFn
	budget     BudgetGate
	classifier func(string, Signals) Classification
}

// Option customizes a Cascader.
type Option func(*Cascader)

// WithQualityFn overrides the default text-based heuristic quality scorer.
func WithQualityFn(fn QualityFn) Option { return func(c *Cascader) { c.quality = fn } }

// WithBudgetGate halts escalation when the gate returns false.
func WithBudgetGate(fn BudgetGate) Option { return func(c *Cascader) { c.budget = fn } }

// WithClassifier overrides the default heuristic classifier.
func WithClassifier(fn func(string, Signals) Classification) Option {
	return func(c *Cascader) { c.classifier = fn }
}

// New builds a Cascader from a set of tiers. Tiers are sorted ascending by
// Cost so that the cheapest tier is always tried first.
func New(tiers []Tier, opts ...Option) (*Cascader, error) {
	if len(tiers) == 0 {
		return nil, errors.New("cascade: at least one tier is required")
	}
	for i, t := range tiers {
		if t.Name == "" {
			return nil, fmt.Errorf("cascade: tier %d has empty Name", i)
		}
	}
	sorted := append([]Tier(nil), tiers...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Cost < sorted[j].Cost })

	c := &Cascader{
		tiers:      sorted,
		quality:    defaultQuality,
		classifier: Classify,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Tiers returns the tier list sorted cheapest-first.
func (c *Cascader) Tiers() []Tier { return append([]Tier(nil), c.tiers...) }

// Run cascades through tiers for a single prompt. The starting tier is the
// cheapest one whose Handles set covers the classified complexity (or the
// cheapest tier overall if no tier explicitly handles the complexity). Each
// tier's Result is scored; if the score is below MinConfidence the next more
// expensive tier is tried.
func (c *Cascader) Run(ctx context.Context, prompt string, signals Signals, infer Inferer) (Outcome, error) {
	if infer == nil {
		return Outcome{}, errors.New("cascade: infer is required")
	}
	cls := c.classifier(prompt, signals)
	start := c.startIndex(cls.Complexity)

	var attempts []Attempt
	var lastErr error
	for i := start; i < len(c.tiers); i++ {
		tier := c.tiers[i]

		if c.budget != nil && !c.budget(tier) {
			attempts = append(attempts, Attempt{Tier: tier.Name, BudgetStop: true})
			// Degrade: return the last non-error attempt if we have one,
			// otherwise surface a budget error.
			if last, ok := lastSuccessful(attempts); ok {
				return Outcome{
					Result:        Result{}, // caller-side context dropped, but we surface the last attempt's tier
					Tier:          tierByName(c.tiers, last.Tier),
					Complexity:    cls.Complexity,
					Attempts:      attempts,
					BudgetStopped: true,
				}, nil
			}
			return Outcome{
				Complexity:    cls.Complexity,
				Attempts:      attempts,
				BudgetStopped: true,
			}, fmt.Errorf("cascade: budget gate halted escalation before %s", tier.Name)
		}

		res, err := infer(ctx, tier)
		if err != nil {
			attempts = append(attempts, Attempt{Tier: tier.Name, Err: err.Error()})
			lastErr = err
			// Hard errors escalate to the next tier immediately.
			continue
		}

		quality := c.quality(tier, res)
		escalate := quality < tier.MinConfidence
		// If this tier handles the classified complexity exactly, don't
		// escalate just because of a low-confidence threshold — the tier was
		// chosen to be sufficient.
		if tierHandles(tier, cls.Complexity) && quality > 0 {
			escalate = false
		}
		if i == len(c.tiers)-1 {
			escalate = false // already at the strongest tier
		}

		attempts = append(attempts, Attempt{
			Tier:      tier.Name,
			Quality:   quality,
			Escalated: escalate,
		})
		if escalate {
			continue
		}
		return Outcome{
			Result:     res,
			Tier:       tier,
			Complexity: cls.Complexity,
			Attempts:   attempts,
			Escalated:  i > start,
		}, nil
	}

	if lastErr != nil {
		return Outcome{Complexity: cls.Complexity, Attempts: attempts}, fmt.Errorf("cascade: all tiers failed: %w", lastErr)
	}
	return Outcome{Complexity: cls.Complexity, Attempts: attempts}, errors.New("cascade: no tier produced a usable result")
}

// Batch runs Run for each prompt concurrently and returns outcomes in input
// order. Concurrency is bounded by maxParallel (defaults to len(prompts) if 0).
func (c *Cascader) Batch(ctx context.Context, prompts []string, signals []Signals, maxParallel int, infer Inferer) []BatchResult {
	out := make([]BatchResult, len(prompts))
	if len(prompts) == 0 {
		return out
	}
	if maxParallel <= 0 || maxParallel > len(prompts) {
		maxParallel = len(prompts)
	}

	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	for i, p := range prompts {
		sig := Signals{}
		if i < len(signals) {
			sig = signals[i]
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, prompt string, sig Signals) {
			defer wg.Done()
			defer func() { <-sem }()
			outcome, err := c.Run(ctx, prompt, sig, infer)
			out[idx] = BatchResult{Outcome: outcome, Err: err}
		}(i, p, sig)
	}
	wg.Wait()
	return out
}

// BatchResult is one element of the parallel Batch output.
type BatchResult struct {
	Outcome Outcome
	Err     error
}

// startIndex picks the first tier index whose Handles covers complexity. If no
// tier explicitly handles it, returns the index of the cheapest tier whose
// Handles contains an equal-or-greater complexity, falling back to 0.
func (c *Cascader) startIndex(complexity Complexity) int {
	for i, tier := range c.tiers {
		if tierHandles(tier, complexity) {
			return i
		}
	}
	// No tier explicitly claims this complexity — find the first whose
	// declared complexities are all "harder" than the request, otherwise 0.
	for i, tier := range c.tiers {
		if len(tier.Handles) == 0 {
			return i
		}
		if anyAtLeast(tier.Handles, complexity) {
			return i
		}
	}
	return 0
}

func tierHandles(tier Tier, complexity Complexity) bool {
	for _, h := range tier.Handles {
		if h == complexity {
			return true
		}
	}
	return false
}

func tierByName(tiers []Tier, name string) Tier {
	for _, t := range tiers {
		if t.Name == name {
			return t
		}
	}
	return Tier{Name: name}
}

func lastSuccessful(attempts []Attempt) (Attempt, bool) {
	for i := len(attempts) - 1; i >= 0; i-- {
		if attempts[i].Err == "" && !attempts[i].BudgetStop {
			return attempts[i], true
		}
	}
	return Attempt{}, false
}

// complexityRank maps complexities onto an ascending integer scale.
func complexityRank(c Complexity) int {
	switch c {
	case ComplexityTrivial:
		return 0
	case ComplexitySimple:
		return 1
	case ComplexityModerate:
		return 2
	case ComplexityComplex:
		return 3
	}
	return 1
}

func anyAtLeast(handles []Complexity, target Complexity) bool {
	want := complexityRank(target)
	for _, h := range handles {
		if complexityRank(h) >= want {
			return true
		}
	}
	return false
}

// defaultQuality is a deterministic content-based heuristic — it has no model
// access. Callers that have a real confidence signal should pass WithQualityFn.
//
// The score is in [0,1]:
//   - explicit Result.Confidence wins when set.
//   - empty/whitespace-only text scores 0.
//   - very short text (<20 chars) scores 0.4.
//   - text containing a refusal stub ("I cannot", "I don't know") scores 0.3.
//   - otherwise 0.85.
func defaultQuality(_ Tier, r Result) float64 {
	if r.Confidence > 0 {
		if r.Confidence > 1 {
			return 1
		}
		return r.Confidence
	}
	text := r.Text
	stripped := ""
	for _, r := range text {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		stripped += string(r)
	}
	if stripped == "" {
		return 0
	}
	if len(text) < 20 {
		return 0.4
	}
	lower := ""
	for _, r := range text {
		if r < 128 {
			if r >= 'A' && r <= 'Z' {
				r = r + 32
			}
			lower += string(r)
		}
	}
	for _, stub := range []string{"i cannot", "i can't", "i don't know", "i'm not sure", "i am not sure"} {
		if contains(lower, stub) {
			return 0.3
		}
	}
	return 0.85
}

func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
