package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// =============================================================================
// OPENSKY API CLIENT
// =============================================================================

// openSkyBaseURL is the base URL for OpenSky API
// Can be overridden via OPENSKY_BASE_URL env var.
// Recommended production primary: https://opensky-network.org
var openSkyBaseURL = "https://opensky-network.org"

// openSkyFallbackBaseURL is an optional secondary upstream used when the primary fails.
// Example: primary=https://opensky-network.org, fallback=Cloudflare Worker URL
var openSkyFallbackBaseURL string

// openSkyClient is a shared HTTP client for OpenSky requests
var openSkyClient *http.Client

// openSkyAPIKey is an optional key for authenticated proxy requests
var openSkyAPIKey string

// openSkyRequestTimeout controls upstream HTTP timeout per attempt.
var openSkyRequestTimeout = 12 * time.Second

// openSkyMaxAttempts controls retries per upstream.
var openSkyMaxAttempts = 2

var openSkyFailoverDiagnostics = newFailoverDiagnostics()
var failoverRequestIDCounter uint64

func init() {
	// Allow overriding the primary base URL.
	if baseURL := os.Getenv("OPENSKY_BASE_URL"); baseURL != "" {
		openSkyBaseURL = strings.TrimRight(baseURL, "/")
		fmt.Printf("✅ OpenSky base URL: %s\n", openSkyBaseURL)
	}

	if fallbackBaseURL := os.Getenv("OPENSKY_FALLBACK_BASE_URL"); fallbackBaseURL != "" {
		openSkyFallbackBaseURL = strings.TrimRight(fallbackBaseURL, "/")
		fmt.Printf("✅ OpenSky fallback base URL: %s\n", openSkyFallbackBaseURL)
	}

	// If using a Cloudflare Worker and no explicit fallback is set,
	// automatically use direct OpenSky as fallback.
	if openSkyFallbackBaseURL == "" && strings.Contains(openSkyBaseURL, ".workers.dev") {
		openSkyFallbackBaseURL = "https://opensky-network.org"
		fmt.Printf("✅ OpenSky fallback base URL (auto): %s\n", openSkyFallbackBaseURL)
	}

	// Optional API key for proxy authentication
	openSkyAPIKey = os.Getenv("OPENSKY_API_KEY")

	if timeoutSeconds := os.Getenv("OPENSKY_TIMEOUT_SECONDS"); timeoutSeconds != "" {
		if seconds, err := strconv.Atoi(timeoutSeconds); err == nil && seconds >= 3 && seconds <= 60 {
			openSkyRequestTimeout = time.Duration(seconds) * time.Second
		}
	}

	if attemptsValue := os.Getenv("OPENSKY_MAX_ATTEMPTS"); attemptsValue != "" {
		if attempts, err := strconv.Atoi(attemptsValue); err == nil && attempts >= 1 && attempts <= 5 {
			openSkyMaxAttempts = attempts
		}
	}

	openSkyClient = &http.Client{
		Timeout: openSkyRequestTimeout,
	}
}

