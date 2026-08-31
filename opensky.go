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
func fetchFlights(icao24 string) ([]Flight, int64, error) {
	url := "https://opensky-network.org/api/states/all"
	if icao24 != "" {
		url = fmt.Sprintf("%s?icao24=%s", url, icao24)
	}
	return doFetch(url)
}

// fetchFlightsByArea calls OpenSky with bounding box parameters
func fetchFlightsByArea(bbox BoundingBox) ([]Flight, int64, error) {
	url := fmt.Sprintf(
		"https://opensky-network.org/api/states/all?lamin=%f&lamax=%f&lomin=%f&lomax=%f",
		bbox.LatMin, bbox.LatMax, bbox.LonMin, bbox.LonMax,
	)
	return doFetch(url)
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
func fetchArrivals(airport string, begin, end int64) ([]HistoricalFlight, error) {
	url := fmt.Sprintf(
		"https://opensky-network.org/api/flights/arrival?airport=%s&begin=%d&end=%d",
		airport, begin, end,
	)
	return doFetchHistorical(url)
}

// fetchDepartures gets flights that departed from a specific airport
func fetchDepartures(airport string, begin, end int64) ([]HistoricalFlight, error) {
	url := fmt.Sprintf(
		"https://opensky-network.org/api/flights/departure?airport=%s&begin=%d&end=%d",
		airport, begin, end,
	)
	return doFetchHistorical(url)
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
