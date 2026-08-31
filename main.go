package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
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

// BoundingBox represents a geographic rectangle for area queries
// All values are in decimal degrees (WGS-84)
type BoundingBox struct {
	LatMin float64 // Southern boundary
	LatMax float64 // Northern boundary
	LonMin float64 // Western boundary
	LonMax float64 // Eastern boundary
}

// fetchFlights calls the OpenSky Network API and returns parsed flights
// Go functions can return multiple values: (result, error)
// This replaces try/catch - the caller must handle errors explicitly
func fetchFlights(icao24 string) ([]Flight, int64, error) {
	// Build the API URL
	url := "https://opensky-network.org/api/states/all"
	if icao24 != "" {
		url = fmt.Sprintf("%s?icao24=%s", url, icao24)
	}

	return doFetch(url)
}

// fetchFlightsByArea calls OpenSky with bounding box parameters
// OpenSky filters server-side, so this is efficient for geographic queries
func fetchFlightsByArea(bbox BoundingBox) ([]Flight, int64, error) {
	url := fmt.Sprintf(
		"https://opensky-network.org/api/states/all?lamin=%f&lamax=%f&lomin=%f&lomax=%f",
		bbox.LatMin, bbox.LatMax, bbox.LonMin, bbox.LonMax,
	)

	return doFetch(url)
}