// fetchFlights calls the OpenSky Network API and returns parsed flights
// Uses cache to avoid hitting the API too frequently
func fetchFlights(icao24 string) ([]Flight, int64, error) {
	// Build cache key
	cacheKey := "flights:all"
	if icao24 != "" {
		cacheKey = fmt.Sprintf("flights:icao24:%s", icao24)
	}

	// Check cache first
	if cached, found := flightCache.Get(cacheKey); found {
		result := cached.(cachedFlightResult)
		return result.Flights, result.Timestamp, nil
	}

	// Cache miss - fetch from API
	url := openSkyBaseURL + "/api/states/all"
	if icao24 != "" {
		url = fmt.Sprintf("%s?icao24=%s", url, icao24)
	}

	flights, timestamp, err := doFetch(url)
	if err != nil && openSkyFallbackBaseURL != "" {
		fallbackURL := openSkyFallbackBaseURL + "/api/states/all"
		if icao24 != "" {
			fallbackURL = fmt.Sprintf("%s?icao24=%s", fallbackURL, icao24)
		}

		requestID := newFailoverRequestID()

		if fallbackURL != url {
			reason := classifyFailoverReason(err)
			if shouldFailover(err) {
				openSkyFailoverDiagnostics.recordAttempt(requestID, reason, url, fallbackURL, err)
				flights, timestamp, fallbackErr := doFetchWithAttempts(fallbackURL, 1)
				if fallbackErr == nil {
					openSkyFailoverDiagnostics.recordSuccess(requestID, reason, url, fallbackURL)
					flightCache.Set(cacheKey, cachedFlightResult{Flights: flights, Timestamp: timestamp}, FlightCacheTTL)
					return flights, timestamp, nil
				}
				openSkyFailoverDiagnostics.recordFailure(requestID, reason, url, fallbackURL, fallbackErr)
				return nil, 0, failoverError{
					RequestID:   requestID,
					Reason:      reason,
					PrimaryURL:  url,
					FallbackURL: fallbackURL,
					PrimaryErr:  err,
					FallbackErr: fallbackErr,
				}
			}
			openSkyFailoverDiagnostics.recordSkipped(requestID, "non_retryable_primary_error", url, fallbackURL, err)
		} else {
			openSkyFailoverDiagnostics.recordSkipped(requestID, "fallback_same_as_primary", url, fallbackURL, err)
		}
	}
	if err != nil {
		return nil, 0, err
	}

	// Store in cache
	flightCache.Set(cacheKey, cachedFlightResult{Flights: flights, Timestamp: timestamp}, FlightCacheTTL)

	return flights, timestamp, nil
}

// fetchFlightsByArea calls OpenSky with bounding box parameters
// Uses cache to avoid hitting the API too frequently
func fetchFlightsByArea(bbox BoundingBox) ([]Flight, int64, error) {
	// Build cache key from bounding box (rounded to reduce key variations)
	cacheKey := fmt.Sprintf("flights:area:%.2f:%.2f:%.2f:%.2f",
		bbox.LatMin, bbox.LatMax, bbox.LonMin, bbox.LonMax)

	// Check cache first
	if cached, found := flightCache.Get(cacheKey); found {
		result := cached.(cachedFlightResult)
		return result.Flights, result.Timestamp, nil
	}

	// Cache miss - fetch from API
	url := fmt.Sprintf(
		"%s/api/states/all?lamin=%f&lamax=%f&lomin=%f&lomax=%f",
		openSkyBaseURL, bbox.LatMin, bbox.LatMax, bbox.LonMin, bbox.LonMax,
	)

	flights, timestamp, err := doFetch(url)
	if err != nil && openSkyFallbackBaseURL != "" {
		fallbackURL := fmt.Sprintf(
			"%s/api/states/all?lamin=%f&lamax=%f&lomin=%f&lomax=%f",
			openSkyFallbackBaseURL, bbox.LatMin, bbox.LatMax, bbox.LonMin, bbox.LonMax,
		)

		requestID := newFailoverRequestID()

		if fallbackURL != url {
			reason := classifyFailoverReason(err)
			if shouldFailover(err) {
				openSkyFailoverDiagnostics.recordAttempt(requestID, reason, url, fallbackURL, err)
				flights, timestamp, fallbackErr := doFetchWithAttempts(fallbackURL, 1)
				if fallbackErr == nil {
					openSkyFailoverDiagnostics.recordSuccess(requestID, reason, url, fallbackURL)
					flightCache.Set(cacheKey, cachedFlightResult{Flights: flights, Timestamp: timestamp}, FlightCacheTTL)
					return flights, timestamp, nil
				}
				openSkyFailoverDiagnostics.recordFailure(requestID, reason, url, fallbackURL, fallbackErr)
				return nil, 0, failoverError{
					RequestID:   requestID,
					Reason:      reason,
					PrimaryURL:  url,
					FallbackURL: fallbackURL,
					PrimaryErr:  err,
					FallbackErr: fallbackErr,
				}
			}
			openSkyFailoverDiagnostics.recordSkipped(requestID, "non_retryable_primary_error", url, fallbackURL, err)
		} else {
			openSkyFailoverDiagnostics.recordSkipped(requestID, "fallback_same_as_primary", url, fallbackURL, err)
		}
	}
	if err != nil {
		return nil, 0, err
	}

	// Store in cache
	flightCache.Set(cacheKey, cachedFlightResult{Flights: flights, Timestamp: timestamp}, FlightCacheTTL)

	return flights, timestamp, nil
}

