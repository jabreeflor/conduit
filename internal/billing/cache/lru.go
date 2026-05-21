// Package cache contains reusable cache primitives for model and tool calls.
package cache

import (
	"container/list"
	"sync"
	"time"
)

type clock func() time.Time

// LRU is a small concurrency-safe in-memory cache with optional TTL support.
type LRU[K comparable, V any] struct {
	mu       sync.Mutex
	maxItems int
	now      clock
	ll       *list.List
	items    map[K]*list.Element
}

type entry[K comparable, V any] struct {
	key       K
	value     V
	expiresAt time.Time
}

// NewLRU creates a cache capped at maxItems. maxItems <= 0 disables storage.
func NewLRU[K comparable, V any](maxItems int) *LRU[K, V] {
	return &LRU[K, V]{
		maxItems: maxItems,
		now:      func() time.Time { return time.Now().UTC() },
		ll:       list.New(),
		items:    make(map[K]*list.Element),
	}
}

func (c *LRU[K, V]) SetClock(now func() time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if now != nil {
		c.now = now
	}
}

func (c *LRU[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var zero V
	el, ok := c.items[key]
	if !ok {
		return zero, false
	}
	ent := el.Value.(entry[K, V])
	if !ent.expiresAt.IsZero() && !c.now().Before(ent.expiresAt) {
		c.ll.Remove(el)
		delete(c.items, key)
		return zero, false
	}
	c.ll.MoveToFront(el)
	return ent.value, true
}

func (c *LRU[K, V]) Put(key K, value V, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.maxItems <= 0 {
		return
	}

	expiresAt := time.Time{}
	if ttl > 0 {
		expiresAt = c.now().Add(ttl)
	}
	if el, ok := c.items[key]; ok {
		el.Value = entry[K, V]{key: key, value: value, expiresAt: expiresAt}
		c.ll.MoveToFront(el)
		return
	}

	c.items[key] = c.ll.PushFront(entry[K, V]{key: key, value: value, expiresAt: expiresAt})
	for len(c.items) > c.maxItems {
		back := c.ll.Back()
		if back == nil {
			return
		}
		ent := back.Value.(entry[K, V])
		delete(c.items, ent.key)
		c.ll.Remove(back)
	}
}

func (c *LRU[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.Remove(el)
		delete(c.items, key)
	}
}

func (c *LRU[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}
