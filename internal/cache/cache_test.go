package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLRUEvictsLeastRecentlyUsedAndExpires(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	c := NewLRU[string, string](2)
	c.SetClock(func() time.Time { return now })

	c.Put("a", "A", time.Minute)
	c.Put("b", "B", time.Minute)
	if _, ok := c.Get("a"); !ok {
		t.Fatal("expected a before eviction")
	}
	c.Put("c", "C", time.Minute)
	if _, ok := c.Get("b"); ok {
		t.Fatal("b should have been LRU-evicted")
	}

	now = now.Add(time.Minute)
	if _, ok := c.Get("a"); ok {
		t.Fatal("a should expire at ttl boundary")
	}
}

func TestToolResultCacheInvalidatesWhenFileChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readme.txt")
	if err := os.WriteFile(path, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	dep, ok := SnapshotFile(path)
	if !ok {
		t.Fatal("expected file snapshot")
	}

	c := NewToolResultCache[string](4, time.Hour)
	c.Put("read", "one", []FileDependency{dep})
	if got, ok := c.Get("read"); !ok || got != "one" {
		t.Fatalf("cached value = %q/%v, want one/true", got, ok)
	}

	time.Sleep(time.Millisecond)
	if err := os.WriteFile(path, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("read"); ok {
		t.Fatal("cache should miss after file mutation")
	}
}

func TestSemanticCacheFindsSimilarEmbedding(t *testing.T) {
	c := NewSemanticCache[string](4, time.Hour, 0.95)
	c.Put("auth", []float64{1, 0, 0}, "cached response")

	got, key, score, ok := c.Lookup([]float64{0.99, 0.01, 0})
	if !ok {
		t.Fatalf("expected semantic hit, score=%f", score)
	}
	if key != "auth" || got != "cached response" {
		t.Fatalf("hit = %q/%q, want auth/cached response", key, got)
	}
}

func TestPlanPrefixReturnsStableKey(t *testing.T) {
	a := PlanPrefix([]string{"system", "tools"}, "message")
	b := PlanPrefix([]string{"system", "tools"}, "different")
	if a.PrefixKey != b.PrefixKey {
		t.Fatal("volatile prompt content should not alter prefix key")
	}
	if a.Prefix != "systemtools" || a.Volatile != "message" {
		t.Fatalf("plan = %#v", a)
	}
}