// cachedFlightResult holds flight data for caching
type cachedFlightResult struct {
	Flights   []Flight
	Timestamp int64
}

// doFetch handles the HTTP request and parsing for real-time endpoints
func doFetch(targetURL string) ([]Flight, int64, error) {
	return doFetchWithAttempts(targetURL, openSkyMaxAttempts)
}

func doFetchWithAttempts(targetURL string, maxAttempts int) ([]Flight, int64, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequest("GET", targetURL, nil)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create request: %w", err)
		}

		// Add proxy auth key if configured
		if openSkyAPIKey != "" {
			req.Header.Set("X-Proxy-Key", openSkyAPIKey)
		}

		resp, err := openSkyClient.Do(req)
		if err != nil {
			if attempt < maxAttempts {
				time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
				continue
			}
			return nil, 0, fmt.Errorf("failed to fetch from OpenSky: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			status := resp.StatusCode
			_ = resp.Body.Close()

			if shouldRetryStatus(status) && attempt < maxAttempts {
				time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
				continue
			}

			return nil, 0, upstreamStatusError{Status: status}
		}

		var openSkyResp OpenSkyResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&openSkyResp)
		_ = resp.Body.Close()
		if decodeErr != nil {
			return nil, 0, fmt.Errorf("failed to decode response: %w", decodeErr)
		}

		flights := parseStates(openSkyResp.States)
		return flights, openSkyResp.Time, nil
	}

	return nil, 0, fmt.Errorf("failed to fetch from OpenSky after retries")
}

type upstreamStatusError struct {
	Status int
}

type failoverError struct {
	RequestID   string
	Reason      string
	PrimaryURL  string
	FallbackURL string
	PrimaryErr  error
	FallbackErr error
}

func (e failoverError) Error() string {
	return fmt.Sprintf(
		"primary upstream failed (%s) [request_id=%s]: %v; fallback failed: %v",
		e.Reason,
		e.RequestID,
		e.PrimaryErr,
		e.FallbackErr,
	)
}

func (e upstreamStatusError) Error() string {
	return fmt.Sprintf("OpenSky returned status %d", e.Status)
}

func shouldFailover(err error) bool {
	var statusErr upstreamStatusError
	if errors.As(err, &statusErr) {
		return shouldRetryStatus(statusErr.Status)
	}

	// Network-level failures are also good failover candidates.
	return true
}

func classifyFailoverReason(err error) string {
	var statusErr upstreamStatusError
	if errors.As(err, &statusErr) {
		return fmt.Sprintf("upstream_status_%d", statusErr.Status)
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline_exceeded"
	}

	if errors.Is(err, context.Canceled) {
		return "request_canceled"
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return "network_timeout"
		}
		return "network_error"
	}

	return "request_error"
}

func shouldRetryStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout, 522, 523, 524:
		return true
	default:
		return false
	}
}

type openSkyFailoverStats struct {
	PrimaryFailures   uint64 `json:"primaryFailures"`
	FailoverAttempts  uint64 `json:"failoverAttempts"`
	FailoverSuccess   uint64 `json:"failoverSuccess"`
	FailoverFailures  uint64 `json:"failoverFailures"`
	FailoverSkipped   uint64 `json:"failoverSkipped"`
	LastRequestID     string `json:"lastRequestId,omitempty"`
	LastReason        string `json:"lastReason,omitempty"`
	LastPrimaryURL    string `json:"lastPrimaryUrl,omitempty"`
	LastFallbackURL   string `json:"lastFallbackUrl,omitempty"`
	LastPrimaryError  string `json:"lastPrimaryError,omitempty"`
	LastFallbackError string `json:"lastFallbackError,omitempty"`
	LastUpdatedUTC    string `json:"lastUpdatedUtc,omitempty"`
}

