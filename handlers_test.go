package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// =============================================================================
// HANDLER TESTS - Testing HTTP handlers with httptest
// =============================================================================

// setupTestRouter creates a Gin router in test mode
// gin.SetMode(TestMode) disables debug logging for cleaner test output
func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New() // gin.New() instead of gin.Default() - no middleware
	return router
}

// TestPingHandler tests the health check endpoint
// This is the simplest handler to test - no external dependencies
func TestPingHandler(t *testing.T) {
	router := setupTestRouter()

	// Register the handler (inline version of what main.go does)
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
			"status":  "healthy",
		})
	})

	// Create a test request
	// httptest.NewRequest creates an http.Request for testing
	req := httptest.NewRequest("GET", "/ping", nil)

	// Create a response recorder
	// This captures what the handler writes
	w := httptest.NewRecorder()

	// Serve the request through the router
	router.ServeHTTP(w, req)

	// Verify status code
	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Parse the response body
	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Verify response fields
	if response["message"] != "pong" {
		t.Errorf("expected message 'pong', got '%s'", response["message"])
	}
	if response["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got '%s'", response["status"])
	}
}

// TestCacheStatsHandler tests the cache statistics endpoint
func TestCacheStatsHandler(t *testing.T) {
	router := setupTestRouter()

	// Register the cache stats handler
	router.GET("/cache/stats", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"flightCache": gin.H{
				"entries": flightCache.Size(),
				"ttl":     FlightCacheTTL.String(),
			},
		})
	})

	req := httptest.NewRequest("GET", "/cache/stats", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Verify JSON structure
	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Verify cache stats are present
	if _, ok := response["flightCache"]; !ok {
		t.Error("response missing flightCache")
	}
}

// TestSearchAirportsHandler_MissingQuery tests error handling for missing query param
func TestSearchAirportsHandler_MissingQuery(t *testing.T) {
	router := setupTestRouter()
	router.GET("/airports", searchAirportsHandler)

	req := httptest.NewRequest("GET", "/airports", nil) // No query param
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should return 400 Bad Request
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for missing query, got %d", http.StatusBadRequest, w.Code)
	}

	// Verify error message
	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if response["error"] == "" {
		t.Error("expected error message in response")
	}
}

// TestSearchAirportsHandler_NotFound tests error handling when no airports match
func TestSearchAirportsHandler_NotFound(t *testing.T) {
	// Initialize empty search indexes
	searchableAirports = []SearchableAirport{}
	airportByICAO = make(map[string]Airport)
	airportByIATA = make(map[string]Airport)
	airportsByCityWord = make(map[string][]*SearchableAirport)

	router := setupTestRouter()
	router.GET("/airports", searchAirportsHandler)

	req := httptest.NewRequest("GET", "/airports?q=xyznonexistent", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should return 404 Not Found
	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d for no results, got %d", http.StatusNotFound, w.Code)
	}
}

// TestGetAirportByICAO_NotFound tests error handling for unknown airport
func TestGetAirportByICAO_NotFound(t *testing.T) {
	// Empty the airport lookup
	airportByICAO = make(map[string]Airport)

	router := setupTestRouter()
	router.GET("/airports/:icao", getAirportByICAO)

	req := httptest.NewRequest("GET", "/airports/XXXX", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if response["error"] == "" {
		t.Error("expected error message")
	}
}

// TestGetAirportByICAO_Success tests successful airport lookup
func TestGetAirportByICAO_Success(t *testing.T) {
	// Set up test airport
	airportByICAO = map[string]Airport{
		"KJFK": {
			ICAO:         "KJFK",
			IATA:         "JFK",
			Name:         "John F Kennedy International Airport",
			Type:         "large_airport",
			Municipality: "New York",
			Country:      "US",
			Lat:          40.6413,
			Lng:          -73.7781,
		},
	}
	promotedAirports = make(map[string]bool)

	router := setupTestRouter()
	router.GET("/airports/:icao", getAirportByICAO)

	req := httptest.NewRequest("GET", "/airports/KJFK", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response["icao"] != "KJFK" {
		t.Errorf("expected ICAO 'KJFK', got '%v'", response["icao"])
	}
	if response["iata"] != "JFK" {
		t.Errorf("expected IATA 'JFK', got '%v'", response["iata"])
	}
}

// TestGetFlightsByArea_MissingParams tests error handling for missing bounding box
func TestGetFlightsByArea_MissingParams(t *testing.T) {
	router := setupTestRouter()
	router.GET("/flights/area", getFlightsByArea)

	tests := []struct {
		name  string
		query string
	}{
		{"missing all params", ""},
		{"missing lamin", "lamax=47&lomin=-123&lomax=-121"},
		{"missing lamax", "lamin=45&lomin=-123&lomax=-121"},
		{"missing lomin", "lamin=45&lamax=47&lomax=-121"},
		{"missing lomax", "lamin=45&lamax=47&lomin=-123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/flights/area"
			if tt.query != "" {
				url += "?" + tt.query
			}

			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
			}
		})
	}
}

// TestGetFlightsByArea_InvalidParams tests error handling for invalid parameter values
func TestGetFlightsByArea_InvalidParams(t *testing.T) {
	router := setupTestRouter()
	router.GET("/flights/area", getFlightsByArea)

	tests := []struct {
		name  string
		query string
	}{
		{"lamin not a number", "lamin=abc&lamax=47&lomin=-123&lomax=-121"},
		{"lamax not a number", "lamin=45&lamax=xyz&lomin=-123&lomax=-121"},
		{"lomin not a number", "lamin=45&lamax=47&lomin=foo&lomax=-121"},
		{"lomax not a number", "lamin=45&lamax=47&lomin=-123&lomax=bar"},
		{"lamin >= lamax", "lamin=50&lamax=45&lomin=-123&lomax=-121"},
		{"lomin >= lomax", "lamin=45&lamax=47&lomin=-121&lomax=-123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/flights/area?"+tt.query, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
			}
		})
	}
}

// TestGetFlightsByAirport_NotFound tests error handling for unknown airport
func TestGetFlightsByAirport_NotFound(t *testing.T) {
	airportByICAO = make(map[string]Airport)

	router := setupTestRouter()
	router.GET("/flights/airport/:icao", getFlightsByAirport)

	req := httptest.NewRequest("GET", "/flights/airport/XXXX", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

// TestSearchAirportsHandler_Success tests successful airport search
func TestSearchAirportsHandler_Success(t *testing.T) {
	// Set up test airports
	testAirport := Airport{
		ICAO:         "KJFK",
		IATA:         "JFK",
		Name:         "John F Kennedy International Airport",
		Type:         "large_airport",
		Municipality: "New York",
		Country:      "US",
		Lat:          40.6413,
		Lng:          -73.7781,
	}

	airportByICAO = map[string]Airport{"KJFK": testAirport}
	airportByIATA = map[string]Airport{"JFK": testAirport}
	searchableAirports = []SearchableAirport{
		{
			Airport:   testAirport,
			icaoLower: "kjfk",
			iataLower: "jfk",
			nameLower: "john f kennedy international airport",
			cityLower: "new york",
		},
	}
	airportsByCityWord = make(map[string][]*SearchableAirport)
	airportsByCityWord["new"] = []*SearchableAirport{&searchableAirports[0]}
	airportsByCityWord["york"] = []*SearchableAirport{&searchableAirports[0]}

	router := setupTestRouter()
	router.GET("/airports", searchAirportsHandler)

	req := httptest.NewRequest("GET", "/airports?q=JFK", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	count := int(response["count"].(float64))
	if count == 0 {
		t.Error("expected at least one airport result")
	}
}
