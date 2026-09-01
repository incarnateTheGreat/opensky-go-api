package main

import (
	"testing"
	"time"
)

// =============================================================================
// RATE LIMITER TESTS
// =============================================================================

// TestRateLimiterAllow tests basic rate limiting
func TestRateLimiterAllow(t *testing.T) {
	// 2 requests per second, bucket size 3
	limiter := NewRateLimiter(2, 3)

	ip := "192.168.1.1"

	// First 3 requests should be allowed (bucket size)
	for i := 0; i < 3; i++ {
		if !limiter.Allow(ip) {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 4th request should be blocked (bucket empty)
	if limiter.Allow(ip) {
		t.Error("4th request should be blocked")
	}
}

// TestRateLimiterRefill tests that tokens refill over time
func TestRateLimiterRefill(t *testing.T) {
	// 10 requests per second, bucket size 2
	limiter := NewRateLimiter(10, 2)

	ip := "192.168.1.1"

	// Use all tokens
	limiter.Allow(ip)
	limiter.Allow(ip)

	// Should be blocked
	if limiter.Allow(ip) {
		t.Error("should be blocked after using all tokens")
	}

	// Wait for refill (100ms = 1 token at 10/sec)
	time.Sleep(150 * time.Millisecond)

	// Should be allowed again
	if !limiter.Allow(ip) {
		t.Error("should be allowed after token refill")
	}
}

// TestRateLimiterMultipleIPs tests that each IP has its own bucket
func TestRateLimiterMultipleIPs(t *testing.T) {
	limiter := NewRateLimiter(1, 2)

	ip1 := "192.168.1.1"
	ip2 := "192.168.1.2"

	// Use all tokens for ip1
	limiter.Allow(ip1)
	limiter.Allow(ip1)

	// ip1 should be blocked
	if limiter.Allow(ip1) {
		t.Error("ip1 should be blocked")
	}

	// ip2 should still have tokens
	if !limiter.Allow(ip2) {
		t.Error("ip2 should be allowed (separate bucket)")
	}
}

// TestRateLimiterGetRemainingTokens tests token count retrieval
func TestRateLimiterGetRemainingTokens(t *testing.T) {
	limiter := NewRateLimiter(1, 5)

	ip := "192.168.1.1"

	// New IP should have full bucket
	remaining := limiter.GetRemainingTokens(ip)
	if remaining != 5 {
		t.Errorf("expected 5 tokens for new IP, got %d", remaining)
	}

	// Use some tokens
	limiter.Allow(ip)
	limiter.Allow(ip)

	remaining = limiter.GetRemainingTokens(ip)
	if remaining != 3 {
		t.Errorf("expected 3 tokens after 2 requests, got %d", remaining)
	}
}

// TestRateLimiterBurstCapacity tests burst handling
func TestRateLimiterBurstCapacity(t *testing.T) {
	// Low rate but high burst
	limiter := NewRateLimiter(0.5, 10)

	ip := "192.168.1.1"

	// Should allow burst of 10 requests
	allowed := 0
	for i := 0; i < 15; i++ {
		if limiter.Allow(ip) {
			allowed++
		}
	}

	if allowed != 10 {
		t.Errorf("expected exactly 10 requests allowed (burst), got %d", allowed)
	}
}

// =============================================================================
// TABLE-DRIVEN TESTS
// =============================================================================

func TestRateLimiterScenarios(t *testing.T) {
	tests := []struct {
		name       string
		rate       float64
		bucketSize int
		requests   int
		expected   int // expected number of allowed requests
	}{
		{
			name:       "small bucket exhausted",
			rate:       1,
			bucketSize: 2,
			requests:   5,
			expected:   2,
		},
		{
			name:       "large bucket handles burst",
			rate:       1,
			bucketSize: 100,
			requests:   50,
			expected:   50,
		},
		{
			name:       "single token bucket",
			rate:       1,
			bucketSize: 1,
			requests:   3,
			expected:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := NewRateLimiter(tt.rate, tt.bucketSize)
			ip := "test"

			allowed := 0
			for i := 0; i < tt.requests; i++ {
				if limiter.Allow(ip) {
					allowed++
				}
			}

			if allowed != tt.expected {
				t.Errorf("expected %d allowed, got %d", tt.expected, allowed)
			}
		})
	}
}

// =============================================================================
// CONCURRENT TESTS
// =============================================================================

// TestRateLimiterConcurrent tests thread safety
func TestRateLimiterConcurrent(t *testing.T) {
	limiter := NewRateLimiter(100, 1000)

	// Spawn many goroutines making requests
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(ip string) {
			for j := 0; j < 100; j++ {
				limiter.Allow(ip)
				limiter.GetRemainingTokens(ip)
			}
			done <- true
		}(string(rune('A' + i)))
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// If we get here without panicking, thread safety is working
}
