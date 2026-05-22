package cache

import "time"

// ResponseCache stores exact-match model responses keyed by full request hash.
type ResponseCache[V any] struct {
	lru *LRU[string, V]
	ttl time.Duration
}

func NewResponseCache[V any](maxItems int, ttl time.Duration) *ResponseCache[V] {
	return &ResponseCache[V]{lru: NewLRU[string, V](maxItems), ttl: ttl}
}

func (c *ResponseCache[V]) SetClock(now func() time.Time) {
	c.lru.SetClock(now)
}

func (c *ResponseCache[V]) Get(key string) (V, bool) {
	return c.lru.Get(key)
}

func (c *ResponseCache[V]) Put(key string, value V) {
	c.lru.Put(key, value, c.ttl)
}
