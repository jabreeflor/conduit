package cache

import (
	"os"
	"time"
)

type FileDependency struct {
	Path    string
	ModTime time.Time
	Size    int64
}

type ToolResult[V any] struct {
	Value        V
	Files        []FileDependency
	StoredAt     time.Time
	Invalidation string
}

type ToolResultCache[V any] struct {
	lru *LRU[string, ToolResult[V]]
	ttl time.Duration
}

func NewToolResultCache[V any](maxItems int, ttl time.Duration) *ToolResultCache[V] {
	return &ToolResultCache[V]{lru: NewLRU[string, ToolResult[V]](maxItems), ttl: ttl}
}

func (c *ToolResultCache[V]) SetClock(now func() time.Time) {
	c.lru.SetClock(now)
}

func (c *ToolResultCache[V]) Get(key string) (V, bool) {
	var zero V
	result, ok := c.lru.Get(key)
	if !ok {
		return zero, false
	}
	if result.filesChanged() {
		c.lru.Delete(key)
		return zero, false
	}
	return result.Value, true
}

func (c *ToolResultCache[V]) Put(key string, value V, files []FileDependency) {
	c.lru.Put(key, ToolResult[V]{
		Value:    value,
		Files:    append([]FileDependency(nil), files...),
		StoredAt: c.lru.now(),
	}, c.ttl)
}

func SnapshotFile(path string) (FileDependency, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return FileDependency{}, false
	}
	return FileDependency{Path: path, ModTime: info.ModTime(), Size: info.Size()}, true
}

func (r ToolResult[V]) filesChanged() bool {
	for _, dep := range r.Files {
		info, err := os.Stat(dep.Path)
		if err != nil || info.IsDir() || !info.ModTime().Equal(dep.ModTime) || info.Size() != dep.Size {
			return true
		}
	}
	return false
}
