// Package agentopt collects token-efficient primitives used by the coding
// agent loop: batched tool execution, early termination on structured-output
// completion, plan-then-execute scaffolding, diff-based file updates instead
// of full rewrites, and aggressive conversation compaction for long runs.
//
// Each primitive is intentionally small and dependency-light so it can be
// adopted incrementally by the existing tool pipeline without forcing a
// rewrite of the coding tool surface.
package agentopt

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// ----- Batched tool calls --------------------------------------------------

// BatchRequest is a single sub-call inside a batched tool variant such as
// `read_files([...])`. ID is echoed back on the matching BatchResult so callers
// can match results to inputs without relying on order.
type BatchRequest struct {
	ID    string
	Input any
}

// BatchResult is one outcome of a batched tool call.
type BatchResult struct {
	ID     string
	Output any
	Err    error
}

// BatchExecutor runs many small tool invocations in parallel with a bounded
// worker pool, preserving input order in the returned slice. It is the
// foundation for batch tool variants like read_files, grep_files, stat_files.
type BatchExecutor struct {
	maxParallel int
}

// NewBatchExecutor returns an executor that runs at most maxParallel calls
// concurrently. maxParallel <= 0 falls back to a sane default of 8.
func NewBatchExecutor(maxParallel int) *BatchExecutor {
	if maxParallel <= 0 {
		maxParallel = 8
	}
	return &BatchExecutor{maxParallel: maxParallel}
}

// Run invokes fn for each request, bounded by maxParallel. Results are
// returned in the same order as requests, with each error attached to its
// BatchResult (a per-item error never aborts the batch).
func (b *BatchExecutor) Run(ctx context.Context, requests []BatchRequest, fn func(context.Context, BatchRequest) (any, error)) []BatchResult {
	out := make([]BatchResult, len(requests))
	if len(requests) == 0 {
		return out
	}
	limit := b.maxParallel
	if limit > len(requests) {
		limit = len(requests)
	}

	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, req := range requests {
		select {
		case <-ctx.Done():
			out[i] = BatchResult{ID: req.ID, Err: ctx.Err()}
			continue
		default:
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, req BatchRequest) {
			defer wg.Done()
			defer func() { <-sem }()
			output, err := fn(ctx, req)
			out[idx] = BatchResult{ID: req.ID, Output: output, Err: err}
		}(i, req)
	}
	wg.Wait()
	return out
}

// ----- Early termination ---------------------------------------------------

// EarlyTermination wraps a streaming token loop so it stops as soon as the
// caller-supplied Done predicate accepts the partial buffer. This is the
// "stop when structured output is complete" optimization from PRD §16.6.
type EarlyTermination struct {
	Done func(buf string) bool
}

// Consume appends token to the running buffer and returns (newBuffer, stop).
// stop becomes true the first call that pushes the buffer into a "done" state.
func (e EarlyTermination) Consume(buf, token string) (string, bool) {
	buf += token
	if e.Done == nil {
		return buf, false
	}
	return buf, e.Done(buf)
}

// JSONObjectComplete returns true once buf contains a balanced top-level JSON
// object (matched braces outside of strings, accounting for escapes). It is a
// cheap structural check, not a full parser, but is sufficient for the common
// "stop streaming once the JSON tool input closes" case.
func JSONObjectComplete(buf string) bool {
	depth := 0
	inString := false
	escaped := false
	started := false
	for i := 0; i < len(buf); i++ {
		c := buf[i]
		if escaped {
			escaped = false
			continue
		}
		if inString {
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
			started = true
		case '}':
			depth--
			if started && depth == 0 {
				return true
			}
		}
	}
	return false
}

// ----- Plan-then-execute ---------------------------------------------------

// PlanStep is a single unit of work in a plan-then-execute loop.
type PlanStep struct {
	ID          string
	Description string
	DependsOn   []string
}

// Plan is an ordered list of steps with dependencies. It validates that all
// referenced dependencies exist and that there are no cycles.
type Plan struct {
	Steps []PlanStep
}

