package coding

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// DelegateTask describes one nested-agent invocation in a delegation
// batch. Tasks form a DAG: each task may DependsOn the IDs of others in
// the same batch and the manager runs them in topological waves —
// independent tasks in a wave run in parallel; the next wave waits for
// all current-wave tasks to finish.
type DelegateTask struct {
	ID        string   // unique within the batch
	Agent     string   // name of the agent profile to run (informational)
	Prompt    string   // the prompt the child agent receives
	DependsOn []string // task IDs in the same batch that must finish first
}

// DelegateResult is the outcome of one nested task. The Lineage field
// records the chain of (parent_session_id, parent_task_id) so audit
// surfaces can reconstruct who delegated what.
type DelegateResult struct {
	TaskID     string
	Agent      string
	SessionID  string
	Output     string
	Error      string
	StartedAt  time.Time
	FinishedAt time.Time
	Lineage    []DelegateLineageStep
}

// DelegateLineageStep is one ancestor entry in a child session's lineage.
type DelegateLineageStep struct {
	ParentSessionID string
	ParentTaskID    string
}

// DelegateRunner is the contract the AgentManager uses to actually run
// a child task. Production wiring injects a Streamer-backed runner;
// tests inject a fake. Keeping this small means the manager itself is
// purely orchestration logic.
type DelegateRunner interface {
	Run(ctx context.Context, task DelegateTask, lineage []DelegateLineageStep) (DelegateResult, error)
}

// AgentManager runs delegated tasks with dependency-aware topological
// batching, lineage tracking, and per-batch summaries.
type AgentManager struct {
	Runner DelegateRunner

	mu       sync.Mutex
	children map[string][]DelegateLineageStep // session_id -> lineage
}

// NewAgentManager wires a manager around a runner.
func NewAgentManager(runner DelegateRunner) *AgentManager {
	return &AgentManager{Runner: runner, children: map[string][]DelegateLineageStep{}}
}

// DelegateBatch runs tasks in dependency order. Independent tasks run
// in parallel; a task fails the batch only if its dependents cannot
// proceed — failed tasks still appear in the returned result list with
// their Error set, so callers can surface partial progress.
func (m *AgentManager) DelegateBatch(ctx context.Context, parentSessionID string, tasks []DelegateTask) ([]DelegateResult, error) {
	if m.Runner == nil {
		return nil, errors.New("delegate: runner required")
	}
	waves, err := topoWaves(tasks)
	if err != nil {
		return nil, err
	}

	resultsByID := make(map[string]DelegateResult, len(tasks))
	skipped := map[string]bool{}

	for _, wave := range waves {
		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, task := range wave {
			task := task

			// Skip if any dependency failed or was skipped.
			skip := false
			for _, dep := range task.DependsOn {
				if r, ok := resultsByID[dep]; ok && r.Error != "" {
					skip = true
					break
				}
				if skipped[dep] {
					skip = true
					break
				}
			}
			if skip {
				skipped[task.ID] = true
				resultsByID[task.ID] = DelegateResult{
					TaskID: task.ID, Agent: task.Agent,
					Error:      "skipped: upstream dependency failed",
					StartedAt:  time.Now().UTC(),
					FinishedAt: time.Now().UTC(),
				}
				continue
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				lineage := []DelegateLineageStep{{ParentSessionID: parentSessionID, ParentTaskID: task.ID}}
				started := time.Now().UTC()
				res, err := m.Runner.Run(ctx, task, lineage)
				if err != nil {
					res.Error = err.Error()
				}
				if res.TaskID == "" {
					res.TaskID = task.ID
				}
				if res.Agent == "" {
					res.Agent = task.Agent
				}
				if res.StartedAt.IsZero() {
					res.StartedAt = started
				}
				if res.FinishedAt.IsZero() {
					res.FinishedAt = time.Now().UTC()
				}
				if len(res.Lineage) == 0 {
					res.Lineage = lineage
				}
				if res.SessionID != "" {
					m.mu.Lock()
					m.children[res.SessionID] = res.Lineage
					m.mu.Unlock()
				}
				mu.Lock()
				resultsByID[task.ID] = res
				mu.Unlock()
			}()
		}
		wg.Wait()
	}

	out := make([]DelegateResult, 0, len(tasks))
	for _, t := range tasks {
		if r, ok := resultsByID[t.ID]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

// SummarizeBatch returns a stable, human-readable summary of one batch
// run: counts by status, then per-task one-liners. Used by the parent
// agent to fold the nested results back into its own context.
func SummarizeBatch(results []DelegateResult) string {
	if len(results) == 0 {
		return "(no delegated tasks)"
	}
	var ok, failed, skipped int
	for _, r := range results {
		switch {
		case r.Error == "":
			ok++
		case strings.HasPrefix(r.Error, "skipped"):
			skipped++
		default:
			failed++
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "delegated %d task(s): ok=%d failed=%d skipped=%d\n", len(results), ok, failed, skipped)
	for _, r := range results {
		status := "ok"
		switch {
		case r.Error == "":
			// keep
		case strings.HasPrefix(r.Error, "skipped"):
			status = "skip"
		default:
			status = "fail"
		}
		first := r.Output
		if i := strings.IndexByte(first, '\n'); i >= 0 {
			first = first[:i]
		}
		if len(first) > 120 {
			first = first[:120]
		}
		fmt.Fprintf(&b, "  [%s] %s (%s): %s\n", status, r.TaskID, r.Agent, first)
		if r.Error != "" {
			fmt.Fprintf(&b, "        error: %s\n", r.Error)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// LineageOf returns the recorded lineage chain for a child session, or
// nil if the manager has never seen that id.
func (m *AgentManager) LineageOf(sessionID string) []DelegateLineageStep {
	m.mu.Lock()
	defer m.mu.Unlock()
	if got, ok := m.children[sessionID]; ok {
		out := make([]DelegateLineageStep, len(got))
		copy(out, got)
		return out
	}
	return nil
}

// topoWaves groups tasks into topologically ordered waves so the
// manager can run independent tasks in parallel. Returns an error if
// the task list contains a cycle or a missing dependency reference.
func topoWaves(tasks []DelegateTask) ([][]DelegateTask, error) {
	byID := map[string]DelegateTask{}
	indeg := map[string]int{}
	dependents := map[string][]string{}
	for _, t := range tasks {
		if _, dup := byID[t.ID]; dup {
			return nil, fmt.Errorf("delegate: duplicate task id %q", t.ID)
		}
		byID[t.ID] = t
		indeg[t.ID] = 0
	}
	for _, t := range tasks {
		for _, dep := range t.DependsOn {
			if _, ok := byID[dep]; !ok {
				return nil, fmt.Errorf("delegate: task %q depends on unknown id %q", t.ID, dep)
			}
			indeg[t.ID]++
			dependents[dep] = append(dependents[dep], t.ID)
		}
	}

	var waves [][]DelegateTask
	remaining := len(tasks)
	for remaining > 0 {
		var wave []DelegateTask
		var ids []string
		for id, d := range indeg {
			if d == 0 {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			return nil, errors.New("delegate: cycle detected in task DAG")
		}
		// Stable order within a wave for deterministic execution.
		sort.Strings(ids)
		for _, id := range ids {
			wave = append(wave, byID[id])
			delete(indeg, id)
			for _, child := range dependents[id] {
				indeg[child]--
			}
		}
		waves = append(waves, wave)
		remaining -= len(wave)
	}
	return waves, nil
}
