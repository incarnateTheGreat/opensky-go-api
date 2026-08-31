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

// HistoricalFlight represents a flight from OpenSky's arrival/departure endpoints
// This has airport info unlike real-time Flight
type HistoricalFlight struct {
	Icao24                     string  `json:"icao24"`
	FirstSeen                  int64   `json:"firstSeen"`
	EstDepartureAirport        *string `json:"estDepartureAirport"`
	LastSeen                   int64   `json:"lastSeen"`
	EstArrivalAirport          *string `json:"estArrivalAirport"`
	Callsign                   *string `json:"callsign"`
	EstDepartureAirportHoriz   int     `json:"estDepartureAirportHorizDistance"`
	EstDepartureAirportVert    int     `json:"estDepartureAirportVertDistance"`
	EstArrivalAirportHoriz     int     `json:"estArrivalAirportHorizDistance"`
	EstArrivalAirportVert      int     `json:"estArrivalAirportVertDistance"`
	DepartureAirportCandidates int     `json:"departureAirportCandidatesCount"`
	ArrivalAirportCandidates   int     `json:"arrivalAirportCandidatesCount"`
}

// HistoricalFlightsResponse is what our API returns for arrival/departure queries
type HistoricalFlightsResponse struct {
	Airport string             `json:"airport"`
	Type    string             `json:"type"` // "arrivals" or "departures"
	Begin   int64              `json:"begin"`
	End     int64              `json:"end"`
	Count   int                `json:"count"`
	Flights []HistoricalFlight `json:"flights"`
}

// BoundingBox represents a geographic rectangle for area queries
type BoundingBox struct {
	LatMin float64 // Southern boundary
	LatMax float64 // Northern boundary
	LonMin float64 // Western boundary
	LonMax float64 // Eastern boundary
}

// TokenResponse represents the OAuth2 token response from OpenSky
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// FilterParams holds the optional query parameters for filtering flights
type FilterParams struct {
	Country  *string
	OnGround *bool
}
