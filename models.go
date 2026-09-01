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

// BoundingBox represents a geographic rectangle for area queries
type BoundingBox struct {
	LatMin float64 // Southern boundary
	LatMax float64 // Northern boundary
	LonMin float64 // Western boundary
	LonMax float64 // Eastern boundary
}

// FilterParams holds the optional query parameters for filtering flights
type FilterParams struct {
	Country  *string
	OnGround *bool
}
