package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// =============================================================================
// OPENSKY API CLIENT
// =============================================================================

// OpenSky OAuth2 credentials - loaded from environment variables in main()
var (
	openSkyClientID     string
	openSkyClientSecret string
	openSkyAccessToken  string // Cached access token
)

// openSkyBaseURL is the base URL for OpenSky API
// This can be overridden in tests to point to a mock server
var openSkyBaseURL = "https://opensky-network.org"

// fetchAccessToken exchanges client credentials for an access token
// This implements OAuth2 Client Credentials flow
func fetchAccessToken() error {
	if openSkyClientID == "" || openSkyClientSecret == "" {
		return fmt.Errorf("OpenSky credentials not configured. Set OPENSKY_CLIENT_ID and OPENSKY_CLIENT_SECRET")
	}

	// Build form data for token request
	data := fmt.Sprintf("grant_type=client_credentials&client_id=%s&client_secret=%s",
		openSkyClientID, openSkyClientSecret)

	req, err := http.NewRequest("POST",
		"https://auth.opensky-network.org/auth/realms/opensky-network/protocol/openid-connect/token",
		strings.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token request failed with status %d", resp.StatusCode)
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("failed to decode token response: %w", err)
	}

	openSkyAccessToken = tokenResp.AccessToken
	fmt.Println("✅ OpenSky access token acquired")
	return nil
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
func doFetch(url string) ([]Flight, int64, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch from OpenSky: %w", err)
	}
	defer resp.Body.Close()

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

// fetchArrivals gets flights that arrived at a specific airport
// Uses cache since historical data doesn't change
func fetchArrivals(airport string, begin, end int64) ([]HistoricalFlight, error) {
	// Build cache key
	cacheKey := fmt.Sprintf("arrivals:%s:%d:%d", airport, begin, end)

	// Check cache first
	if cached, found := historicalCache.Get(cacheKey); found {
		return cached.([]HistoricalFlight), nil
	}

	// Cache miss - fetch from API
	url := fmt.Sprintf(
		"%s/api/flights/arrival?airport=%s&begin=%d&end=%d",
		openSkyBaseURL, airport, begin, end,
	)

	flights, err := doFetchHistorical(url)
	if err != nil {
		return nil, err
	}

	// Store in cache
	historicalCache.Set(cacheKey, flights, HistoricalCacheTTL)

	return flights, nil
}

// fetchDepartures gets flights that departed from a specific airport
// Uses cache since historical data doesn't change
func fetchDepartures(airport string, begin, end int64) ([]HistoricalFlight, error) {
	// Build cache key
	cacheKey := fmt.Sprintf("departures:%s:%d:%d", airport, begin, end)

	// Check cache first
	if cached, found := historicalCache.Get(cacheKey); found {
		return cached.([]HistoricalFlight), nil
	}

	// Cache miss - fetch from API
	url := fmt.Sprintf(
		"%s/api/flights/departure?airport=%s&begin=%d&end=%d",
		openSkyBaseURL, airport, begin, end,
	)

	flights, err := doFetchHistorical(url)
	if err != nil {
		return nil, err
	}

	// Store in cache
	historicalCache.Set(cacheKey, flights, HistoricalCacheTTL)

	return flights, nil
}

// doFetchHistorical handles authenticated requests to historical endpoints
func doFetchHistorical(url string) ([]HistoricalFlight, error) {
	if openSkyAccessToken == "" {
		return nil, fmt.Errorf("OpenSky access token not available. Check your CLIENT_ID and CLIENT_SECRET")
	}

	fmt.Printf("DEBUG: Fetching %s\n", url)

	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+openSkyAccessToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from OpenSky: %w", err)
	}
	defer resp.Body.Close()

	fmt.Printf("DEBUG: Response status: %d\n", resp.StatusCode)

	// Handle auth errors - refresh token and retry
	if resp.StatusCode == http.StatusUnauthorized {
		if err := fetchAccessToken(); err != nil {
			return nil, fmt.Errorf("authentication failed and token refresh failed: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+openSkyAccessToken)
		resp, err = client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("retry failed: %w", err)
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no flights found for this airport/time range (OpenSky returned 404)")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenSky returned status %d", resp.StatusCode)
	}

	var flights []HistoricalFlight
	if err := json.NewDecoder(resp.Body).Decode(&flights); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return flights, nil
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
