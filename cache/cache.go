package cache

import (
	"sync"
	"time"
)

// Cache is a generic thread-safe in-memory cache with TTL expiration.
type Cache[T any] struct {
	mu        sync.RWMutex
	value     T
	cachedAt  time.Time
	ttl       time.Duration
	populated bool
}

// New creates a new Cache with the given TTL. A TTL of 0 disables caching.
func New[T any](ttl time.Duration) *Cache[T] {
	return &Cache[T]{ttl: ttl}
}

// Get returns the cached value and true if it exists and has not expired.
// Returns the zero value and false otherwise.
func (c *Cache[T]) Get() (T, bool) {
	if c.ttl <= 0 {
		var zero T
		return zero, false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.populated {
		var zero T
		return zero, false
	}

	if time.Since(c.cachedAt) > c.ttl {
		var zero T
		return zero, false
	}

	return c.value, true
}

// Set stores a value in the cache with the current timestamp.
func (c *Cache[T]) Set(value T) {
	if c.ttl <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.value = value
	c.cachedAt = time.Now()
	c.populated = true
}

// Invalidate clears the cached value.
func (c *Cache[T]) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()

	var zero T
	c.value = zero
	c.populated = false
}
