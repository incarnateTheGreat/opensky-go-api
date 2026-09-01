package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// =============================================================================
// MOCK OPENSKY API TESTS - Testing with a mock HTTP server
// =============================================================================

// These tests demonstrate how to mock external APIs using httptest.Server
// Key concepts:
// 1. httptest.NewServer() creates a real HTTP server on a random port
// 2. The server runs in the same process, so we control the responses
// 3. We point our code at the mock server instead of the real API

// TestFetchFlights_MockServer tests the fetchFlights function with a mock server
func TestFetchFlights_MockServer(t *testing.T) {
	// Create a mock server that returns a valid OpenSky response
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request
		if r.URL.Path != "/api/states/all" {
			t.Errorf("expected path /api/states/all, got %s", r.URL.Path)
		}

		// Return a mock response
		response := OpenSkyResponse{
			Time: 1234567890,
			States: [][]interface{}{
				{
					"abc123",  // icao24
					"UAL123 ", // callsign (with trailing space, like real API)
					"United States",
					1234567880.0, // time_position
					1234567890.0, // last_contact
					-122.4,       // longitude
					37.8,         // latitude
					10000.0,      // baro_altitude
					false,        // on_ground
					250.0,        // velocity
					45.0,         // true_track
					0.0,          // vertical_rate
					nil,          // sensors
					10500.0,      // geo_altitude
					"1234",       // squawk
					false,        // spi
					0,            // position_source
					0,            // category
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	// Save original base URL and restore after test
	originalBaseURL := openSkyBaseURL
	defer func() { openSkyBaseURL = originalBaseURL }()

	// Point to mock server
	openSkyBaseURL = mockServer.URL

	// Clear cache to ensure we hit the mock server
	flightCache.Clear()

	// Call the function under test
	flights, timestamp, err := fetchFlights("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify results
	if timestamp != 1234567890 {
		t.Errorf("expected timestamp 1234567890, got %d", timestamp)
	}

	if len(flights) != 1 {
		t.Fatalf("expected 1 flight, got %d", len(flights))
	}

	flight := flights[0]
	if flight.Icao24 != "abc123" {
		t.Errorf("expected icao24 'abc123', got '%s'", flight.Icao24)
	}
	// Note: safeStringPtr doesn't trim spaces, so callsign retains trailing space
	if flight.Callsign == nil || *flight.Callsign != "UAL123 " {
		t.Errorf("expected callsign 'UAL123 ' (with trailing space), got '%v'", flight.Callsign)
	}
	if flight.OriginCountry != "United States" {
		t.Errorf("expected country 'United States', got '%s'", flight.OriginCountry)
	}
}

// TestFetchFlights_ByICAO24 tests fetching a specific aircraft
func TestFetchFlights_ByICAO24(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify query parameter
		icao24 := r.URL.Query().Get("icao24")
		if icao24 != "abc123" {
			t.Errorf("expected icao24 query param 'abc123', got '%s'", icao24)
		}

		response := OpenSkyResponse{
			Time:   1234567890,
			States: [][]interface{}{},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	originalBaseURL := openSkyBaseURL
	defer func() { openSkyBaseURL = originalBaseURL }()
	openSkyBaseURL = mockServer.URL
	flightCache.Clear()

	_, _, err := fetchFlights("abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestFetchFlightsByArea_MockServer tests bounding box queries
func TestFetchFlightsByArea_MockServer(t *testing.T) {
	requestReceived := false

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true

		// Verify bounding box params are present
		query := r.URL.Query()
		if query.Get("lamin") == "" {
			t.Error("missing lamin parameter")
		}
		if query.Get("lamax") == "" {
			t.Error("missing lamax parameter")
		}

		response := OpenSkyResponse{
			Time: 1234567890,
			States: [][]interface{}{
				{"xyz789", "DAL456 ", "United States", nil, 1234567890.0, -100.0, 40.0, 11000.0, false, 300.0, 90.0, 0.0, nil, 11500.0, "5555", false, 0, 0},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	originalBaseURL := openSkyBaseURL
	defer func() { openSkyBaseURL = originalBaseURL }()
	openSkyBaseURL = mockServer.URL
	flightCache.Clear()

	bbox := BoundingBox{
		LatMin: 35.0,
		LatMax: 45.0,
		LonMin: -110.0,
		LonMax: -90.0,
	}

	flights, _, err := fetchFlightsByArea(bbox)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !requestReceived {
		t.Error("mock server did not receive request")
	}

	if len(flights) != 1 {
		t.Fatalf("expected 1 flight, got %d", len(flights))
	}
}

// TestFetchFlights_ServerError tests error handling when API returns errors
func TestFetchFlights_ServerError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"500 Internal Server Error", http.StatusInternalServerError},
		{"503 Service Unavailable", http.StatusServiceUnavailable},
		{"429 Too Many Requests", http.StatusTooManyRequests},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer mockServer.Close()

			originalBaseURL := openSkyBaseURL
			defer func() { openSkyBaseURL = originalBaseURL }()
			openSkyBaseURL = mockServer.URL
			flightCache.Clear()

			_, _, err := fetchFlights("")
			if err == nil {
				t.Error("expected error for server error response")
			}
		})
	}
}

// TestFetchFlights_InvalidJSON tests handling of malformed responses
func TestFetchFlights_InvalidJSON(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("this is not valid JSON"))
	}))
	defer mockServer.Close()

	originalBaseURL := openSkyBaseURL
	defer func() { openSkyBaseURL = originalBaseURL }()
	openSkyBaseURL = mockServer.URL
	flightCache.Clear()

	_, _, err := fetchFlights("")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// TestFetchFlights_EmptyStates tests handling of response with no flights
func TestFetchFlights_EmptyStates(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := OpenSkyResponse{
			Time:   1234567890,
			States: nil, // No flights
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	originalBaseURL := openSkyBaseURL
	defer func() { openSkyBaseURL = originalBaseURL }()
	openSkyBaseURL = mockServer.URL
	flightCache.Clear()

	flights, _, err := fetchFlights("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(flights) != 0 {
		t.Errorf("expected 0 flights, got %d", len(flights))
	}
}

// TestCachePreventsDuplicateRequests verifies caching prevents redundant API calls
func TestCachePreventsDuplicateRequests(t *testing.T) {
	requestCount := 0

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		response := OpenSkyResponse{
			Time:   1234567890,
			States: [][]interface{}{},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	originalBaseURL := openSkyBaseURL
	defer func() { openSkyBaseURL = originalBaseURL }()
	openSkyBaseURL = mockServer.URL
	flightCache.Clear()

	// First call should hit the server
	_, _, _ = fetchFlights("")
	if requestCount != 1 {
		t.Errorf("expected 1 request after first call, got %d", requestCount)
	}

	// Second call should use cache
	_, _, _ = fetchFlights("")
	if requestCount != 1 {
		t.Errorf("expected 1 request after second call (cached), got %d", requestCount)
	}

	// Clear cache and call again
	flightCache.Clear()
	_, _, _ = fetchFlights("")
	if requestCount != 2 {
		t.Errorf("expected 2 requests after cache clear, got %d", requestCount)
	}
}

// strPtr is a helper to create string pointers for test data
func strPtr(s string) *string {
	return &s
}
