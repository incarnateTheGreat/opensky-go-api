package main

import (
	"testing"
	"time"
)

// =============================================================================
// CACHE TESTS
// =============================================================================

// TestCacheSetAndGet tests basic cache operations
func TestCacheSetAndGet(t *testing.T) {
	cache := NewCache()

	// Test setting and getting a value
	cache.Set("key1", "value1", 1*time.Minute)

	value, found := cache.Get("key1")
	if !found {
		t.Error("expected to find key1, but it was not found")
	}
	if value != "value1" {
		t.Errorf("expected value1, got %v", value)
	}
}

// TestCacheGetMiss tests cache miss behavior
func TestCacheGetMiss(t *testing.T) {
	cache := NewCache()

	value, found := cache.Get("nonexistent")
	if found {
		t.Error("expected cache miss for nonexistent key")
	}
	if value != nil {
		t.Errorf("expected nil value for cache miss, got %v", value)
	}
}

// TestCacheExpiration tests that expired entries are not returned
func TestCacheExpiration(t *testing.T) {
	cache := NewCache()

	// Set with very short TTL
	cache.Set("expires", "soon", 50*time.Millisecond)

	// Should exist immediately
	_, found := cache.Get("expires")
	if !found {
		t.Error("expected to find key immediately after setting")
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Should be gone now
	_, found = cache.Get("expires")
	if found {
		t.Error("expected key to be expired")
	}
}

// TestCacheDelete tests explicit deletion
func TestCacheDelete(t *testing.T) {
	cache := NewCache()

	cache.Set("toDelete", "value", 1*time.Minute)
	cache.Delete("toDelete")

	_, found := cache.Get("toDelete")
	if found {
		t.Error("expected key to be deleted")
	}
}

// TestCacheClear tests clearing all entries
func TestCacheClear(t *testing.T) {
	cache := NewCache()

	cache.Set("key1", "value1", 1*time.Minute)
	cache.Set("key2", "value2", 1*time.Minute)
	cache.Set("key3", "value3", 1*time.Minute)

	if cache.Size() != 3 {
		t.Errorf("expected 3 entries, got %d", cache.Size())
	}

	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("expected 0 entries after clear, got %d", cache.Size())
	}
}

// TestCacheCleanup tests the cleanup function
func TestCacheCleanup(t *testing.T) {
	cache := NewCache()

	// Add some entries with different TTLs
	cache.Set("expires1", "value", 50*time.Millisecond)
	cache.Set("expires2", "value", 50*time.Millisecond)
	cache.Set("stays", "value", 1*time.Minute)

	// Wait for some to expire
	time.Sleep(100 * time.Millisecond)

	// Run cleanup
	removed := cache.Cleanup()
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}

	// Only "stays" should remain
	if cache.Size() != 1 {
		t.Errorf("expected 1 entry remaining, got %d", cache.Size())
	}

	_, found := cache.Get("stays")
	if !found {
		t.Error("expected 'stays' to still exist")
	}
}

// TestCacheOverwrite tests that setting the same key overwrites
func TestCacheOverwrite(t *testing.T) {
	cache := NewCache()

	cache.Set("key", "original", 1*time.Minute)
	cache.Set("key", "updated", 1*time.Minute)

	value, _ := cache.Get("key")
	if value != "updated" {
		t.Errorf("expected 'updated', got %v", value)
	}
}

// TestCacheDifferentTypes tests storing different types
func TestCacheDifferentTypes(t *testing.T) {
	cache := NewCache()

	// Store different types
	cache.Set("string", "hello", 1*time.Minute)
	cache.Set("int", 42, 1*time.Minute)
	cache.Set("slice", []int{1, 2, 3}, 1*time.Minute)
	cache.Set("struct", struct{ Name string }{"test"}, 1*time.Minute)

	// Retrieve and type assert
	if v, _ := cache.Get("string"); v != "hello" {
		t.Errorf("string: expected 'hello', got %v", v)
	}

	if v, _ := cache.Get("int"); v != 42 {
		t.Errorf("int: expected 42, got %v", v)
	}

	if v, _ := cache.Get("slice"); len(v.([]int)) != 3 {
		t.Errorf("slice: expected length 3, got %d", len(v.([]int)))
	}
}

// =============================================================================
// TABLE-DRIVEN TESTS (idiomatic Go pattern)
// =============================================================================

// TestCacheTTL uses table-driven tests to check various TTL scenarios
func TestCacheTTL(t *testing.T) {
	tests := []struct {
		name       string
		ttl        time.Duration
		waitTime   time.Duration
		shouldFind bool
	}{
		{
			name:       "not expired",
			ttl:        1 * time.Second,
			waitTime:   10 * time.Millisecond,
			shouldFind: true,
		},
		{
			name:       "just expired",
			ttl:        50 * time.Millisecond,
			waitTime:   100 * time.Millisecond,
			shouldFind: false,
		},
		{
			name:       "zero wait",
			ttl:        100 * time.Millisecond,
			waitTime:   0,
			shouldFind: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewCache()
			cache.Set("test", "value", tt.ttl)

			time.Sleep(tt.waitTime)

			_, found := cache.Get("test")
			if found != tt.shouldFind {
				t.Errorf("expected found=%v, got found=%v", tt.shouldFind, found)
			}
		})
	}
}

func TestCacheGetStale(t *testing.T) {
	cache := NewCache()

	cache.Set("stale", "value", 30*time.Millisecond)
	time.Sleep(60 * time.Millisecond)

	value, found, staleAge := cache.GetStale("stale")
	if !found {
		t.Fatal("expected stale key to be found")
	}
	if value != "value" {
		t.Fatalf("expected stale value, got %v", value)
	}
	if staleAge <= 0 {
		t.Fatalf("expected positive stale age, got %v", staleAge)
	}
}
