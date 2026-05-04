// Package cascade implements cost-aware cascading inference: classify each
// request by complexity using free local heuristics, try the cheapest model
// tier first, and escalate to a stronger tier only when confidence or quality
// thresholds are missed. The classifier is deliberately heuristic — no model
// call is required to pick a starting tier.
package cascade

import (
	"regexp"
	"strings"
	"unicode"
)

// Complexity classifies how hard a request likely is to satisfy.
//
// The four tiers follow PRD §16.4: trivial requests can usually be answered by
// the smallest local model; simple requests fit a small chat model; moderate
// requests benefit from a mid-tier model; complex requests need the strongest
// available model.
type Complexity string

const (
	ComplexityTrivial  Complexity = "trivial"
	ComplexitySimple   Complexity = "simple"
	ComplexityModerate Complexity = "moderate"
	ComplexityComplex  Complexity = "complex"
)

// Classification is the heuristic verdict for a single request.
type Classification struct {
	Complexity Complexity
	Score      int      // raw heuristic score, useful for logging/tuning
	Signals    []string // human-readable signals that drove the verdict
}

// Signals captures pre-computed metadata that callers can supply when they
// already know things about the request (turn count, prior tool failures, etc).
// All fields are optional; the classifier degrades gracefully without them.
type Signals struct {
	TurnCount      int  // conversation depth so far
	PriorEscalated bool // true if a prior turn already escalated
	HasAttachments bool // image/PDF/screenshot present
}

var (
	codeFenceRE = regexp.MustCompile("(?s)```")
	fileRefRE   = regexp.MustCompile(`(?:[A-Za-z0-9_./-]+\.(?:go|py|js|ts|tsx|jsx|rs|java|rb|c|h|cpp|hpp|cs|swift|kt|md|yaml|yml|json|toml|sh))`)
	urlRE       = regexp.MustCompile(`https?://\S+`)
)

// complexKeywords are reasoning-heavy verbs and phrases that almost always
// warrant a stronger model. Hits add to the heuristic score.
var complexKeywords = []string{
	"refactor", "design", "architect", "optimi", "debug", "diagnose",
	"prove", "derive", "analy", "compare", "trade-off", "tradeoff",
	"explain why", "step by step", "step-by-step", "plan",
}

// trivialKeywords are short, lookup-style requests safely served by the
// cheapest tier.
var trivialKeywords = []string{
	"what is", "define", "list", "rename", "translate",
	"yes or no", "y/n", "tldr", "tl;dr", "summari",
}

// Classify returns a heuristic verdict for prompt with optional signals.
//
// Scoring is intentionally simple and deterministic so that tuning is
// transparent: callers can log Score and Signals to understand a verdict.
func Classify(prompt string, signals Signals) Classification {
	lower := strings.ToLower(prompt)
	trimmed := strings.TrimSpace(prompt)
	score := 0
	hits := []string{}

	// Length signal — very short prompts skew trivial, very long ones complex.
	wordCount := len(strings.FieldsFunc(trimmed, func(r rune) bool {
		return unicode.IsSpace(r)
	}))
	switch {
	case wordCount == 0:
		// Empty prompt — treat as trivial; caller likely has tool inputs only.
		hits = append(hits, "empty")
	case wordCount <= 12:
		score--
		hits = append(hits, "short")
	case wordCount >= 60:
		score++
		hits = append(hits, "long")
	case wordCount >= 200:
		score += 2
		hits = append(hits, "very_long")
	}

	// Code presence — fenced blocks indicate the model will be reading or
	// writing code, which is more demanding than prose.
	if codeFenceRE.MatchString(prompt) {
		score++
		hits = append(hits, "code_block")
	}
	if fileRefs := fileRefRE.FindAllString(prompt, -1); len(fileRefs) > 0 {
		score++
		hits = append(hits, "file_ref")
		if len(fileRefs) >= 3 {
			score++
			hits = append(hits, "many_file_refs")
		}
	}

	// External URLs hint at synthesis or fetching; modest bump.
	if urlRE.MatchString(prompt) {
		score++
		hits = append(hits, "url")
	}

	// Keyword scanning.
	for _, kw := range complexKeywords {
		if strings.Contains(lower, kw) {
			score++
			hits = append(hits, "kw:"+kw)
			break
		}
	}
	for _, kw := range trivialKeywords {
		if strings.Contains(lower, kw) {
			score--
			hits = append(hits, "trivial_kw")
			break
		}
	}

	// Conversation depth — long sessions accumulate state that warrants more
	// reasoning capacity.
	switch {
	case signals.TurnCount >= 20:
		score += 2
		hits = append(hits, "deep_turn")
	case signals.TurnCount >= 8:
		score++
		hits = append(hits, "many_turns")
	}
	if signals.PriorEscalated {
		score++
		hits = append(hits, "prior_escalated")
	}
	if signals.HasAttachments {
		score++
		hits = append(hits, "attachments")
	}

	complexity := bucket(score)
	return Classification{Complexity: complexity, Score: score, Signals: hits}
}

func bucket(score int) Complexity {
	switch {
	case score <= -1:
		return ComplexityTrivial
	case score == 0:
		return ComplexitySimple
	case score <= 2:
		return ComplexityModerate
	default:
		return ComplexityComplex
	}
}
