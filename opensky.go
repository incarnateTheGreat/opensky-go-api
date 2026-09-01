package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// =============================================================================
// OPENSKY API CLIENT
// =============================================================================

// openSkyBaseURL is the base URL for OpenSky API
// Can be overridden via OPENSKY_BASE_URL env var (e.g., for Cloudflare Worker proxy)
var openSkyBaseURL = "https://opensky-network.org"

// openSkyClient is a shared HTTP client for OpenSky requests
var openSkyClient *http.Client

// openSkyAPIKey is an optional key for authenticated proxy requests
var openSkyAPIKey string

func init() {
	// Allow overriding the base URL (for Cloudflare Worker proxy)
	if baseURL := os.Getenv("OPENSKY_BASE_URL"); baseURL != "" {
		openSkyBaseURL = baseURL
		fmt.Printf("✅ OpenSky base URL: %s\n", baseURL)
	}

	// Optional API key for proxy authentication
	openSkyAPIKey = os.Getenv("OPENSKY_API_KEY")

	openSkyClient = &http.Client{
		Timeout: 30 * time.Second,
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
		return nil, 0, fmt.Errorf("failed to fetch from OpenSky: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("OpenSky returned status %d", resp.StatusCode)
	}

	var openSkyResp OpenSkyResponse
	if err := json.NewDecoder(resp.Body).Decode(&openSkyResp); err != nil {
		return nil, 0, fmt.Errorf("failed to decode response: %w", err)
	}

	flights := parseStates(openSkyResp.States)
	return flights, openSkyResp.Time, nil
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
