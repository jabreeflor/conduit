package cache

import (
	"math"
	"time"
)

// SemanticEntry stores an embedding and response for near-duplicate prompts.
type SemanticEntry[V any] struct {
	Key       string
	Embedding []float64
	Value     V
}

type SemanticCache[V any] struct {
	lru       *LRU[string, SemanticEntry[V]]
	ttl       time.Duration
	threshold float64
}

func NewSemanticCache[V any](maxItems int, ttl time.Duration, threshold float64) *SemanticCache[V] {
	if threshold <= 0 {
		threshold = 0.95
	}
	return &SemanticCache[V]{
		lru:       NewLRU[string, SemanticEntry[V]](maxItems),
		ttl:       ttl,
		threshold: threshold,
	}
}

func (c *SemanticCache[V]) SetClock(now func() time.Time) {
	c.lru.SetClock(now)
}

func (c *SemanticCache[V]) Put(key string, embedding []float64, value V) {
	c.lru.Put(key, SemanticEntry[V]{
		Key:       key,
		Embedding: append([]float64(nil), embedding...),
		Value:     value,
	}, c.ttl)
}

func (c *SemanticCache[V]) Lookup(embedding []float64) (V, string, float64, bool) {
	var zero V
	bestKey := ""
	bestScore := -1.0
	var best V

	c.lru.mu.Lock()
	keys := make([]string, 0, len(c.lru.items))
	for key := range c.lru.items {
		keys = append(keys, key)
	}
	c.lru.mu.Unlock()

	for _, key := range keys {
		entry, ok := c.lru.Get(key)
		if !ok {
			continue
		}
		score := CosineSimilarity(embedding, entry.Embedding)
		if score > bestScore {
			bestScore = score
			bestKey = entry.Key
			best = entry.Value
		}
	}
	if bestScore < c.threshold {
		return zero, "", bestScore, false
	}
	return best, bestKey, bestScore, true
}

func CosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
