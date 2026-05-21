// Package efficiency monitors prompt and inference efficiency and feeds an
// auto-tuner that nudges context-budget, relevance-threshold, cache-TTL, and
// routing-threshold settings toward better steady-state performance.
//
// The package is intentionally pure-Go and dependency-free so it can be wired
// into any layer of the engine (router, context assembler, cache, cascade).
// Callers Record per-task events; the monitor maintains rolling aggregates
// and the AutoTuner produces an Adjustments value the caller applies to the
// settings it owns. Adjustments are always within configurable bounds and
// every change is reported with a human-readable Reason for transparency.
package efficiency

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// TaskEvent records the resource cost and outcome of one logical task. Tasks
// usually correspond to one user-visible turn (which may span many provider
// calls). Counters that are unknown can be left at zero.
type TaskEvent struct {
	TaskID         string
	Feature        string
	Plugin         string
	Model          string
	InputTokens    int
	OutputTokens   int
	CostUSD        float64
	Latency        time.Duration
	Success        bool
	Retries        int
	Escalated      bool    // cascade escalated to a more expensive tier
	CacheHits      int     // any cache hierarchy hit (prefix/KV/exact/semantic/tool)
	CacheLookups   int     // any cache lookup attempt
	ContextTokens  int     // tokens after assembler optimization
	ContextDropped int     // tokens dropped by assembler
	BudgetUSD      float64 // per-task budget when enforced (0 = none)
	RecordedAt     time.Time
}

// Metrics is the snapshot returned by Monitor.Snapshot.
type Metrics struct {
	Tasks                int
	SuccessfulTasks      int
	TokensPerSuccessTask float64
	CostPerSuccessTask   float64
	CacheHitRate         float64
	ContextUtilization   float64 // mean ContextTokens / (ContextTokens+ContextDropped)
	WasteRatio           float64 // mean dropped / total context tokens (0..1)
	RoutingEfficiency    float64 // 1 - escalation_rate (no escalation = perfect routing)
	RetryRate            float64
	EscalationRate       float64
	BudgetExceedRate     float64
	WindowStart          time.Time
	WindowEnd            time.Time
}

// Monitor maintains rolling efficiency metrics over a configurable window.
type Monitor struct {
	mu     sync.Mutex
	events []TaskEvent
	cfg    Config
}

// Config controls how the monitor retains data.
type Config struct {
	// Window is the maximum age of an event retained for snapshots and
	// auto-tuning. Default 7 days.
	Window time.Duration
	// MaxEvents caps the in-memory buffer regardless of Window. Default 10k.
	MaxEvents int
	// Now overrides time.Now (testing only).
	Now func() time.Time
}

// New creates a Monitor with sane defaults applied to cfg.
func New(cfg Config) *Monitor {
	if cfg.Window <= 0 {
		cfg.Window = 7 * 24 * time.Hour
	}
	if cfg.MaxEvents <= 0 {
		cfg.MaxEvents = 10_000
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Monitor{cfg: cfg}
}

// Record stores ev. RecordedAt is filled in if zero.
func (m *Monitor) Record(ev TaskEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ev.RecordedAt.IsZero() {
		ev.RecordedAt = m.cfg.Now()
	}
	m.events = append(m.events, ev)
	m.evictLocked()
}

func (m *Monitor) evictLocked() {
	cutoff := m.cfg.Now().Add(-m.cfg.Window)
	keep := m.events[:0]
	for _, e := range m.events {
		if e.RecordedAt.After(cutoff) {
			keep = append(keep, e)
		}
	}
	m.events = keep
	if len(m.events) > m.cfg.MaxEvents {
		drop := len(m.events) - m.cfg.MaxEvents
		m.events = append([]TaskEvent(nil), m.events[drop:]...)
	}
}

// Snapshot returns a metric snapshot for the active window.
func (m *Monitor) Snapshot() Metrics {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.evictLocked()
	return computeMetrics(m.events)
}

// Events returns a copy of the retained events for reporting/export.
func (m *Monitor) Events() []TaskEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]TaskEvent, len(m.events))
	copy(out, m.events)
	return out
}