// Validate ensures every DependsOn reference resolves to a known step and that
// no cycle exists. It returns nil for an empty plan.
func (p Plan) Validate() error {
	known := make(map[string]bool, len(p.Steps))
	for _, s := range p.Steps {
		if s.ID == "" {
			return errors.New("plan: step ID is required")
		}
		if known[s.ID] {
			return fmt.Errorf("plan: duplicate step ID %q", s.ID)
		}
		known[s.ID] = true
	}
	for _, s := range p.Steps {
		for _, dep := range s.DependsOn {
			if !known[dep] {
				return fmt.Errorf("plan: step %q depends on unknown step %q", s.ID, dep)
			}
		}
	}
	// cycle detection (Kahn's algorithm)
	indeg := make(map[string]int, len(p.Steps))
	adj := make(map[string][]string, len(p.Steps))
	for _, s := range p.Steps {
		indeg[s.ID] += 0
		for _, d := range s.DependsOn {
			indeg[s.ID]++
			adj[d] = append(adj[d], s.ID)
		}
	}
	queue := []string{}
	for id, n := range indeg {
		if n == 0 {
			queue = append(queue, id)
		}
	}
	visited := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		visited++
		for _, next := range adj[id] {
			indeg[next]--
			if indeg[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if visited != len(p.Steps) {
		return errors.New("plan: dependency cycle detected")
	}
	return nil
}

// TopologicalOrder returns step IDs in a dependency-respecting order, with
// stable ordering across runs (sibling steps preserve their original index).
func (p Plan) TopologicalOrder() ([]string, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	indeg := make(map[string]int, len(p.Steps))
	adj := make(map[string][]string, len(p.Steps))
	originalIndex := make(map[string]int, len(p.Steps))
	for i, s := range p.Steps {
		originalIndex[s.ID] = i
		indeg[s.ID] += 0
		for _, d := range s.DependsOn {
			indeg[s.ID]++
			adj[d] = append(adj[d], s.ID)
		}
	}

	ready := []string{}
	for id, n := range indeg {
		if n == 0 {
			ready = append(ready, id)
		}
	}
	sort.SliceStable(ready, func(i, j int) bool { return originalIndex[ready[i]] < originalIndex[ready[j]] })

	out := make([]string, 0, len(p.Steps))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		out = append(out, id)
		for _, next := range adj[id] {
			indeg[next]--
			if indeg[next] == 0 {
				ready = append(ready, next)
			}
		}
		sort.SliceStable(ready, func(i, j int) bool { return originalIndex[ready[i]] < originalIndex[ready[j]] })
	}
	return out, nil
}

// ----- Diff-based file updates ---------------------------------------------

// Edit is a single contiguous replacement inside a file, expressed as
// "replace this exact string with that one". The agent emits Edits instead of
// full file rewrites so prompts stay small even for large files.
type Edit struct {
	OldString string
	NewString string
	// Occurrence is 1-based. 0 means "first occurrence" (the common case);
	// a negative value is invalid. Use ReplaceAll=true to skip the index.
	Occurrence int
	ReplaceAll bool
}

// ApplyEdits applies edits to original in order, returning the new content.
// It returns an error if any OldString is missing, or if it appears more than
// once and ReplaceAll is false and Occurrence is the default (0/1) — the same
// safety contract used by the existing edit tool.
func ApplyEdits(original string, edits []Edit) (string, error) {
	current := original
	for i, e := range edits {
		if e.OldString == "" {
			return "", fmt.Errorf("edit %d: OldString is required", i)
		}
		if e.OldString == e.NewString {
			return "", fmt.Errorf("edit %d: OldString and NewString are identical", i)
		}
		if e.ReplaceAll {
			if !strings.Contains(current, e.OldString) {
				return "", fmt.Errorf("edit %d: OldString not found", i)
			}
			current = strings.ReplaceAll(current, e.OldString, e.NewString)
			continue
		}
		count := strings.Count(current, e.OldString)
		if count == 0 {
			return "", fmt.Errorf("edit %d: OldString not found", i)
		}
		occurrence := e.Occurrence
		if occurrence == 0 {
			occurrence = 1
			if count > 1 {
				return "", fmt.Errorf("edit %d: OldString matches %d times; set ReplaceAll or Occurrence to disambiguate", i, count)
			}
		}
		if occurrence < 1 || occurrence > count {
			return "", fmt.Errorf("edit %d: occurrence %d out of range (have %d)", i, occurrence, count)
		}
		current = replaceNth(current, e.OldString, e.NewString, occurrence)
	}
	return current, nil
}

func replaceNth(haystack, needle, replacement string, n int) string {
	out := make([]byte, 0, len(haystack))
	count := 0
	i := 0
	for i < len(haystack) {
		if i+len(needle) <= len(haystack) && haystack[i:i+len(needle)] == needle {
			count++
			if count == n {
				out = append(out, replacement...)
				out = append(out, haystack[i+len(needle):]...)
				return string(out)
			}
			out = append(out, needle...)
			i += len(needle)
			continue
		}
		out = append(out, haystack[i])
		i++
	}
	return string(out)
}

// EditTokenSavings estimates the prompt-token savings of emitting edits rather
// than the full new file. It is a rough char/4 token estimate consistent with
// the contextassembler heuristic. Negative values mean edits would be larger
// than the rewrite — callers can fall back to a full write in that case.
func EditTokenSavings(original, rewritten string, edits []Edit) int {
	rewriteTokens := estimateTokens(rewritten)
	editTokens := 0
	for _, e := range edits {
		editTokens += estimateTokens(e.OldString) + estimateTokens(e.NewString)
	}
	_ = original
	return rewriteTokens - editTokens
}

func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	n := len(s) / 4
	if n == 0 {
		return 1
	}
	return n
}