type failoverDiagnostics struct {
	mu    sync.Mutex
	stats openSkyFailoverStats
}

func newFailoverDiagnostics() *failoverDiagnostics {
	return &failoverDiagnostics{}
}

func newFailoverRequestID() string {
	seq := atomic.AddUint64(&failoverRequestIDCounter, 1)
	return fmt.Sprintf("fo-%d-%d", time.Now().UTC().UnixMilli(), seq)
}

func (d *failoverDiagnostics) recordAttempt(requestID, reason, primaryURL, fallbackURL string, primaryErr error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stats.PrimaryFailures++
	d.stats.FailoverAttempts++
	d.stats.LastRequestID = requestID
	d.stats.LastReason = reason
	d.stats.LastPrimaryURL = primaryURL
	d.stats.LastFallbackURL = fallbackURL
	d.stats.LastPrimaryError = primaryErr.Error()
	d.stats.LastFallbackError = ""
	d.stats.LastUpdatedUTC = time.Now().UTC().Format(time.RFC3339)
}

func (d *failoverDiagnostics) recordSuccess(requestID, reason, primaryURL, fallbackURL string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stats.FailoverSuccess++
	d.stats.LastRequestID = requestID
	d.stats.LastReason = reason
	d.stats.LastPrimaryURL = primaryURL
	d.stats.LastFallbackURL = fallbackURL
	d.stats.LastFallbackError = ""
	d.stats.LastUpdatedUTC = time.Now().UTC().Format(time.RFC3339)
}

func (d *failoverDiagnostics) recordFailure(requestID, reason, primaryURL, fallbackURL string, fallbackErr error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stats.FailoverFailures++
	d.stats.LastRequestID = requestID
	d.stats.LastReason = reason
	d.stats.LastPrimaryURL = primaryURL
	d.stats.LastFallbackURL = fallbackURL
	d.stats.LastFallbackError = fallbackErr.Error()
	d.stats.LastUpdatedUTC = time.Now().UTC().Format(time.RFC3339)
}

func (d *failoverDiagnostics) recordSkipped(requestID, reason, primaryURL, fallbackURL string, primaryErr error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stats.PrimaryFailures++
	d.stats.FailoverSkipped++
	d.stats.LastRequestID = requestID
	d.stats.LastReason = reason
	d.stats.LastPrimaryURL = primaryURL
	d.stats.LastFallbackURL = fallbackURL
	d.stats.LastPrimaryError = primaryErr.Error()
	d.stats.LastFallbackError = ""
	d.stats.LastUpdatedUTC = time.Now().UTC().Format(time.RFC3339)
}

func (d *failoverDiagnostics) snapshot() openSkyFailoverStats {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.stats
}

func (d *failoverDiagnostics) reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stats = openSkyFailoverStats{}
}

func getOpenSkyFailoverStats() openSkyFailoverStats {
	return openSkyFailoverDiagnostics.snapshot()
}

func resetOpenSkyFailoverStats() {
	openSkyFailoverDiagnostics.reset()
}

// parseStates converts OpenSky's mixed arrays into typed Flight structs
func parseStates(states [][]interface{}) []Flight {
	flights := make([]Flight, 0, len(states))

	for _, state := range states {
		if len(state) < 17 {
			continue
		}

		flight := Flight{
			Icao24:        safeString(state[0]),
			Callsign:      safeStringPtr(state[1]),
			OriginCountry: safeString(state[2]),
			LastContact:   safeInt64(state[4]),
			Longitude:     safeFloat64Ptr(state[5]),
			Latitude:      safeFloat64Ptr(state[6]),
			Altitude:      safeFloat64Ptr(state[7]),
			OnGround:      safeBool(state[8]),
			Velocity:      safeFloat64Ptr(state[9]),
		}

		flights = append(flights, flight)
	}

	return flights
}
