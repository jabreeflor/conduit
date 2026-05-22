package cache

import "time"

// PrefixPlan separates stable prompt content from volatile request content so
// providers with prompt caching can attach their cache hints to the prefix.
type PrefixPlan struct {
	Prefix       string
	Volatile     string
	PrefixKey    string
	CacheControl string
}

func PlanPrefix(stableParts []string, volatile string) PrefixPlan {
	prefix := ""
	for _, part := range stableParts {
		prefix += part
	}
	return PrefixPlan{
		Prefix:       prefix,
		Volatile:     volatile,
		PrefixKey:    MustKey("prompt-prefix", prefix),
		CacheControl: "cacheable-prefix",
	}
}

// KVCache tracks warm local-model contexts with LRU eviction.
type KVCache[V any] struct {
	lru *LRU[string, V]
	ttl time.Duration
}

func NewKVCache[V any](maxItems int, ttl time.Duration) *KVCache[V] {
	return &KVCache[V]{lru: NewLRU[string, V](maxItems), ttl: ttl}
}

func (c *KVCache[V]) SetClock(now func() time.Time) {
	c.lru.SetClock(now)
}

func (c *KVCache[V]) Get(prefixKey string) (V, bool) {
	return c.lru.Get(prefixKey)
}

func (c *KVCache[V]) Put(prefixKey string, handle V) {
	c.lru.Put(prefixKey, handle, c.ttl)
}