func computeMetrics(events []TaskEvent) Metrics {
	if len(events) == 0 {
		return Metrics{}
	}
	var (
		successful           int
		tokens               float64
		cost                 float64
		hits, lookups        int
		ctxKept, ctxDropped  int
		retries, escalations int
		budgetExceeded       int
		hasContextSamples    int
	)
	start := events[0].RecordedAt
	end := events[0].RecordedAt
	for _, e := range events {
		if e.RecordedAt.Before(start) {
			start = e.RecordedAt
		}
		if e.RecordedAt.After(end) {
			end = e.RecordedAt
		}
		if e.Success {
			successful++
			tokens += float64(e.InputTokens + e.OutputTokens)
			cost += e.CostUSD
		}
		hits += e.CacheHits
		lookups += e.CacheLookups
		if e.ContextTokens+e.ContextDropped > 0 {
			ctxKept += e.ContextTokens
			ctxDropped += e.ContextDropped
			hasContextSamples++
		}
		retries += e.Retries
		if e.Escalated {
			escalations++
		}
		if e.BudgetUSD > 0 && e.CostUSD > e.BudgetUSD {
			budgetExceeded++
		}
	}
	m := Metrics{
		Tasks:           len(events),
		SuccessfulTasks: successful,
		WindowStart:     start,
		WindowEnd:       end,
	}
	if successful > 0 {
		m.TokensPerSuccessTask = tokens / float64(successful)
		m.CostPerSuccessTask = cost / float64(successful)
	}
	if lookups > 0 {
		m.CacheHitRate = float64(hits) / float64(lookups)
	}
	totalCtx := ctxKept + ctxDropped
	if totalCtx > 0 {
		m.ContextUtilization = float64(ctxKept) / float64(totalCtx)
		m.WasteRatio = float64(ctxDropped) / float64(totalCtx)
	}
	tasks := float64(len(events))
	m.RetryRate = float64(retries) / tasks
	m.EscalationRate = float64(escalations) / tasks
	m.RoutingEfficiency = 1 - m.EscalationRate
	if m.RoutingEfficiency < 0 {
		m.RoutingEfficiency = 0
	}
	m.BudgetExceedRate = float64(budgetExceeded) / tasks
	return m
}

// ----- Auto-tuner ----------------------------------------------------------

// Settings is the live settings snapshot the AutoTuner reads from. Tunable
// fields are wide-open floats; concrete callers map them to their domain
// (context.MaxTokens, semantic.Threshold, cache.TTL seconds, cascade.Threshold).
type Settings struct {
	ContextBudgetTokens  int
	RelevanceThreshold   float64 // 0..1
	CacheTTLSeconds      int
	CascadeMinConfidence float64 // 0..1
}

// Bounds clamps adjustments so the tuner can never go off the rails.
type Bounds struct {
	MinContextBudget int
	MaxContextBudget int
	MinRelevance     float64
	MaxRelevance     float64
	MinCacheTTL      int
	MaxCacheTTL      int
	MinCascadeConf   float64
	MaxCascadeConf   float64
	// MaxStepFraction caps a single tuning step at this fraction of the
	// current value (e.g. 0.2 = at most ±20% per cycle). Default 0.2.
	MaxStepFraction float64
}

// DefaultBounds is a conservative bound suitable for production wiring.
var DefaultBounds = Bounds{
	MinContextBudget: 2_000,
	MaxContextBudget: 200_000,
	MinRelevance:     0.05,
	MaxRelevance:     0.9,
	MinCacheTTL:      60,
	MaxCacheTTL:      7 * 24 * 3600,
	MinCascadeConf:   0.1,
	MaxCascadeConf:   0.95,
	MaxStepFraction:  0.2,
}

// Adjustments is the diff applied to Settings on a tune cycle. Each Reason
// is a human-readable explanation suitable for the session log so users can
// audit what changed and why.
type Adjustments struct {
	NewSettings Settings
	Reasons     []string
	Changed     bool
}

// AutoTuner converts a Metrics snapshot into Adjustments under Bounds.
// All decisions are deterministic given the inputs.
type AutoTuner struct {
	bounds Bounds
}

// NewAutoTuner returns a tuner using bounds. Zero-valued bounds default to
// DefaultBounds; partially set bounds inherit from DefaultBounds.
func NewAutoTuner(bounds Bounds) *AutoTuner {
	return &AutoTuner{bounds: mergeBounds(bounds)}
}

