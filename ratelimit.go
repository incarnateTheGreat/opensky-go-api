package main

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// =============================================================================
// RATE LIMITER - Token bucket algorithm with per-IP tracking
// =============================================================================

// RateLimiter implements the token bucket algorithm
// Each IP gets a bucket that fills with tokens over time
// Each request consumes one token; if no tokens, request is rejected
type RateLimiter struct {
	buckets    map[string]*tokenBucket
	mu         sync.RWMutex
	rate       float64       // Tokens added per second
	bucketSize int           // Maximum tokens in bucket
	cleanup    time.Duration // How often to clean up old buckets
}

// tokenBucket tracks tokens for a single IP
type tokenBucket struct {
	tokens     float64   // Current token count
	lastUpdate time.Time // Last time tokens were added
}

// NewRateLimiter creates a rate limiter
// rate: requests per second allowed
// bucketSize: burst capacity (max requests at once)
func NewRateLimiter(rate float64, bucketSize int) *RateLimiter {
	rl := &RateLimiter{
		buckets:    make(map[string]*tokenBucket),
		rate:       rate,
		bucketSize: bucketSize,
		cleanup:    5 * time.Minute,
	}

	// Start cleanup goroutine to prevent memory bloat
	go rl.cleanupRoutine()

	return rl
}

// Allow checks if a request from the given IP should be allowed
// Returns true if allowed, false if rate limited
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	bucket, exists := rl.buckets[ip]

	if !exists {
		// New IP - create bucket with full tokens
		rl.buckets[ip] = &tokenBucket{
			tokens:     float64(rl.bucketSize) - 1, // -1 for this request
			lastUpdate: now,
		}
		return true
	}

	// Calculate tokens to add based on time elapsed
	elapsed := now.Sub(bucket.lastUpdate).Seconds()
	bucket.tokens += elapsed * rl.rate

	// Cap at bucket size
	if bucket.tokens > float64(rl.bucketSize) {
		bucket.tokens = float64(rl.bucketSize)
	}

	bucket.lastUpdate = now

	// Check if we have a token to spend
	if bucket.tokens >= 1 {
		bucket.tokens--
		return true
	}

	return false
}

// GetRemainingTokens returns how many tokens an IP has left
func (rl *RateLimiter) GetRemainingTokens(ip string) int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	bucket, exists := rl.buckets[ip]
	if !exists {
		return rl.bucketSize
	}

	// Calculate current tokens (without modifying)
	elapsed := time.Since(bucket.lastUpdate).Seconds()
	tokens := bucket.tokens + elapsed*rl.rate
	if tokens > float64(rl.bucketSize) {
		tokens = float64(rl.bucketSize)
	}

	return int(tokens)
}

// cleanupRoutine removes stale buckets to prevent memory leaks
func (rl *RateLimiter) cleanupRoutine() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		staleThreshold := 10 * time.Minute

		for ip, bucket := range rl.buckets {
			// Remove buckets that haven't been used recently AND are full
			if now.Sub(bucket.lastUpdate) > staleThreshold {
				delete(rl.buckets, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// =============================================================================
// GIN MIDDLEWARE
// =============================================================================

// RateLimitMiddleware creates a Gin middleware that applies rate limiting
// Returns 429 Too Many Requests if rate limit exceeded
func RateLimitMiddleware(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get client IP (handles proxies via X-Forwarded-For)
		ip := c.ClientIP()

		if !limiter.Allow(ip) {
			remaining := limiter.GetRemainingTokens(ip)
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("Retry-After", "1") // Suggest retry after 1 second

			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":     "rate limit exceeded",
				"message":   "too many requests, please slow down",
				"remaining": remaining,
			})
			return
		}

		// Add rate limit headers to response
		remaining := limiter.GetRemainingTokens(ip)
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))

		c.Next()
	}
}

// =============================================================================
// GLOBAL RATE LIMITER
// =============================================================================

// Default rate limiter: 10 requests/second with burst of 20
// This is generous for normal use but prevents abuse
var defaultRateLimiter = NewRateLimiter(10, 20)
