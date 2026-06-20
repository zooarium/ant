// Package cache provides a minimal in-memory TTL cache with lazy expiry.
//
// It is safe for concurrent use and requires no background goroutines:
// expired entries are evicted on read.
package cache

import (
	"sync"
	"time"
)

type entry struct {
	value     any
	expiresAt time.Time
}

// TTLCache is a concurrency-safe map-backed cache where every entry expires
// after a fixed TTL.
type TTLCache struct {
	mu  sync.RWMutex
	ttl time.Duration
	m   map[string]entry
}

// New returns a TTLCache that expires entries after ttl.
func New(ttl time.Duration) *TTLCache {
	return &TTLCache{
		ttl: ttl,
		m:   make(map[string]entry),
	}
}

// Get returns the value stored for key and whether it was present and unexpired.
// Expired entries are treated as absent (and lazily evicted).
func (c *TTLCache) Get(key string) (any, bool) {
	c.mu.RLock()
	e, ok := c.m[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expiresAt) {
		c.mu.Lock()
		// Re-check under the write lock to avoid racing a concurrent Set.
		if cur, still := c.m[key]; still && time.Now().After(cur.expiresAt) {
			delete(c.m, key)
		}
		c.mu.Unlock()
		return nil, false
	}
	return e.value, true
}

// Set stores value for key with the cache TTL.
func (c *TTLCache) Set(key string, value any) {
	c.mu.Lock()
	c.m[key] = entry{value: value, expiresAt: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}

// Delete removes a single entry, used for explicit invalidation on writes.
func (c *TTLCache) Delete(key string) {
	c.mu.Lock()
	delete(c.m, key)
	c.mu.Unlock()
}

// Purge removes all entries from the cache.
func (c *TTLCache) Purge() {
	c.mu.Lock()
	c.m = make(map[string]entry)
	c.mu.Unlock()
}
