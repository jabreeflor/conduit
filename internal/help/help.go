// Package help is the in-app documentation surface for Conduit (PRD §17.3).
//
// It owns:
//   - a registry of help Topics (id, title, body, related),
//   - a fuzzy-ish text Search() over title + body,
//   - a Dispatch() entry point used by the TUI's `/help` slash command,
//   - a catalog of UI Tooltips keyed by element id, used by the TUI / GUI to
//     render the small help bubbles next to settings, status pills, etc.,
//   - a Tour structure used by the first-launch guided tour.
//
// All content lives in topics.go / tooltips.go / tour.go. Surfaces query this
// package so help text is single-sourced — the docs site (issue #137), the
// TUI tooltips, and the `/help` command all read from the same registry.
package help

import (
	"fmt"
	"sort"
	"strings"
)

// Topic is one help article.
type Topic struct {
	// ID is the slug used by `/help <id>` and "Learn more" deep links.
	ID string
	// Title is the human-readable heading.
	Title string
	// Body is markdown-flavored plain text (TUI renders as plain text;
	// docs site renders as markdown).
	Body string
	// Related lists IDs of adjacent topics, surfaced as a "See also" list.
	Related []string
	// LearnMore is an optional URL into the docs site; used by the
	// contextual "Learn more" links on friction points.
	LearnMore string
}

// Tooltip is the short hover-text for one UI element.
type Tooltip struct {
	// Element is the surface-specific id, e.g. "tui.status_bar.cost",
	// "gui.settings.providers.priority".
	Element string
	// Text is one or two short sentences. Keep under 140 chars.
	Text string
	// Topic, if non-empty, points at a Topic.ID for "Learn more".
	Topic string
}

// TourStep is one step of the first-launch guided tour.
type TourStep struct {
	// Element is a UI anchor id the surface can highlight.
	Element string
	// Title is shown as the step heading.
	Title string
	// Body is shown as the step body.
	Body string
}

// Registry holds all help content. Surfaces use the package-level Default
// registry; Registry exists as a separate type so tests can build isolated
// registries without touching package state.
type Registry struct {
	topics   map[string]Topic
	tooltips map[string]Tooltip
	tour     []TourStep
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		topics:   map[string]Topic{},
		tooltips: map[string]Tooltip{},
	}
}

// AddTopic adds or replaces a topic.
func (r *Registry) AddTopic(t Topic) {
	r.topics[t.ID] = t
}

// AddTooltip adds or replaces a tooltip.
func (r *Registry) AddTooltip(t Tooltip) {
	r.tooltips[t.Element] = t
}

// SetTour replaces the tour.
func (r *Registry) SetTour(steps []TourStep) {
	r.tour = steps
}

// Topic looks up a topic by ID. Match is case-insensitive on the slug.
func (r *Registry) Topic(id string) (Topic, bool) {
	t, ok := r.topics[strings.ToLower(strings.TrimSpace(id))]
	return t, ok
}

// Tooltip looks up a tooltip by element id.
func (r *Registry) Tooltip(element string) (Tooltip, bool) {
	t, ok := r.tooltips[element]
	return t, ok
}

// Tour returns the tour steps in order.
func (r *Registry) Tour() []TourStep {
	out := make([]TourStep, len(r.tour))
	copy(out, r.tour)
	return out
}

// AllTopics returns every topic, sorted by ID. Useful for `/help` with no
// arguments (acts as an index).
func (r *Registry) AllTopics() []Topic {
	out := make([]Topic, 0, len(r.topics))
	for _, t := range r.topics {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// SearchHit is one match with a small relevance score (higher is better).
type SearchHit struct {
	Topic Topic
	Score int
}

// Search runs a case-insensitive substring search over title + body. Title
// hits weigh more than body hits, and an exact slug match wins outright.
// Designed to be searchable from the command palette (PRD §17.3).
func (r *Registry) Search(query string) []SearchHit {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	var hits []SearchHit
	for _, t := range r.topics {
		score := 0
		switch {
		case strings.EqualFold(t.ID, q):
			score = 1000
		case strings.Contains(strings.ToLower(t.ID), q):
			score = 50
		}
		if strings.Contains(strings.ToLower(t.Title), q) {
			score += 20
		}
		if strings.Contains(strings.ToLower(t.Body), q) {
			score += 5
		}
		if score > 0 {
			hits = append(hits, SearchHit{Topic: t, Score: score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	return hits
}

// DispatchResult is what the TUI prints after running `/help …`.
type DispatchResult struct {
	Output string
	Found  bool
}

// Dispatch implements the `/help` slash command. The TUI calls
// `help.Default.Dispatch(line)` and prints `.Output` into the conversation.
//
// Forms:
//
//	/help                      → index of all topics
//	/help <topic-id>           → render that topic
//	/help search <query>       → search results
//	/help <free text>          → falls through to search if no topic match
func (r *Registry) Dispatch(line string) DispatchResult {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "/help")
	line = strings.TrimSpace(line)

	if line == "" {
		return DispatchResult{Output: r.renderIndex(), Found: true}
	}

	if strings.HasPrefix(line, "search ") {
		return DispatchResult{Output: r.renderSearch(strings.TrimPrefix(line, "search ")), Found: true}
	}

	if t, ok := r.Topic(line); ok {
		return DispatchResult{Output: r.renderTopic(t), Found: true}
	}

	// Fall through to a search; this is what users want when they type
	// `/help workflows` and there's no exact slug.
	hits := r.Search(line)
	if len(hits) == 0 {
		return DispatchResult{
			Output: fmt.Sprintf("No help topic matches %q.\nTry `/help` for the index.", line),
			Found:  false,
		}
	}
	if len(hits) == 1 {
		return DispatchResult{Output: r.renderTopic(hits[0].Topic), Found: true}
	}
	return DispatchResult{Output: r.renderSearch(line), Found: true}
}

func (r *Registry) renderIndex() string {
	var b strings.Builder
	b.WriteString("Available help topics:\n\n")
	for _, t := range r.AllTopics() {
		fmt.Fprintf(&b, "  %-22s %s\n", t.ID, t.Title)
	}
	b.WriteString("\nUse `/help <topic>` or `/help search <query>`.")
	return b.String()
}

func (r *Registry) renderTopic(t Topic) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s\n", t.Title, strings.TrimSpace(t.Body))
	if len(t.Related) > 0 {
		b.WriteString("\nSee also: ")
		b.WriteString(strings.Join(t.Related, ", "))
		b.WriteString(".")
	}
	if t.LearnMore != "" {
		fmt.Fprintf(&b, "\nLearn more: %s", t.LearnMore)
	}
	return b.String()
}

func (r *Registry) renderSearch(query string) string {
	hits := r.Search(query)
	if len(hits) == 0 {
		return fmt.Sprintf("No help topic matches %q.", query)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Results for %q:\n\n", query)
	for _, h := range hits {
		fmt.Fprintf(&b, "  %-22s %s\n", h.Topic.ID, h.Topic.Title)
	}
	return b.String()
}

// Default is the package-level registry, populated by topics.go / tooltips.go
// / tour.go in their init() functions. Surfaces should use this directly.
var Default = NewRegistry()