func mergeBounds(b Bounds) Bounds {
	d := DefaultBounds
	if b.MinContextBudget > 0 {
		d.MinContextBudget = b.MinContextBudget
	}
	if b.MaxContextBudget > 0 {
		d.MaxContextBudget = b.MaxContextBudget
	}
	if b.MinRelevance > 0 {
		d.MinRelevance = b.MinRelevance
	}
	if b.MaxRelevance > 0 {
		d.MaxRelevance = b.MaxRelevance
	}
	if b.MinCacheTTL > 0 {
		d.MinCacheTTL = b.MinCacheTTL
	}
	if b.MaxCacheTTL > 0 {
		d.MaxCacheTTL = b.MaxCacheTTL
	}
	if b.MinCascadeConf > 0 {
		d.MinCascadeConf = b.MinCascadeConf
	}
	if b.MaxCascadeConf > 0 {
		d.MaxCascadeConf = b.MaxCascadeConf
	}
	if b.MaxStepFraction > 0 {
		d.MaxStepFraction = b.MaxStepFraction
	}
	return d
}

// Tune applies heuristics to current and metrics, producing Adjustments. If no
// change is warranted, Changed is false and NewSettings == current.
//
// Heuristics (all overridable by the caller — the user is told what changed):
//
//   - WasteRatio > 0.4 → shrink ContextBudget and raise RelevanceThreshold.
//     The assembler is paying for context that gets dropped.
//   - WasteRatio < 0.1 and SuccessfulTasks ≥ 20 → grow ContextBudget slightly.
//     We have headroom and the agent is succeeding.
//   - CacheHitRate < 0.2 with ≥ 50 lookups → grow CacheTTL. Hits are too
//     transient.
//   - CacheHitRate > 0.7 → shrink CacheTTL slightly (avoid serving stale).
//   - EscalationRate > 0.5 → lower CascadeMinConfidence (cheap tier rarely
//     accepted; tighten threshold so it has to do less work).
//   - EscalationRate < 0.05 with ≥ 20 tasks → raise CascadeMinConfidence
//     (cheap tier almost always accepted; let it accept harder tasks).
func (t *AutoTuner) Tune(current Settings, m Metrics) Adjustments {
	out := Adjustments{NewSettings: current}
	step := t.bounds.MaxStepFraction

	// Context budget + relevance threshold ----------------------------------
	if m.WasteRatio > 0.4 {
		newBudget := scaleInt(current.ContextBudgetTokens, 1-step)
		newBudget = clampInt(newBudget, t.bounds.MinContextBudget, t.bounds.MaxContextBudget)
		if newBudget != current.ContextBudgetTokens {
			out.NewSettings.ContextBudgetTokens = newBudget
			out.Reasons = append(out.Reasons,
				fmt.Sprintf("waste ratio %.0f%% > 40%%: shrink context budget %d → %d",
					m.WasteRatio*100, current.ContextBudgetTokens, newBudget))
		}
		newRelevance := clampFloat(current.RelevanceThreshold+step*0.25, t.bounds.MinRelevance, t.bounds.MaxRelevance)
		if newRelevance != current.RelevanceThreshold {
			out.NewSettings.RelevanceThreshold = newRelevance
			out.Reasons = append(out.Reasons,
				fmt.Sprintf("waste ratio %.0f%% > 40%%: raise relevance threshold %.2f → %.2f",
					m.WasteRatio*100, current.RelevanceThreshold, newRelevance))
		}
	} else if m.WasteRatio > 0 && m.WasteRatio < 0.1 && m.SuccessfulTasks >= 20 {
		newBudget := scaleInt(current.ContextBudgetTokens, 1+step*0.5)
		newBudget = clampInt(newBudget, t.bounds.MinContextBudget, t.bounds.MaxContextBudget)
		if newBudget != current.ContextBudgetTokens {
			out.NewSettings.ContextBudgetTokens = newBudget
			out.Reasons = append(out.Reasons,
				fmt.Sprintf("waste ratio %.0f%% < 10%% with %d successes: grow context budget %d → %d",
					m.WasteRatio*100, m.SuccessfulTasks, current.ContextBudgetTokens, newBudget))
		}
	}

	// Cache TTL --------------------------------------------------------------
	if m.CacheHitRate > 0 && m.CacheHitRate < 0.2 {
		newTTL := scaleInt(current.CacheTTLSeconds, 1+step)
		newTTL = clampInt(newTTL, t.bounds.MinCacheTTL, t.bounds.MaxCacheTTL)
		if newTTL != current.CacheTTLSeconds {
			out.NewSettings.CacheTTLSeconds = newTTL
			out.Reasons = append(out.Reasons,
				fmt.Sprintf("cache hit rate %.0f%% < 20%%: grow cache TTL %ds → %ds",
					m.CacheHitRate*100, current.CacheTTLSeconds, newTTL))
		}
	} else if m.CacheHitRate > 0.7 {
		newTTL := scaleInt(current.CacheTTLSeconds, 1-step*0.5)
		newTTL = clampInt(newTTL, t.bounds.MinCacheTTL, t.bounds.MaxCacheTTL)
		if newTTL != current.CacheTTLSeconds {
			out.NewSettings.CacheTTLSeconds = newTTL
			out.Reasons = append(out.Reasons,
				fmt.Sprintf("cache hit rate %.0f%% > 70%%: shrink cache TTL %ds → %ds (avoid stale)",
					m.CacheHitRate*100, current.CacheTTLSeconds, newTTL))
		}
	}

	// Cascade confidence -----------------------------------------------------
	if m.EscalationRate > 0.5 {
		newConf := clampFloat(current.CascadeMinConfidence-step*0.25, t.bounds.MinCascadeConf, t.bounds.MaxCascadeConf)
		if newConf != current.CascadeMinConfidence {
			out.NewSettings.CascadeMinConfidence = newConf
			out.Reasons = append(out.Reasons,
				fmt.Sprintf("escalation rate %.0f%% > 50%%: lower cascade threshold %.2f → %.2f",
					m.EscalationRate*100, current.CascadeMinConfidence, newConf))
		}
	} else if m.EscalationRate < 0.05 && m.Tasks >= 20 {
		newConf := clampFloat(current.CascadeMinConfidence+step*0.25, t.bounds.MinCascadeConf, t.bounds.MaxCascadeConf)
		if newConf != current.CascadeMinConfidence {
			out.NewSettings.CascadeMinConfidence = newConf
			out.Reasons = append(out.Reasons,
				fmt.Sprintf("escalation rate %.0f%% < 5%% with %d tasks: raise cascade threshold %.2f → %.2f",
					m.EscalationRate*100, m.Tasks, current.CascadeMinConfidence, newConf))
		}
	}

	out.Changed = out.NewSettings != current
	return out
}

