// Package speculative provides Conduit's draft-verifier speculative decoding
// loop and continuous batched-inference scheduler used by parallel sub-agents.
//
// Speculative decoding pairs a fast "draft" model (e.g. Llama 3 8B Q4) with a
// stronger "verifier" model (e.g. Llama 3 70B Q5). The draft proposes a short
// run of tokens; the verifier accepts the longest matching prefix. The loop
// stops as soon as the verifier rejects, then resumes with a new draft.
//
// The package is provider-agnostic: callers supply Drafter and Verifier
// implementations. Acceptance threshold and lookahead window are configurable.
package speculative

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Drafter proposes the next lookahead tokens from prompt+accepted.
type Drafter interface {
	// Draft returns at most lookahead tokens. The slice may be shorter when
	// the draft model emits an end-of-stream signal.
	Draft(ctx context.Context, prompt string, accepted []string, lookahead int) ([]string, error)
}

// Verifier evaluates a proposed prefix and returns the prefix length the
// strong model accepts. A returned value of 0 means "draft was rejected
// entirely, please try again with a fresh proposal". A value of len(proposal)
// accepts the whole batch. A nextToken is returned when the verifier
// substitutes its own next token; callers should always append it.
type Verifier interface {
	Verify(ctx context.Context, prompt string, accepted []string, proposal []string) (acceptedPrefix int, nextToken string, done bool, err error)
}

// Config tunes the loop.
type Config struct {
	// Lookahead is the number of tokens the drafter proposes per round.
	// Defaults to 4.
	Lookahead int
	// MaxTokens caps total accepted tokens. 0 means no cap (rely on done).
	MaxTokens int
	// MinAcceptanceRate halts the loop when the rolling acceptance rate
	// drops below this threshold — at that point the draft model is hurting
	// rather than helping and we should fall back to verifier-only decoding.
	// Window length is fixed at MinSamplesForBackoff. Default is 0.25.
	MinAcceptanceRate float64
	// MinSamplesForBackoff is the rolling window size used when checking
	// MinAcceptanceRate. Default is 8.
	MinSamplesForBackoff int
}

// Outcome is the speculative decode summary.
type Outcome struct {
	Tokens         []string
	DraftAttempts  int
	DraftedTokens  int
	AcceptedTokens int
	BackedOff      bool
	Elapsed        time.Duration
}

// AcceptanceRate is the fraction of drafted tokens accepted.
func (o Outcome) AcceptanceRate() float64 {
	if o.DraftedTokens == 0 {
		return 0
	}
	return float64(o.AcceptedTokens) / float64(o.DraftedTokens)
}

// Decode runs the speculative loop. Behaviour:
//   - If drafter is nil, the loop falls back to single-token verifier calls.
//   - If verifier is nil, an error is returned.
func Decode(ctx context.Context, prompt string, drafter Drafter, verifier Verifier, cfg Config) (Outcome, error) {
	if verifier == nil {
		return Outcome{}, errors.New("speculative: verifier is required")
	}
	lookahead := cfg.Lookahead
	if lookahead <= 0 {
		lookahead = 4
	}
	threshold := cfg.MinAcceptanceRate
	if threshold == 0 {
		threshold = 0.25
	}
	window := cfg.MinSamplesForBackoff
	if window <= 0 {
		window = 8
	}
	out := Outcome{}
	start := time.Now()
	defer func() { out.Elapsed = time.Since(start) }()

	accepted := []string{}
	for {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if cfg.MaxTokens > 0 && len(accepted) >= cfg.MaxTokens {
			out.Tokens = accepted
			return out, nil
		}

		var proposal []string
		if drafter != nil && !out.BackedOff {
			out.DraftAttempts++
			tokens, err := drafter.Draft(ctx, prompt, accepted, lookahead)
			if err != nil {
				return out, fmt.Errorf("speculative: drafter: %w", err)
			}
			proposal = tokens
			out.DraftedTokens += len(proposal)
		}

		acceptedPrefix, nextToken, done, err := verifier.Verify(ctx, prompt, accepted, proposal)
		if err != nil {
			return out, fmt.Errorf("speculative: verifier: %w", err)
		}
		if acceptedPrefix < 0 {
			acceptedPrefix = 0
		}
		if acceptedPrefix > len(proposal) {
			acceptedPrefix = len(proposal)
		}
		out.AcceptedTokens += acceptedPrefix
		accepted = append(accepted, proposal[:acceptedPrefix]...)
		if nextToken != "" {
			accepted = append(accepted, nextToken)
		}
		if done {
			out.Tokens = accepted
			return out, nil
		}

		// Back-off check: if rolling acceptance rate has dropped well below
		// threshold, stop drafting and run verifier-only from here on.
		if !out.BackedOff && out.DraftedTokens >= window {
			if out.AcceptanceRate() < threshold {
				out.BackedOff = true
			}
		}
	}
}

