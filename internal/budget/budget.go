// Package budget evaluates spend against configured monthly limits and produces
// warning/critical/blocked decisions before each model call.
package budget

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jabreeflor/conduit/internal/config"
	"github.com/jabreeflor/conduit/internal/contracts"
)

// SpendLevel classifies current spend relative to a configured limit.
type SpendLevel int

const (
	SpendLevelOK       SpendLevel = iota // below warning threshold
	SpendLevelWarning                    // ≥ warning_pct of limit
	SpendLevelCritical                   // ≥ 90% of limit
	SpendLevelBlocked                    // ≥ 100% of limit with hard_stop: true
)

// Decision is the result of a pre-call budget check.
type Decision struct {
	Allowed  bool
	Level    SpendLevel
	Key      string  // model name or "overall"
	SpentUSD float64 // current month spend (including estimated call)
	LimitUSD float64
	PctUsed  float64
	HardStop bool
}

// ModelReport is the per-model or overall budget status shown in dashboards.
type ModelReport struct {
	SpentUSD               float64
	LimitUSD               float64
	PctUsed                float64
	Level                  SpendLevel
	HardStop               bool
	ProjectedOvershootDate *time.Time // nil when no overshoot expected this month
}

// Report is the full budget status across all configured models and overall.
type Report struct {
	Overall ModelReport
	ByModel map[string]ModelReport
	AsOf    time.Time
}

// MonthlySpend holds aggregated spend for a single calendar month.
type MonthlySpend struct {
	Overall float64
	ByModel map[string]float64
}

// DailySpend holds per-day-of-month spend breakdowns for one calendar month.
// Keys are day-of-month integers (1–31); missing days had zero spend.
type DailySpend struct {
	Overall map[int]float64            // day-of-month → spend
	ByModel map[string]map[int]float64 // model → day-of-month → spend
}

// ErrHardStop is returned by Check when a hard-stop limit would be breached.
var ErrHardStop = errors.New("budget hard stop: call blocked to avoid exceeding monthly limit")

// Enforcer evaluates spend against a BudgetsConfig. It is safe for concurrent
// use after construction.
type Enforcer struct {
	cfg     config.BudgetsConfig
	logPath string
}

// New creates an Enforcer that reads spend from the default usage log
// (~/.conduit/usage.jsonl).
func New(cfg config.BudgetsConfig) (*Enforcer, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("budget: resolve home dir: %w", err)
	}
	return NewWithLogPath(cfg, filepath.Join(home, ".conduit", "usage.jsonl")), nil
}

// NewWithLogPath creates an Enforcer with an explicit log path (useful in tests).
func NewWithLogPath(cfg config.BudgetsConfig, logPath string) *Enforcer {
	return &Enforcer{cfg: cfg, logPath: logPath}
}

// Check evaluates whether a model call with the given estimated cost is allowed
// under the configured budgets for the current calendar month.
//
// It checks the model-specific budget first, then the overall budget. If a
// hard-stop limit would be breached, the returned Decision has Allowed=false
// and the error is ErrHardStop.
func (e *Enforcer) Check(model string, estimatedCostUSD float64) (Decision, error) {
	now := time.Now()
	spend, err := ReadMonthlySpend(e.logPath, now)
	if err != nil {
		// If we can't read the log, fail open (allow the call) to avoid
		// blocking all inference when the log is temporarily unavailable.
		return Decision{Allowed: true, Level: SpendLevelOK}, nil
	}

	// Model-specific limit takes precedence over overall.
	if mb, ok := e.cfg.Models[model]; ok && mb.MonthlyLimit > 0 {
		projected := spend.ByModel[model] + estimatedCostUSD
		d := evaluate(model, projected, mb.MonthlyLimit, mb.WarningPct, mb.HardStop)
		if !d.Allowed {
			return d, ErrHardStop
		}
		return d, nil
	}

	// Fall back to overall limit.
	if e.cfg.Overall.MonthlyLimit > 0 {
		projected := spend.Overall + estimatedCostUSD
		d := evaluate("overall", projected, e.cfg.Overall.MonthlyLimit, 75, false)
		if !d.Allowed {
			return d, ErrHardStop
		}
		return d, nil
	}

	return Decision{Allowed: true, Level: SpendLevelOK, Key: model}, nil
}

