package main

// =============================================================================
// MODELS - Data structures for the API
// =============================================================================

// Flight represents a single aircraft's state from OpenSky (real-time)
// Struct tags (the `json:"..."` part) control JSON serialization
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

// ErrorResponse represents a standard API error payload
type ErrorResponse struct {
	Error   string `json:"error"`
	Hint    string `json:"hint,omitempty"`
	Example string `json:"example,omitempty"`
}

// PingResponse represents the health check response
type PingResponse struct {
	Message string `json:"message"`
	Status  string `json:"status"`
}

// CacheStats represents cache stats details
type CacheStats struct {
	Entries int    `json:"entries"`
	TTL     string `json:"ttl"`
}

// CacheStatsResponse represents the cache stats response payload
type CacheStatsResponse struct {
	FlightCache CacheStats `json:"flightCache"`
}

// AirportResponse represents airport JSON payloads returned by handlers
type AirportResponse struct {
	ICAO         string  `json:"icao"`
	IATA         string  `json:"iata"`
	Name         string  `json:"name"`
	Type         string  `json:"type,omitempty"`
	Municipality string  `json:"municipality"`
	Country      string  `json:"country"`
	Region       string  `json:"region,omitempty"`
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
	Radius       float64 `json:"radius,omitempty"`
}

// AirportSearchResponse represents airport search results
type AirportSearchResponse struct {
	Query   string            `json:"query"`
	Count   int               `json:"count"`
	Results []AirportResponse `json:"results"`
}

// BoundingBoxResponse represents bounding box values in API responses
type BoundingBoxResponse struct {
	Lamin float64 `json:"lamin"`
	Lamax float64 `json:"lamax"`
	Lomin float64 `json:"lomin"`
	Lomax float64 `json:"lomax"`
}

// FlightsByAirportResponse represents flights in the area around an airport
type FlightsByAirportResponse struct {
	Airport AirportResponse     `json:"airport"`
	BBox    BoundingBoxResponse `json:"bbox"`
	Time    int64               `json:"time"`
	Count   int                 `json:"count"`
	Flights []Flight            `json:"flights"`
}

// BoundingBox represents a geographic rectangle for area queries
type BoundingBox struct {
	LatMin float64 // Southern boundary
	LatMax float64 // Northern boundary
	LonMin float64 // Western boundary
	LonMax float64 // Eastern boundary
}
