package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	neturl "net/url"
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

// openSkyClientID and openSkyClientSecret are optional OpenSky credentials
// used for direct OpenSky upstream calls.
var openSkyClientID string
var openSkyClientSecret string

// openSkyRequestTimeout controls upstream HTTP timeout per attempt.
var openSkyRequestTimeout = 12 * time.Second

// openSkyTotalRequestTimeout caps total time spent across primary + fallback.
var openSkyTotalRequestTimeout = 14 * time.Second

// openSkyMaxAttempts controls retries per upstream.
var openSkyMaxAttempts = 2

// openSkyStaleMaxAge controls how long expired cache entries can be served
// when both upstreams fail.
var openSkyStaleMaxAge = 5 * time.Minute

var openSkyFailoverDiagnostics = newFailoverDiagnostics()
var failoverRequestIDCounter uint64

const defaultUpstreamProbePath = "/api/states/all?lamin=37.90&lamax=38.00&lomin=23.80&lomax=23.90"

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
	openSkyClientID = os.Getenv("OPENSKY_CLIENT_ID")
	openSkyClientSecret = os.Getenv("OPENSKY_CLIENT_SECRET")

	if timeoutSeconds := os.Getenv("OPENSKY_TIMEOUT_SECONDS"); timeoutSeconds != "" {
		if seconds, err := strconv.Atoi(timeoutSeconds); err == nil && seconds >= 3 && seconds <= 60 {
			openSkyRequestTimeout = time.Duration(seconds) * time.Second
		}
	}

	if totalTimeoutSeconds := os.Getenv("OPENSKY_TOTAL_TIMEOUT_SECONDS"); totalTimeoutSeconds != "" {
		if seconds, err := strconv.Atoi(totalTimeoutSeconds); err == nil && seconds >= 3 && seconds <= 120 {
			openSkyTotalRequestTimeout = time.Duration(seconds) * time.Second
		}
	} else {
		// By default, reserve only a small cushion above per-attempt timeout
		// to avoid doubling latency on sequential failover.
		openSkyTotalRequestTimeout = openSkyRequestTimeout + 2*time.Second
	}

	if attemptsValue := os.Getenv("OPENSKY_MAX_ATTEMPTS"); attemptsValue != "" {
		if attempts, err := strconv.Atoi(attemptsValue); err == nil && attempts >= 1 && attempts <= 5 {
			openSkyMaxAttempts = attempts
		}
	}

	if staleMaxAgeSeconds := os.Getenv("OPENSKY_STALE_MAX_AGE_SECONDS"); staleMaxAgeSeconds != "" {
		if seconds, err := strconv.Atoi(staleMaxAgeSeconds); err == nil && seconds >= 0 && seconds <= 3600 {
			openSkyStaleMaxAge = time.Duration(seconds) * time.Second
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

	totalStarted := time.Now()
	primaryStarted := time.Now()
	flights, timestamp, err := doFetch(url)
	primaryDuration := time.Since(primaryStarted)
	if err != nil && openSkyFallbackBaseURL != "" {
		fallbackURL := openSkyFallbackBaseURL + "/api/states/all"
		if icao24 != "" {
			fallbackURL = fmt.Sprintf("%s?icao24=%s", fallbackURL, icao24)
		}

		requestID := newFailoverRequestID()

		if fallbackURL != url {
			reason := classifyFailoverReason(err)
			if shouldFailover(err) {
				openSkyFailoverDiagnostics.recordAttempt(requestID, reason, url, fallbackURL, primaryDuration, err)
				remainingBudget := openSkyTotalRequestTimeout - time.Since(totalStarted)
				if remainingBudget <= 0 {
					openSkyFailoverDiagnostics.recordSkipped(requestID, "no_remaining_budget", url, fallbackURL, primaryDuration, err)
					return nil, 0, err
				}

				fallbackTimeout := remainingBudget
				if fallbackTimeout > openSkyRequestTimeout {
					fallbackTimeout = openSkyRequestTimeout
				}

				fallbackStarted := time.Now()
				flights, timestamp, fallbackErr := doFetchWithAttemptsAndTimeout(fallbackURL, 1, fallbackTimeout)
				fallbackDuration := time.Since(fallbackStarted)
				if fallbackErr == nil {
					openSkyFailoverDiagnostics.recordSuccess(requestID, reason, url, fallbackURL, primaryDuration, fallbackDuration)
					flightCache.Set(cacheKey, cachedFlightResult{Flights: flights, Timestamp: timestamp}, FlightCacheTTL)
					return flights, timestamp, nil
				}

				if staleValue, found, staleAge := flightCache.GetStale(cacheKey); found && staleAge > 0 && staleAge <= openSkyStaleMaxAge {
					if staleResult, ok := staleValue.(cachedFlightResult); ok {
						openSkyFailoverDiagnostics.recordServedStale(requestID, reason, url, fallbackURL, primaryDuration, fallbackDuration, staleAge, err, fallbackErr)
						return staleResult.Flights, staleResult.Timestamp, nil
					}
				}

				openSkyFailoverDiagnostics.recordFailure(requestID, reason, url, fallbackURL, primaryDuration, fallbackDuration, fallbackErr)
				return nil, 0, failoverError{
					RequestID:   requestID,
					Reason:      reason,
					PrimaryURL:  url,
					FallbackURL: fallbackURL,
					PrimaryErr:  err,
					FallbackErr: fallbackErr,
				}
			}
			openSkyFailoverDiagnostics.recordSkipped(requestID, "non_retryable_primary_error", url, fallbackURL, primaryDuration, err)
		} else {
			openSkyFailoverDiagnostics.recordSkipped(requestID, "fallback_same_as_primary", url, fallbackURL, primaryDuration, err)
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

	totalStarted := time.Now()
	primaryStarted := time.Now()
	flights, timestamp, err := doFetch(url)
	primaryDuration := time.Since(primaryStarted)
	if err != nil && openSkyFallbackBaseURL != "" {
		fallbackURL := fmt.Sprintf(
			"%s/api/states/all?lamin=%f&lamax=%f&lomin=%f&lomax=%f",
			openSkyFallbackBaseURL, bbox.LatMin, bbox.LatMax, bbox.LonMin, bbox.LonMax,
		)

		requestID := newFailoverRequestID()

		if fallbackURL != url {
			reason := classifyFailoverReason(err)
			if shouldFailover(err) {
				openSkyFailoverDiagnostics.recordAttempt(requestID, reason, url, fallbackURL, primaryDuration, err)
				remainingBudget := openSkyTotalRequestTimeout - time.Since(totalStarted)
				if remainingBudget <= 0 {
					openSkyFailoverDiagnostics.recordSkipped(requestID, "no_remaining_budget", url, fallbackURL, primaryDuration, err)
					return nil, 0, err
				}

				fallbackTimeout := remainingBudget
				if fallbackTimeout > openSkyRequestTimeout {
					fallbackTimeout = openSkyRequestTimeout
				}

				fallbackStarted := time.Now()
				flights, timestamp, fallbackErr := doFetchWithAttemptsAndTimeout(fallbackURL, 1, fallbackTimeout)
				fallbackDuration := time.Since(fallbackStarted)
				if fallbackErr == nil {
					openSkyFailoverDiagnostics.recordSuccess(requestID, reason, url, fallbackURL, primaryDuration, fallbackDuration)
					flightCache.Set(cacheKey, cachedFlightResult{Flights: flights, Timestamp: timestamp}, FlightCacheTTL)
					return flights, timestamp, nil
				}

				if staleValue, found, staleAge := flightCache.GetStale(cacheKey); found && staleAge > 0 && staleAge <= openSkyStaleMaxAge {
					if staleResult, ok := staleValue.(cachedFlightResult); ok {
						openSkyFailoverDiagnostics.recordServedStale(requestID, reason, url, fallbackURL, primaryDuration, fallbackDuration, staleAge, err, fallbackErr)
						return staleResult.Flights, staleResult.Timestamp, nil
					}
				}

				openSkyFailoverDiagnostics.recordFailure(requestID, reason, url, fallbackURL, primaryDuration, fallbackDuration, fallbackErr)
				return nil, 0, failoverError{
					RequestID:   requestID,
					Reason:      reason,
					PrimaryURL:  url,
					FallbackURL: fallbackURL,
					PrimaryErr:  err,
					FallbackErr: fallbackErr,
				}
			}
			openSkyFailoverDiagnostics.recordSkipped(requestID, "non_retryable_primary_error", url, fallbackURL, primaryDuration, err)
		} else {
			openSkyFailoverDiagnostics.recordSkipped(requestID, "fallback_same_as_primary", url, fallbackURL, primaryDuration, err)
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
	return doFetchWithAttemptsWithClient(targetURL, maxAttempts, openSkyClient)
}

func doFetchWithAttemptsAndTimeout(targetURL string, maxAttempts int, timeout time.Duration) ([]Flight, int64, error) {
	if timeout < 1*time.Second {
		timeout = 1 * time.Second
	}

	client := &http.Client{Timeout: timeout}
	return doFetchWithAttemptsWithClient(targetURL, maxAttempts, client)
}

func doFetchWithAttemptsWithClient(targetURL string, maxAttempts int, client *http.Client) ([]Flight, int64, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequest("GET", targetURL, nil)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create request: %w", err)
		}

		// Add proxy auth key if configured
		applyUpstreamAuth(req, targetURL)

		resp, err := client.Do(req)
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
	ServedStale       uint64 `json:"servedStale"`
	LastRequestID     string `json:"lastRequestId,omitempty"`
	LastReason        string `json:"lastReason,omitempty"`
	LastPrimaryURL    string `json:"lastPrimaryUrl,omitempty"`
	LastFallbackURL   string `json:"lastFallbackUrl,omitempty"`
	LastPrimaryError  string `json:"lastPrimaryError,omitempty"`
	LastFallbackError string `json:"lastFallbackError,omitempty"`
	LastPrimaryMS     int64  `json:"lastPrimaryMs,omitempty"`
	LastFallbackMS    int64  `json:"lastFallbackMs,omitempty"`
	LastStaleAgeMS    int64  `json:"lastStaleAgeMs,omitempty"`
	LastServedStale   bool   `json:"lastServedStale"`
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

func (d *failoverDiagnostics) recordAttempt(requestID, reason, primaryURL, fallbackURL string, primaryDuration time.Duration, primaryErr error) {
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
	d.stats.LastPrimaryMS = primaryDuration.Milliseconds()
	d.stats.LastFallbackMS = 0
	d.stats.LastStaleAgeMS = 0
	d.stats.LastServedStale = false
	d.stats.LastUpdatedUTC = time.Now().UTC().Format(time.RFC3339)
}

func (d *failoverDiagnostics) recordSuccess(requestID, reason, primaryURL, fallbackURL string, primaryDuration, fallbackDuration time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stats.FailoverSuccess++
	d.stats.LastRequestID = requestID
	d.stats.LastReason = reason
	d.stats.LastPrimaryURL = primaryURL
	d.stats.LastFallbackURL = fallbackURL
	d.stats.LastPrimaryError = ""
	d.stats.LastFallbackError = ""
	d.stats.LastPrimaryMS = primaryDuration.Milliseconds()
	d.stats.LastFallbackMS = fallbackDuration.Milliseconds()
	d.stats.LastStaleAgeMS = 0
	d.stats.LastServedStale = false
	d.stats.LastUpdatedUTC = time.Now().UTC().Format(time.RFC3339)
}

func (d *failoverDiagnostics) recordFailure(requestID, reason, primaryURL, fallbackURL string, primaryDuration, fallbackDuration time.Duration, fallbackErr error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stats.FailoverFailures++
	d.stats.LastRequestID = requestID
	d.stats.LastReason = reason
	d.stats.LastPrimaryURL = primaryURL
	d.stats.LastFallbackURL = fallbackURL
	d.stats.LastFallbackError = fallbackErr.Error()
	d.stats.LastPrimaryMS = primaryDuration.Milliseconds()
	d.stats.LastFallbackMS = fallbackDuration.Milliseconds()
	d.stats.LastStaleAgeMS = 0
	d.stats.LastServedStale = false
	d.stats.LastUpdatedUTC = time.Now().UTC().Format(time.RFC3339)
}

func (d *failoverDiagnostics) recordSkipped(requestID, reason, primaryURL, fallbackURL string, primaryDuration time.Duration, primaryErr error) {
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
	d.stats.LastPrimaryMS = primaryDuration.Milliseconds()
	d.stats.LastFallbackMS = 0
	d.stats.LastStaleAgeMS = 0
	d.stats.LastServedStale = false
	d.stats.LastUpdatedUTC = time.Now().UTC().Format(time.RFC3339)
}

func (d *failoverDiagnostics) recordServedStale(requestID, reason, primaryURL, fallbackURL string, primaryDuration, fallbackDuration, staleAge time.Duration, primaryErr, fallbackErr error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stats.FailoverFailures++
	d.stats.ServedStale++
	d.stats.LastRequestID = requestID
	d.stats.LastReason = reason
	d.stats.LastPrimaryURL = primaryURL
	d.stats.LastFallbackURL = fallbackURL
	d.stats.LastPrimaryError = primaryErr.Error()
	d.stats.LastFallbackError = fallbackErr.Error()
	d.stats.LastPrimaryMS = primaryDuration.Milliseconds()
	d.stats.LastFallbackMS = fallbackDuration.Milliseconds()
	d.stats.LastStaleAgeMS = staleAge.Milliseconds()
	d.stats.LastServedStale = true
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

type upstreamProbeResult struct {
	Name       string `json:"name"`
	URL        string `json:"url,omitempty"`
	Configured bool   `json:"configured"`
	AuthMode   string `json:"authMode"`
	Success    bool   `json:"success"`
	StatusCode int    `json:"statusCode,omitempty"`
	DurationMS int64  `json:"durationMs"`
	Error      string `json:"error,omitempty"`
}

func getUpstreamProbePath() string {
	if probePath := strings.TrimSpace(os.Getenv("OPENSKY_PROBE_PATH")); probePath != "" {
		if strings.HasPrefix(probePath, "/") {
			return probePath
		}
		return "/" + probePath
	}

	return defaultUpstreamProbePath
}

func getUpstreamProbeTimeout() time.Duration {
	if timeoutSeconds := os.Getenv("OPENSKY_PROBE_TIMEOUT_SECONDS"); timeoutSeconds != "" {
		if seconds, err := strconv.Atoi(timeoutSeconds); err == nil && seconds >= 1 && seconds <= 30 {
			return time.Duration(seconds) * time.Second
		}
	}

	if openSkyRequestTimeout < 5*time.Second {
		return openSkyRequestTimeout
	}

	return 5 * time.Second
}

func probeUpstream(name, baseURL, probePath string, timeout time.Duration) upstreamProbeResult {
	result := upstreamProbeResult{
		Name:       name,
		Configured: strings.TrimSpace(baseURL) != "",
		AuthMode:   "none",
	}

	if !result.Configured {
		result.Error = "not configured"
		return result
	}

	url := strings.TrimRight(baseURL, "/") + probePath
	result.URL = url
	result.AuthMode = authModeForTarget(url)

	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		result.Error = fmt.Sprintf("failed to create request: %v", err)
		return result
	}

	applyUpstreamAuth(req, url)

	started := time.Now()
	resp, err := client.Do(req)
	result.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	result.StatusCode = resp.StatusCode
	result.Success = resp.StatusCode == http.StatusOK
	if !result.Success {
		result.Error = fmt.Sprintf("status %d", resp.StatusCode)
	}

	return result
}

func probeOpenSkyUpstreams() map[string]interface{} {
	probePath := getUpstreamProbePath()
	timeout := getUpstreamProbeTimeout()
	primary := probeUpstream("primary", openSkyBaseURL, probePath, timeout)
	fallback := probeUpstream("fallback", openSkyFallbackBaseURL, probePath, timeout)

	return map[string]interface{}{
		"timeUtc":   time.Now().UTC().Format(time.RFC3339),
		"probePath": probePath,
		"timeoutMs": timeout.Milliseconds(),
		"primary":   primary,
		"fallback":  fallback,
	}
}

func applyUpstreamAuth(req *http.Request, targetURL string) {
	switch authModeForTarget(targetURL) {
	case "proxy_key":
		if openSkyAPIKey != "" {
			req.Header.Set("X-Proxy-Key", openSkyAPIKey)
		}
	case "basic_auth":
		req.SetBasicAuth(openSkyClientID, openSkyClientSecret)
	}
}

func authModeForTarget(targetURL string) string {
	if shouldUseProxyAuth(targetURL) {
		if openSkyAPIKey != "" {
			return "proxy_key"
		}
		return "none"
	}

	if openSkyClientID != "" && openSkyClientSecret != "" {
		return "basic_auth"
	}

	return "none"
}

func shouldUseProxyAuth(targetURL string) bool {
	parsed, err := neturl.Parse(targetURL)
	if err != nil {
		return false
	}

	host := strings.ToLower(parsed.Hostname())
	return strings.Contains(host, "workers.dev")
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
