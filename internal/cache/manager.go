package cache

import (
	"sync"
	"time"
)

// CacheKey is a tagged key for use with the CacheManager.
type CacheKey struct {
	Type  string // "prefix", "kv", "semantic", "tool", "response"
	Value string
}

// CacheStats provides counts of cached items across all cache types.
type CacheStats struct {
	PrefixCacheSize    int
	KVCacheSize        int
	SemanticCacheSize  int
	ToolResultCacheSize int
	ResponseCacheSize  int
	Total              int
}

// CacheManager composes all cache types into a single point of configuration.
type CacheManager struct {
	mu                sync.Mutex
	prefixCache       *KVCache[string]
	kvCache           *KVCache[string]
	semanticCache     *SemanticCache[string]
	toolResultCache   *ToolResultCache[string]
	responseCache     *ResponseCache[string]
	enabled           bool
	cacheableTools    map[string]bool
}

// NewCacheManager creates a new manager with default TTLs and sizes.
// Set enabled to false to bypass all caching operations.
func NewCacheManager(enabled bool, cacheableTools []string) *CacheManager {
	cm := &CacheManager{
		prefixCache:       NewKVCache[string](100, 24*time.Hour),
		kvCache:           NewKVCache[string](500, 24*time.Hour),
		semanticCache:     NewSemanticCache[string](200, 24*time.Hour, 0.95),
		toolResultCache:   NewToolResultCache[string](1000, 7*24*time.Hour),
		responseCache:     NewResponseCache[string](500, 24*time.Hour),
		enabled:           enabled,
		cacheableTools:    make(map[string]bool),
	}
	for _, tool := range cacheableTools {
		cm.cacheableTools[tool] = true
	}
	return cm
}

// Get retrieves a value from the appropriate cache type.
// Returns ("", false) if not found or if caching is disabled.
func (m *CacheManager) Get(key CacheKey) (string, bool) {
	if !m.enabled {
		return "", false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	switch key.Type {
	case "prefix":
		return m.prefixCache.Get(key.Value)
	case "kv":
		return m.kvCache.Get(key.Value)
	case "semantic":
		return m.semanticCache.Get(key.Value)
	case "tool":
		return m.toolResultCache.Get(key.Value)
	case "response":
		return m.responseCache.Get(key.Value)
	default:
		return "", false
	}
}

// Set stores a value in the appropriate cache type.
// Does nothing if caching is disabled.
func (m *CacheManager) Set(key CacheKey, value string) {
	if !m.enabled {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	switch key.Type {
	case "prefix":
		m.prefixCache.Put(key.Value, value)
	case "kv":
		m.kvCache.Put(key.Value, value)
	case "semantic":
		// semantic cache requires embedding; use Lookup instead
	case "tool":
		m.toolResultCache.Put(key.Value, value, nil)
	case "response":
		m.responseCache.Put(key.Value, value)
	}
}

// IsCacheableTool checks if a tool is in the allow-list for result caching.
func (m *CacheManager) IsCacheableTool(toolName string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cacheableTools[toolName]
}

// Stats returns cache statistics for monitoring and debugging.
func (m *CacheManager) Stats() CacheStats {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats := CacheStats{
		PrefixCacheSize:    m.prefixCache.lru.Len(),
		KVCacheSize:        m.kvCache.lru.Len(),
		SemanticCacheSize:  m.semanticCache.lru.Len(),
		ToolResultCacheSize: m.toolResultCache.lru.Len(),
		ResponseCacheSize:  m.responseCache.lru.Len(),
	}
	stats.Total = stats.PrefixCacheSize + stats.KVCacheSize + stats.SemanticCacheSize +
		stats.ToolResultCacheSize + stats.ResponseCacheSize
	return stats
}

// Flush clears all caches.
func (m *CacheManager) Flush() {
	if !m.enabled {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.prefixCache = NewKVCache[string](100, 24*time.Hour)
	m.kvCache = NewKVCache[string](500, 24*time.Hour)
	m.semanticCache = NewSemanticCache[string](200, 24*time.Hour, 0.95)
	m.toolResultCache = NewToolResultCache[string](1000, 7*24*time.Hour)
	m.responseCache = NewResponseCache[string](500, 24*time.Hour)
}

// SetClock sets the clock function for all caches (used in tests).
func (m *CacheManager) SetClock(now func() time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.prefixCache.SetClock(now)
	m.kvCache.SetClock(now)
	m.semanticCache.SetClock(now)
	m.toolResultCache.SetClock(now)
	m.responseCache.SetClock(now)
}
