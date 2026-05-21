package help

import (
	"strings"
	"testing"
)

func newTestRegistry() *Registry {
	r := NewRegistry()
	r.AddTopic(Topic{ID: "providers", Title: "Providers", Body: "Anthropic and friends."})
	r.AddTopic(Topic{ID: "router", Title: "Model router", Body: "Picks a provider per request."})
	r.AddTopic(Topic{ID: "memory", Title: "Memory", Body: "Long-term context."})
	return r
}

func TestTopicLookupCaseInsensitive(t *testing.T) {
	r := newTestRegistry()
	if _, ok := r.Topic("PROVIDERS"); !ok {
		t.Errorf("uppercase lookup failed")
	}
	if _, ok := r.Topic("  router  "); !ok {
		t.Errorf("whitespace not trimmed")
	}
	if _, ok := r.Topic("nonexistent"); ok {
		t.Errorf("unknown topic should miss")
	}
}

func TestSearchRanksTitleOverBody(t *testing.T) {
	r := NewRegistry()
	r.AddTopic(Topic{ID: "a", Title: "Routing primer", Body: "Lorem."})
	r.AddTopic(Topic{ID: "b", Title: "Memory", Body: "Mentions routing in passing."})
	hits := r.Search("routing")
	if len(hits) != 2 {
		t.Fatalf("want 2 hits, got %d", len(hits))
	}
	if hits[0].Topic.ID != "a" {
		t.Errorf("title hit should outrank body hit; got order %v then %v", hits[0].Topic.ID, hits[1].Topic.ID)
	}
}

func TestSearchExactSlugWins(t *testing.T) {
	r := NewRegistry()
	r.AddTopic(Topic{ID: "router", Title: "Z", Body: ""})
	r.AddTopic(Topic{ID: "config", Title: "Router this and router that", Body: "router router"})
	hits := r.Search("router")
	if hits[0].Topic.ID != "router" {
		t.Errorf("exact slug should win; got %v", hits[0].Topic.ID)
	}
}

func TestDispatchIndex(t *testing.T) {
	r := newTestRegistry()
	res := r.Dispatch("/help")
	if !res.Found {
		t.Fatalf("index should be Found")
	}
	for _, want := range []string{"providers", "router", "memory"} {
		if !strings.Contains(res.Output, want) {
			t.Errorf("index missing %q; got: %s", want, res.Output)
		}
	}
}

func TestDispatchExactTopic(t *testing.T) {
	r := newTestRegistry()
	res := r.Dispatch("/help router")
	if !res.Found {
		t.Fatalf("topic should be Found")
	}
	if !strings.Contains(res.Output, "Model router") {
		t.Errorf("missing topic title; got: %s", res.Output)
	}
}

func TestDispatchSearchFallthrough(t *testing.T) {
	r := NewRegistry()
	r.AddTopic(Topic{ID: "a", Title: "Routing", Body: ""})
	r.AddTopic(Topic{ID: "b", Title: "Routing primer", Body: ""})
	res := r.Dispatch("/help routing")
	if !res.Found {
		t.Fatalf("fallthrough search should be Found")
	}
	if !strings.Contains(res.Output, "Results for") {
		t.Errorf("expected search results header; got: %s", res.Output)
	}
}

func TestDispatchUnknown(t *testing.T) {
	r := newTestRegistry()
	res := r.Dispatch("/help quantumflux")
	if res.Found {
		t.Errorf("unknown query should not be Found")
	}
	if !strings.Contains(res.Output, "No help topic matches") {
		t.Errorf("expected miss message; got: %s", res.Output)
	}
}

func TestDispatchSearchVerb(t *testing.T) {
	r := newTestRegistry()
	res := r.Dispatch("/help search memory")
	if !res.Found || !strings.Contains(res.Output, "Memory") {
		t.Errorf("search verb failed; got: %s", res.Output)
	}
}

func TestTooltipLookup(t *testing.T) {
	tip, ok := Default.Tooltip("tui.status_bar.cost")
	if !ok {
		t.Fatal("expected tui.status_bar.cost tooltip in Default registry")
	}
	if tip.Text == "" {
		t.Error("tooltip text empty")
	}
	if tip.Topic == "" {
		t.Error("tooltip should link to a topic")
	}
	// And the linked topic must actually exist.
	if _, ok := Default.Topic(tip.Topic); !ok {
		t.Errorf("tooltip references unknown topic %q", tip.Topic)
	}
}

func TestTourPopulated(t *testing.T) {
	steps := Default.Tour()
	if len(steps) == 0 {
		t.Fatal("default tour is empty")
	}
	for i, s := range steps {
		if s.Title == "" || s.Body == "" {
			t.Errorf("tour step %d has empty title or body", i)
		}
	}
}

func TestDefaultRegistryLoaded(t *testing.T) {
	if len(Default.AllTopics()) == 0 {
		t.Fatal("Default registry has no topics — init() not running?")
	}
	for _, want := range []string{"providers", "router", "sessions", "memory", "coding", "config"} {
		if _, ok := Default.Topic(want); !ok {
			t.Errorf("Default missing required topic %q", want)
		}
	}
}