// Report returns the full budget status for all configured limits.
// Projected overshoot dates use a rolling 7-day window rate so the estimate
// reacts quickly to recent spending surges rather than being dampened by a
// quiet start to the month.
func (e *Enforcer) Report() Report {
	now := time.Now()
	daily, _ := ReadDailySpend(e.logPath, now)

	// Sum daily totals to get monthly totals (avoids a second log pass).
	monthlyOverall := sumDaily(daily.Overall)
	monthlyByModel := make(map[string]float64, len(daily.ByModel))
	for model, days := range daily.ByModel {
		monthlyByModel[model] = sumDaily(days)
	}

	byModel := make(map[string]ModelReport, len(e.cfg.Models))
	for name, mb := range e.cfg.Models {
		if mb.MonthlyLimit <= 0 {
			continue
		}
		spent := monthlyByModel[name]
		d := evaluate(name, spent, mb.MonthlyLimit, mb.WarningPct, mb.HardStop)
		byModel[name] = ModelReport{
			SpentUSD:               spent,
			LimitUSD:               mb.MonthlyLimit,
			PctUsed:                d.PctUsed,
			Level:                  d.Level,
			HardStop:               mb.HardStop,
			ProjectedOvershootDate: projectOvershoot(now, daily.ByModel[name], spent, mb.MonthlyLimit),
		}
	}

	var overall ModelReport
	if e.cfg.Overall.MonthlyLimit > 0 {
		d := evaluate("overall", monthlyOverall, e.cfg.Overall.MonthlyLimit, 75, false)
		overall = ModelReport{
			SpentUSD:               monthlyOverall,
			LimitUSD:               e.cfg.Overall.MonthlyLimit,
			PctUsed:                d.PctUsed,
			Level:                  d.Level,
			ProjectedOvershootDate: projectOvershoot(now, daily.Overall, monthlyOverall, e.cfg.Overall.MonthlyLimit),
		}
	}

	return Report{Overall: overall, ByModel: byModel, AsOf: now}
}

// ReadMonthlySpend scans the JSONL log at logPath and totals spend for the
// calendar month containing month. Missing files return an empty spend (no error).
func ReadMonthlySpend(logPath string, month time.Time) (MonthlySpend, error) {
	result := MonthlySpend{ByModel: make(map[string]float64)}

	f, err := os.Open(logPath)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("budget: open log: %w", err)
	}
	defer f.Close()

	y, m, _ := month.Date()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry contracts.UsageEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // skip malformed lines
		}
		ey, em, _ := entry.At.Date()
		if ey != y || em != m {
			continue
		}
		result.Overall += entry.CostUSD
		result.ByModel[entry.Model] += entry.CostUSD
	}
	return result, scanner.Err()
}

// ReadDailySpend scans the JSONL log and returns per-day-of-month spend for
// the calendar month containing month. Missing files return empty maps (no error).
func ReadDailySpend(logPath string, month time.Time) (DailySpend, error) {
	result := DailySpend{
		Overall: make(map[int]float64),
		ByModel: make(map[string]map[int]float64),
	}

	f, err := os.Open(logPath)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("budget: open log: %w", err)
	}
	defer f.Close()

	y, m, _ := month.Date()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry contracts.UsageEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		ey, em, ed := entry.At.Date()
		if ey != y || em != m {
			continue
		}
		result.Overall[ed] += entry.CostUSD
		if result.ByModel[entry.Model] == nil {
			result.ByModel[entry.Model] = make(map[int]float64)
		}
		result.ByModel[entry.Model][ed] += entry.CostUSD
	}
	return result, scanner.Err()
}

// evaluate classifies projected spend against a limit and returns a Decision.
func evaluate(key string, projectedSpend, limit float64, warningPct int, hardStop bool) Decision {
	if limit <= 0 {
		return Decision{Allowed: true, Level: SpendLevelOK, Key: key}
	}
	pct := projectedSpend / limit * 100
	warnThreshold := float64(warningPct)
	if warnThreshold <= 0 {
		warnThreshold = 75
	}

	var level SpendLevel
	switch {
	case pct >= 100 && hardStop:
		level = SpendLevelBlocked
	case pct >= 90:
		level = SpendLevelCritical
	case pct >= warnThreshold:
		level = SpendLevelWarning
	default:
		level = SpendLevelOK
	}

	return Decision{
		Allowed:  level != SpendLevelBlocked,
		Level:    level,
		Key:      key,
		SpentUSD: projectedSpend,
		LimitUSD: limit,
		PctUsed:  pct,
		HardStop: hardStop,
	}
}

// projectOvershoot estimates when spend will exceed limitUSD using the average
// daily rate over the trailing min(7, dayOfMonth) days. This 7-day window
// responds to recent spending surges without being distorted by a quiet start
// to the month. Returns nil when no overshoot is expected within the current
// calendar month.
func projectOvershoot(now time.Time, dailySpend map[int]float64, totalSpentUSD, limitUSD float64) *time.Time {
	if limitUSD <= 0 {
		return nil
	}
	remaining := limitUSD - totalSpentUSD
	if remaining <= 0 {
		// Already over budget.
		t := now
		return &t
	}

	// Trailing window: last min(7, dayOfMonth) calendar days.
	dayOfMonth := now.Day()
	windowSize := 7
	if dayOfMonth < windowSize {
		windowSize = dayOfMonth
	}
	if windowSize <= 0 {
		return nil
	}

	var windowSpend float64
	for d := dayOfMonth - windowSize + 1; d <= dayOfMonth; d++ {
		windowSpend += dailySpend[d]
	}
	dailyRate := windowSpend / float64(windowSize)
	if dailyRate <= 0 {
		return nil
	}

	daysUntilOvershoot := remaining / dailyRate
	overshootTime := now.Add(time.Duration(daysUntilOvershoot * float64(24*time.Hour)))
	if overshootTime.Month() != now.Month() || overshootTime.Year() != now.Year() {
		return nil // overshoot falls outside the current calendar month
	}
	return &overshootTime
}

// sumDaily returns the sum of all values in a day-of-month spend map.
func sumDaily(m map[int]float64) float64 {
	var total float64
	for _, v := range m {
		total += v
	}
	return total
}
