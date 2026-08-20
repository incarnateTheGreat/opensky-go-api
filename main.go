package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// =============================================================================
// STRUCTS - Go's version of TypeScript interfaces/types
// =============================================================================

// Flight represents a single aircraft's state from OpenSky
// Struct tags (the `json:"..."` part) control JSON serialization
// - Think of them like decorators that tell Go how to map JSON keys
// - Capitalized fields = exported (public), required for JSON marshaling
type Flight struct {
	Icao24        string   `json:"icao24"`         // Unique aircraft identifier
	Callsign      *string  `json:"callsign"`       // Flight number (pointer = nullable)
	OriginCountry string   `json:"origin_country"` // Country of registration
	Longitude     *float64 `json:"longitude"`      // WGS-84 longitude
	Latitude      *float64 `json:"latitude"`       // WGS-84 latitude
	Altitude      *float64 `json:"altitude"`       // Barometric altitude in meters
	Velocity      *float64 `json:"velocity"`       // Ground speed in m/s
	OnGround      bool     `json:"on_ground"`      // Is the aircraft on ground?
	LastContact   int64    `json:"last_contact"`   // Unix timestamp of last contact
}

// OpenSkyResponse is the raw API response structure
// The API returns states as [][]interface{} (mixed-type arrays)
// This is similar to: { time: number, states: any[][] } in TypeScript
type OpenSkyResponse struct {
	Time   int64           `json:"time"`
	States [][]interface{} `json:"states"` // Each state is a mixed array
}

// FlightsResponse is what our API returns - clean, typed data
type FlightsResponse struct {
	Time    int64    `json:"time"`
	Count   int      `json:"count"`
	Flights []Flight `json:"flights"`
}

// =============================================================================
// OPENSKY API CLIENT
// =============================================================================

// fetchFlights calls the OpenSky Network API and returns parsed flights
// Go functions can return multiple values: (result, error)
// This replaces try/catch - the caller must handle errors explicitly
func fetchFlights(icao24 string) ([]Flight, int64, error) {
	// Build the API URL
	url := "https://opensky-network.org/api/states/all"
	if icao24 != "" {
		url = fmt.Sprintf("%s?icao24=%s", url, icao24)
	}

	// Create HTTP client with timeout (good practice to avoid hanging)
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Make the request
	resp, err := client.Get(url)
	if err != nil {
		// Return empty slice, zero time, and the error
		// In Go, you return zero values for other returns when erroring
		return nil, 0, fmt.Errorf("failed to fetch from OpenSky: %w", err)
	}
	// defer = "run this when the function exits" (like finally in JS)
	// Important: always close response bodies to prevent memory leaks
	defer resp.Body.Close()

	// Check for non-200 responses
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("OpenSky returned status %d", resp.StatusCode)
	}

	// Decode JSON into our struct
	var openSkyResp OpenSkyResponse
	if err := json.NewDecoder(resp.Body).Decode(&openSkyResp); err != nil {
		return nil, 0, fmt.Errorf("failed to decode response: %w", err)
	}

	// Parse the raw states into typed Flight structs
	flights := parseStates(openSkyResp.States)

	return flights, openSkyResp.Time, nil
}

// parseStates converts OpenSky's mixed arrays into typed Flight structs
// OpenSky returns each state as an array like:
// [icao24, callsign, origin_country, time_position, last_contact, longitude, latitude, ...]
// This is awkward to work with, so we convert to proper structs
func parseStates(states [][]interface{}) []Flight {
	// make() creates a slice with initial capacity (performance optimization)
	flights := make([]Flight, 0, len(states))

	for _, state := range states {
		// Skip malformed entries
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

// =============================================================================
// HELPER FUNCTIONS - Safe type conversions from interface{}
// =============================================================================
// These handle the fact that OpenSky returns mixed-type arrays
// In TypeScript you'd use type guards; in Go we use type assertions

func safeString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func safeStringPtr(v interface{}) *string {
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		// Trim whitespace from callsigns
		trimmed := s
		if len(trimmed) > 0 {
			return &trimmed
		}
	}
	return nil
}

func safeFloat64Ptr(v interface{}) *float64 {
	if v == nil {
		return nil
	}
	// JSON numbers decode as float64 in Go
	if f, ok := v.(float64); ok {
		return &f
	}
	return nil
}

func safeInt64(v interface{}) int64 {
	if f, ok := v.(float64); ok {
		return int64(f)
	}
	return 0
}

func safeBool(v interface{}) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// =============================================================================
// HTTP HANDLERS
// =============================================================================

// getFlights handles GET /flights
// Returns all current flights from OpenSky
func getFlights(c *gin.Context) {
	flights, timestamp, err := fetchFlights("")
	if err != nil {
		// c.AbortWithStatusJSON stops the handler chain and returns error JSON
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, FlightsResponse{
		Time:    timestamp,
		Count:   len(flights),
		Flights: flights,
	})
}

// getFlightByICAO handles GET /flights/:icao
// Returns a specific flight by its ICAO24 identifier
func getFlightByICAO(c *gin.Context) {
	// c.Param() gets URL parameters (like req.params in Express)
	icao := c.Param("icao")

	flights, timestamp, err := fetchFlights(icao)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	if len(flights) == 0 {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("no flight found with ICAO24: %s", icao),
		})
		return
	}

	c.JSON(http.StatusOK, FlightsResponse{
		Time:    timestamp,
		Count:   len(flights),
		Flights: flights,
	})
}

func thing(c *gin.Context) {
	theStr := "hello"
	c.String(http.StatusOK, theStr)
}

// =============================================================================
// MAIN - Application entry point
// =============================================================================

func main() {
	// gin.Default() creates a router with Logger and Recovery middleware
	router := gin.Default()

	// Health check - Railway/Render/etc use this to verify deployment
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong thing",
			"status":  "healthy",
		})
	})

	// Flight endpoints
	router.GET("/flights", getFlights)
	router.GET("/flights/:icao", getFlightByICAO)
	router.GET("/arf", thing)

	// Get port from environment variable (Railway sets this)
	// os.Getenv returns "" if not set, so we default to 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Start the server
	fmt.Printf("🛫 OpenSky API server starting on port %s\n", port)
	router.Run(":" + port)
}