func scaleInt(n int, factor float64) int {
	if n == 0 {
		return 0
	}
	scaled := float64(n) * factor
	if scaled < float64(n) && scaled > float64(n)-1 {
		return n - 1
	}
	if scaled > float64(n) && scaled < float64(n)+1 {
		return n + 1
	}
	return int(scaled)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ----- Weekly report ------------------------------------------------------

// WeeklyReport groups events into 7-day buckets and reports the trend in
// tokens-per-success-task and cache-hit-rate. It is the surface a `conduit
// usage efficiency` CLI would render.
type WeeklyReport struct {
	GeneratedAt time.Time
	Weeks       []WeekBucket
	Latest      Metrics
}

// WeekBucket is a single 7-day window in the report.
type WeekBucket struct {
	Start   time.Time
	End     time.Time
	Metrics Metrics
}

// BuildWeeklyReport buckets the monitor's retained events into rolling 7-day
// windows ending at now. Buckets are returned oldest-first. Empty windows are
// included so the report length is stable.
func BuildWeeklyReport(m *Monitor, now time.Time, weeks int) (WeeklyReport, error) {
	if m == nil {
		return WeeklyReport{}, errors.New("efficiency: monitor is nil")
	}
	if weeks <= 0 {
		weeks = 4
	}
	events := m.Events()
	sort.SliceStable(events, func(i, j int) bool { return events[i].RecordedAt.Before(events[j].RecordedAt) })

	rep := WeeklyReport{GeneratedAt: now, Latest: m.Snapshot()}
	for w := weeks - 1; w >= 0; w-- {
		end := now.AddDate(0, 0, -7*w)
		start := end.AddDate(0, 0, -7)
		bucket := WeekBucket{Start: start, End: end}
		var inWindow []TaskEvent
		for _, e := range events {
			if !e.RecordedAt.Before(start) && e.RecordedAt.Before(end) {
				inWindow = append(inWindow, e)
			}
		}
		bucket.Metrics = computeMetrics(inWindow)
		rep.Weeks = append(rep.Weeks, bucket)
	}
	return rep, nil
}
