package main

import (
	"testing"
)

// =============================================================================
// OPENSKY PARSING TESTS
// =============================================================================

// TestParseStates tests the OpenSky response parsing
func TestParseStates(t *testing.T) {
	// Mock OpenSky API response format
	// Index: 0=icao24, 1=callsign, 2=origin_country, 3=time_position,
	//        4=last_contact, 5=longitude, 6=latitude, 7=baro_altitude,
	//        8=on_ground, 9=velocity, 10=true_track, 11=vertical_rate,
	//        12=sensors, 13=geo_altitude, 14=squawk, 15=spi, 16=position_source

	states := [][]interface{}{
		{
			"abc123",        // 0: icao24
			"UAL123  ",      // 1: callsign
			"United States", // 2: origin_country
			1693500000.0,    // 3: time_position
			1693500001.0,    // 4: last_contact
			-73.7781,        // 5: longitude
			40.6413,         // 6: latitude
			10000.0,         // 7: baro_altitude
			false,           // 8: on_ground
			250.5,           // 9: velocity
			90.0,            // 10: true_track
			0.0,             // 11: vertical_rate
			nil,             // 12: sensors
			10050.0,         // 13: geo_altitude
			"1234",          // 14: squawk
			false,           // 15: spi
			0.0,             // 16: position_source
		},
	}

	flights := parseStates(states)

	if len(flights) != 1 {
		t.Fatalf("expected 1 flight, got %d", len(flights))
	}

	f := flights[0]

	if f.Icao24 != "abc123" {
		t.Errorf("Icao24: expected 'abc123', got '%s'", f.Icao24)
	}

	if f.Callsign == nil || *f.Callsign != "UAL123  " {
		t.Errorf("Callsign: expected 'UAL123  ', got %v", f.Callsign)
	}

	if f.OriginCountry != "United States" {
		t.Errorf("OriginCountry: expected 'United States', got '%s'", f.OriginCountry)
	}

	if f.Longitude == nil || *f.Longitude != -73.7781 {
		t.Errorf("Longitude: expected -73.7781, got %v", f.Longitude)
	}

	if f.Latitude == nil || *f.Latitude != 40.6413 {
		t.Errorf("Latitude: expected 40.6413, got %v", f.Latitude)
	}

	if f.OnGround != false {
		t.Errorf("OnGround: expected false, got %v", f.OnGround)
	}

	if f.Velocity == nil || *f.Velocity != 250.5 {
		t.Errorf("Velocity: expected 250.5, got %v", f.Velocity)
	}
}

// TestParseStatesEmpty tests parsing empty states
func TestParseStatesEmpty(t *testing.T) {
	states := [][]interface{}{}
	flights := parseStates(states)

	if len(flights) != 0 {
		t.Errorf("expected 0 flights for empty input, got %d", len(flights))
	}
}

// TestParseStatesNilValues tests parsing with nil values
func TestParseStatesNilValues(t *testing.T) {
	states := [][]interface{}{
		{
			"abc123",                          // 0: icao24
			nil,                               // 1: callsign (nil)
			"Canada",                          // 2: origin_country
			nil,                               // 3: time_position
			1693500001.0,                      // 4: last_contact
			nil,                               // 5: longitude (nil - no position)
			nil,                               // 6: latitude (nil - no position)
			nil,                               // 7: baro_altitude (nil)
			true,                              // 8: on_ground
			nil,                               // 9: velocity (nil)
			nil, nil, nil, nil, nil, nil, nil, // 10-16
		},
	}

	flights := parseStates(states)

	if len(flights) != 1 {
		t.Fatalf("expected 1 flight, got %d", len(flights))
	}

	f := flights[0]

	if f.Callsign != nil {
		t.Errorf("Callsign: expected nil, got %v", f.Callsign)
	}

	if f.Longitude != nil {
		t.Errorf("Longitude: expected nil, got %v", f.Longitude)
	}

	if f.Latitude != nil {
		t.Errorf("Latitude: expected nil, got %v", f.Latitude)
	}

	if f.OnGround != true {
		t.Errorf("OnGround: expected true, got %v", f.OnGround)
	}
}

// TestParseStatesShortArray tests that short arrays are skipped
func TestParseStatesShortArray(t *testing.T) {
	states := [][]interface{}{
		{"abc123", "UAL123"}, // Only 2 elements, should be skipped
		{
			"def456", "DAL456", "United States", nil, 1693500001.0,
			-80.0, 35.0, 8000.0, false, 200.0,
			nil, nil, nil, nil, nil, nil, nil, // Complete 17 elements
		},
	}

	flights := parseStates(states)

	if len(flights) != 1 {
		t.Errorf("expected 1 flight (short array skipped), got %d", len(flights))
	}

	if flights[0].Icao24 != "def456" {
		t.Errorf("expected def456, got %s", flights[0].Icao24)
	}
}

// TestParseStatesMultiple tests parsing multiple flights
func TestParseStatesMultiple(t *testing.T) {
	makeState := func(icao string) []interface{} {
		return []interface{}{
			icao, "CALL", "Country", nil, 1000.0,
			-75.0, 40.0, 10000.0, false, 250.0,
			nil, nil, nil, nil, nil, nil, nil,
		}
	}

	states := [][]interface{}{
		makeState("flight1"),
		makeState("flight2"),
		makeState("flight3"),
	}

	flights := parseStates(states)

	if len(flights) != 3 {
		t.Errorf("expected 3 flights, got %d", len(flights))
	}
}