// ----- Continuous batched inference ---------------------------------------

// BatchJob is one request submitted to the continuous batcher.
type BatchJob struct {
	ID     string
	Prompt string
}

// BatchInferer runs a slice of jobs as a single batched call to the underlying
// runtime. Implementations should preserve job order in the returned slice.
type BatchInferer func(ctx context.Context, jobs []BatchJob) ([]BatchOutput, error)

// BatchOutput is one item of the batched response.
type BatchOutput struct {
	ID     string
	Output string
	Err    error
}

// BatcherConfig tunes the continuous batcher.
type BatcherConfig struct {
	// MaxBatch is the largest batch the runtime should receive. Default 8.
	MaxBatch int
	// MaxWait is the longest a job may sit in the queue waiting for siblings
	// before the batch flushes. Default 2ms — small enough to be invisible to
	// interactive use, large enough to coalesce parallel sub-agent calls.
	MaxWait time.Duration
}

// Batcher coalesces concurrent single-job submissions into batched calls. It
// is intended to back parallel sub-agent execution where many small inference
// requests arrive in quick succession.
type Batcher struct {
	cfg     BatcherConfig
	inferer BatchInferer

	mu      sync.Mutex
	pending []pendingJob
	timer   *time.Timer
}

type pendingJob struct {
	job   BatchJob
	reply chan BatchOutput
}

// NewBatcher constructs a Batcher with sane defaults applied to cfg.
func NewBatcher(cfg BatcherConfig, inferer BatchInferer) (*Batcher, error) {
	if inferer == nil {
		return nil, errors.New("speculative: inferer is required")
	}
	if cfg.MaxBatch <= 0 {
		cfg.MaxBatch = 8
	}
	if cfg.MaxWait <= 0 {
		cfg.MaxWait = 2 * time.Millisecond
	}
	return &Batcher{cfg: cfg, inferer: inferer}, nil
}

// Submit enqueues job and blocks until it has been processed in a batch.
// Multiple goroutines may Submit concurrently; the Batcher coalesces them.
func (b *Batcher) Submit(ctx context.Context, job BatchJob) (BatchOutput, error) {
	reply := make(chan BatchOutput, 1)
	b.mu.Lock()
	b.pending = append(b.pending, pendingJob{job: job, reply: reply})
	if len(b.pending) >= b.cfg.MaxBatch {
		batch := b.takeLocked()
		b.mu.Unlock()
		go b.dispatch(ctx, batch)
	} else {
		if b.timer == nil {
			b.timer = time.AfterFunc(b.cfg.MaxWait, b.flushTimer)
		}
		b.mu.Unlock()
	}

	select {
	case out := <-reply:
		return out, out.Err
	case <-ctx.Done():
		return BatchOutput{}, ctx.Err()
	}
}

func (b *Batcher) flushTimer() {
	b.mu.Lock()
	batch := b.takeLocked()
	b.mu.Unlock()
	if len(batch) == 0 {
		return
	}
	b.dispatch(context.Background(), batch)
}

func (b *Batcher) takeLocked() []pendingJob {
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	batch := b.pending
	b.pending = nil
	return batch
}

func (b *Batcher) dispatch(ctx context.Context, batch []pendingJob) {
	jobs := make([]BatchJob, len(batch))
	for i, p := range batch {
		jobs[i] = p.job
	}
	outputs, err := b.inferer(ctx, jobs)
	if err != nil {
		for _, p := range batch {
			p.reply <- BatchOutput{ID: p.job.ID, Err: err}
		}
		return
	}
	// Index outputs by ID so we tolerate inferers that reorder.
	byID := make(map[string]BatchOutput, len(outputs))
	for _, o := range outputs {
		byID[o.ID] = o
	}
	for _, p := range batch {
		o, ok := byID[p.job.ID]
		if !ok {
			p.reply <- BatchOutput{ID: p.job.ID, Err: fmt.Errorf("speculative: inferer omitted job %q", p.job.ID)}
			continue
		}
		p.reply <- o
	}
}