// ----- Conversation compaction --------------------------------------------

// Message is one turn in an agent conversation. Tokens is an estimate;
// 0 falls back to a char/4 estimate of Content.
type Message struct {
	Role    string // "user" | "assistant" | "tool" | "system"
	Content string
	Tokens  int
	Pinned  bool // protected from compaction (system + recent fences)
}

// CompactionPolicy declares when and how to compact a transcript.
type CompactionPolicy struct {
	// MaxTokens is the soft ceiling; once total tokens exceed this, Compact
	// drops the oldest unpinned messages until the transcript fits.
	MaxTokens int
	// KeepRecent always retains this many most-recent messages, regardless of
	// the budget. Defaults to 6 when unset.
	KeepRecent int
	// SummaryFn is an optional rollup applied to the dropped messages. The
	// returned Message is inserted at the boundary so the agent has a hint of
	// what was removed. If nil, dropped messages disappear without a marker.
	SummaryFn func(dropped []Message) Message
}

// Compact applies the policy to messages and returns the compacted transcript
// plus the count of messages dropped (excluding any inserted summary).
func Compact(messages []Message, policy CompactionPolicy) ([]Message, int) {
	keepRecent := policy.KeepRecent
	if keepRecent <= 0 {
		keepRecent = 6
	}
	tokens := 0
	for i := range messages {
		if messages[i].Tokens == 0 {
			messages[i].Tokens = estimateTokens(messages[i].Content)
		}
		tokens += messages[i].Tokens
	}
	if policy.MaxTokens <= 0 || tokens <= policy.MaxTokens {
		return messages, 0
	}

	// Walk from oldest to newest, dropping unpinned messages outside the
	// keepRecent tail until we fit.
	cutoff := len(messages) - keepRecent
	if cutoff < 0 {
		cutoff = 0
	}

	dropped := []Message{}
	kept := make([]Message, 0, len(messages))
	for i, m := range messages {
		if i < cutoff && !m.Pinned && tokens > policy.MaxTokens {
			dropped = append(dropped, m)
			tokens -= m.Tokens
			continue
		}
		kept = append(kept, m)
	}

	if len(dropped) == 0 {
		return messages, 0
	}
	if policy.SummaryFn != nil {
		summary := policy.SummaryFn(dropped)
		if summary.Tokens == 0 {
			summary.Tokens = estimateTokens(summary.Content)
		}
		// Insert at the head so the assistant sees "previously: ..." first.
		kept = append([]Message{summary}, kept...)
	}
	return kept, len(dropped)
}
