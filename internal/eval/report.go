package eval

import "sort"

// Summarize groups results by model.
func Summarize(results []CaseResult) []Summary {
	type acc struct {
		s Summary
		m metricAcc
	}
	byModel := map[string]*acc{}
	for _, result := range results {
		a := byModel[result.Model]
		if a == nil {
			a = &acc{s: Summary{Model: result.Model}}
			byModel[result.Model] = a
		}
		a.s.Total++
		if result.Passed {
			a.s.Passed++
		}
		a.s.AvgCostUSD += result.Observed.CostUSD
		a.s.AvgLatencySecs += result.Observed.DurationSeconds
		a.m.add(result.Metrics)
	}

	out := make([]Summary, 0, len(byModel))
	for _, a := range byModel {
		if a.s.Total > 0 {
			a.s.ScorePercent = float64(a.s.Passed) / float64(a.s.Total) * 100
			a.s.AvgCostUSD /= float64(a.s.Total)
			a.s.AvgLatencySecs /= float64(a.s.Total)
		}
		a.s.Metrics = a.m.summary()
		out = append(out, a.s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ScorePercent == out[j].ScorePercent {
			return out[i].Model < out[j].Model
		}
		return out[i].ScorePercent > out[j].ScorePercent
	})
	return out
}

type metricAcc struct {
	total               int
	instructionFollowed int
	toolSelectionOK     int
	hookCompliant       int
	contextRetained     int
	structuredTagOK     int
	workflowCompleted   int
	injectionResistant  int
	escalated           int
	costUSD             float64
	latencySeconds      float64
}

func (a *metricAcc) add(m HarnessMetricEvent) {
	a.total++
	a.instructionFollowed += bit(m.InstructionFollowed)
	a.toolSelectionOK += bit(m.ToolSelectionOK)
	a.hookCompliant += bit(m.HookCompliant)
	a.contextRetained += bit(m.ContextRetained)
	a.structuredTagOK += bit(m.StructuredTagOK)
	a.workflowCompleted += bit(m.WorkflowCompleted)
	a.injectionResistant += bit(m.InjectionResistant)
	a.escalated += bit(m.Escalated)
	a.costUSD += m.CostUSD
	a.latencySeconds += m.LatencySeconds
}

func (a metricAcc) summary() MetricSummary {
	if a.total == 0 {
		return MetricSummary{}
	}
	return MetricSummary{
		InstructionFollowRate: pct(a.instructionFollowed, a.total),
		ToolSelectionAccuracy: pct(a.toolSelectionOK, a.total),
		HookCompliance:        pct(a.hookCompliant, a.total),
		ContextRetention:      pct(a.contextRetained, a.total),
		StructuredTagFidelity: pct(a.structuredTagOK, a.total),
		WorkflowCompletion:    pct(a.workflowCompleted, a.total),
		InjectionResistance:   pct(a.injectionResistant, a.total),
		CostEfficiencyUSD:     a.costUSD / float64(a.total),
		LatencySeconds:        a.latencySeconds / float64(a.total),
		EscalationTriggerRate: pct(a.escalated, a.total),
	}
}

func bit(ok bool) int {
	if ok {
		return 1
	}
	return 0
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}