// doFetch is a helper that handles the actual HTTP request and parsing
// This avoids duplicating the HTTP/JSON logic in multiple functions
func doFetch(url string) ([]Flight, int64, error) {
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
// FILTERING - Apply query param filters to flight data
// =============================================================================

// FilterParams holds the optional query parameters for filtering flights
// Using pointers (*string, *bool) lets us distinguish "not provided" from "empty"
// This is like: { country?: string; onGround?: boolean } in TypeScript
type FilterParams struct {
	Country  *string
	OnGround *bool
}

// filterFlights returns a new slice containing only flights that match the filters
// Go convention: don't mutate inputs, return new data
func filterFlights(flights []Flight, params FilterParams) []Flight {
	// If no filters, return original slice (optimization)
	if params.Country == nil && params.OnGround == nil {
		return flights
	}

	// make() with 0 length but capacity hint for performance
	filtered := make([]Flight, 0, len(flights))

	for _, flight := range flights {
		// Check country filter (case-sensitive match)
		if params.Country != nil && flight.OriginCountry != *params.Country {
			continue // Skip this flight - doesn't match
		}

		// Check on_ground filter
		if params.OnGround != nil && flight.OnGround != *params.OnGround {
			continue
		}

		// Passed all filters - include this flight
		filtered = append(filtered, flight)
	}

	return filtered
}

// =============================================================================
// HTTP HANDLERS
// =============================================================================

// getFlights handles GET /flights
// Supports query params: ?country=US&on_ground=true
func getFlights(c *gin.Context) {
	// -------------------------------------------------------------------------
	// STEP 1: Parse query params
	// c.Query() returns "" if the param isn't present
	// We convert to pointers so we can tell "not provided" from "empty string"
	// -------------------------------------------------------------------------
	var params FilterParams

	// Country filter - c.Query() is like req.query.country in Express
	country := c.Query("country")
	fmt.Printf("DEBUG: country param = %q\n", country) // %q shows quotes around strings

	if country != "" {
		params.Country = &country
	}

	// On-ground filter - need to parse string to bool
	onGroundStr := c.Query("on_ground")
	if onGroundStr != "" {
		onGround := onGroundStr == "true"
		params.OnGround = &onGround
	}

	// -------------------------------------------------------------------------
	// STEP 2: Fetch from OpenSky
	// -------------------------------------------------------------------------
	flights, timestamp, err := fetchFlights("")
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// -------------------------------------------------------------------------
	// STEP 3: Apply filters (if any)
	// -------------------------------------------------------------------------
	filtered := filterFlights(flights, params)

	c.JSON(http.StatusOK, FlightsResponse{
		Time:    timestamp,
		Count:   len(filtered),
		Flights: filtered,
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

// getFlightsByArea handles GET /flights/area?lamin=...&lamax=...&lomin=...&lomax=...
// Returns flights within a geographic bounding box
// Example: /flights/area?lamin=45.0&lamax=47.0&lomin=-123.0&lomax=-121.0 (Pacific Northwest)
func getFlightsByArea(c *gin.Context) {
	// -------------------------------------------------------------------------
	// Parse bounding box parameters
	// All 4 params are required - we use strconv.ParseFloat to convert strings
	// -------------------------------------------------------------------------
	laminStr := c.Query("lamin")
	lamaxStr := c.Query("lamax")
	lominStr := c.Query("lomin")
	lomaxStr := c.Query("lomax")

	// Check all params are provided
	if laminStr == "" || lamaxStr == "" || lominStr == "" || lomaxStr == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "missing required params: lamin, lamax, lomin, lomax",
			"example": "/flights/area?lamin=45.0&lamax=47.0&lomin=-123.0&lomax=-121.0",
		})
		return
	}

	// Parse strings to float64
	// strconv.ParseFloat(string, bitSize) returns (float64, error)
	// 64 = parse as float64 precision
	lamin, err := strconv.ParseFloat(laminStr, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "lamin must be a valid number",
		})
		return
	}

	lamax, err := strconv.ParseFloat(lamaxStr, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "lamax must be a valid number",
		})
		return
	}

	lomin, err := strconv.ParseFloat(lominStr, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "lomin must be a valid number",
		})
		return
	}

	lomax, err := strconv.ParseFloat(lomaxStr, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "lomax must be a valid number",
		})
		return
	}

	// Validate that min < max
	if lamin >= lamax {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "lamin must be less than lamax",
		})
		return
	}
	if lomin >= lomax {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "lomin must be less than lomax",
		})
		return
	}

	// -------------------------------------------------------------------------
	// Fetch from OpenSky with bounding box
	// -------------------------------------------------------------------------
	bbox := BoundingBox{
		LatMin: lamin,
		LatMax: lamax,
		LonMin: lomin,
		LonMax: lomax,
	}

	flights, timestamp, err := fetchFlightsByArea(bbox)
	if err != nil {
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

// getFlightsByCity handles GET /flights/city/:name
// Looks up city from our database, calculates bounding box, returns flights
// Optional query param: ?radius=0.5 (degrees, default 0.3)
func getFlightsByCity(c *gin.Context) {
	// Get city name from URL path
	name := c.Param("name")

	// Look up in our city database
	city, found := lookupCity(name)
	if !found {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("city not found: %s", name),
			"hint":  "try a major city name like 'toronto', 'london', 'tokyo'",
		})
		return
	}

	// Parse optional radius param (default 0.3 degrees ≈ 33km)
	radius := 0.3
	if radiusStr := c.Query("radius"); radiusStr != "" {
		if r, err := strconv.ParseFloat(radiusStr, 64); err == nil && r > 0 && r < 5 {
			radius = r
		}
	}

	// Convert to bounding box
	bbox := cityToBoundingBox(city, radius)

	// Fetch flights
	flights, timestamp, err := fetchFlightsByArea(bbox)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Return with city info included
	c.JSON(http.StatusOK, gin.H{
		"city": gin.H{
			"name":    city.Display,
			"country": city.Country,
			"lat":     city.Lat,
			"lng":     city.Lng,
			"radius":  radius,
		},
		"bbox": gin.H{
			"lamin": bbox.LatMin,
			"lamax": bbox.LatMax,
			"lomin": bbox.LonMin,
			"lomax": bbox.LonMax,
		},
		"time":    timestamp,
		"count":   len(flights),
		"flights": flights,
	})
}

func thing(c *gin.Context) {
	theStr := "hello man. what's up?"
	c.String(http.StatusOK, theStr)
}

// =============================================================================
// MAIN - Application entry point
// =============================================================================

func main() {
	// -------------------------------------------------------------------------
	// Load cities database at startup
	// -------------------------------------------------------------------------
	if err := loadCities("data/worldcities.csv"); err != nil {
		fmt.Printf("Warning: failed to load cities: %v\n", err)
		fmt.Println("City lookup endpoint will not work")
	} else {
		fmt.Printf("✅ Loaded %d cities\n", getCityCount())
	}

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
	router.GET("/flights/area", getFlightsByArea)       // Bounding box query
	router.GET("/flights/city/:name", getFlightsByCity) // City name lookup
	router.GET("/flights/:icao", getFlightByICAO)       // Must be last - catches all
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
