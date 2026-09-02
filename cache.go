package main

import (
	"sync"
	"time"
)

// =============================================================================
// IN-MEMORY CACHE - Thread-safe caching with TTL
// =============================================================================

// CacheEntry holds a cached value and its expiration time
type CacheEntry struct {
	Value      interface{} // The cached data (any type)
	Expiration time.Time   // When this entry expires
}

// Cache is a thread-safe in-memory cache with TTL support
// Uses sync.RWMutex to allow concurrent reads while blocking writes
type Cache struct {
	data map[string]CacheEntry
	mu   sync.RWMutex
}

// NewCache creates a new empty cache
func NewCache() *Cache {
	return &Cache{
		data: make(map[string]CacheEntry),
	}
}

// Get retrieves a value from the cache
// Returns the value and true if found and not expired
// Returns nil and false if not found or expired
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	entry, found := c.data[key]
	c.mu.RUnlock()

	if !found {
		return nil, false
	}

	// Check if expired
	if time.Now().After(entry.Expiration) {
		return nil, false
	}

	return entry.Value, true
}

// GetStale retrieves a value even if it is expired.
// Returns staleAge > 0 when the entry is expired.
func (c *Cache) GetStale(key string) (interface{}, bool, time.Duration) {
	c.mu.RLock()
	entry, found := c.data[key]
	c.mu.RUnlock()

	if !found {
		return nil, false, 0
	}

	now := time.Now()
	if now.After(entry.Expiration) {
		return entry.Value, true, now.Sub(entry.Expiration)
	}

	return entry.Value, true, 0
}

// Set stores a value in the cache with a TTL (time to live)
func (c *Cache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = CacheEntry{
		Value:      value,
		Expiration: time.Now().Add(ttl),
	}
}

// Delete removes a key from the cache
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
}

// Clear removes all entries from the cache
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = make(map[string]CacheEntry)
}

// Size returns the number of entries in the cache (including expired ones)
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}

// Cleanup removes all expired entries from the cache
// Can be called periodically via a goroutine to prevent memory bloat
func (c *Cache) Cleanup() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	removed := 0

	for key, entry := range c.data {
		if now.After(entry.Expiration) {
			delete(c.data, key)
			removed++
		}
	}

	return removed
}

// StartCleanupRoutine starts a background goroutine that periodically cleans expired entries
// Returns a channel that can be closed to stop the cleanup routine
func (c *Cache) StartCleanupRoutine(interval time.Duration) chan struct{} {
	stop := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_ = c.Cleanup()
			case <-stop:
				return
			}
		}
	}()

	return stop
}

// =============================================================================
// GLOBAL CACHE INSTANCES
// =============================================================================

// flightCache caches OpenSky flight data (short TTL - data updates frequently)
var flightCache = NewCache()

// Cache TTL constants
const (
	FlightCacheTTL = 15 * time.Second // OpenSky updates every ~10 seconds
)
