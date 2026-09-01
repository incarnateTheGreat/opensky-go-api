package main

import (
	"math"
	"testing"
)

// =============================================================================
// AIRPORT FUNCTION TESTS
// =============================================================================

// TestHaversineDistance tests the distance calculation between two points
func TestHaversineDistance(t *testing.T) {
	tests := []struct {
		name      string
		lat1      float64
		lng1      float64
		lat2      float64
		lng2      float64
		expected  float64 // expected distance in km (with tolerance)
		tolerance float64
	}{
		{
			name: "NYC to London",
			lat1: 40.7128, lng1: -74.0060, // New York
			lat2: 51.5074, lng2: -0.1278, // London
			expected:  5570, // roughly 5570 km
			tolerance: 50,
		},
		{
			name: "Toronto to Montreal",
			lat1: 43.6532, lng1: -79.3832, // Toronto
			lat2: 45.5017, lng2: -73.5673, // Montreal
			expected:  505, // roughly 505 km
			tolerance: 10,
		},
		{
			name: "same point",
			lat1: 45.0, lng1: -75.0,
			lat2: 45.0, lng2: -75.0,
			expected:  0,
			tolerance: 0.001,
		},
		{
			name: "across equator",
			lat1: 10.0, lng1: 0.0,
			lat2: -10.0, lng2: 0.0,
			expected:  2224, // roughly 2224 km
			tolerance: 20,
		},
		{
			name: "across date line",
			lat1: 35.0, lng1: 179.0,
			lat2: 35.0, lng2: -179.0,
			expected:  182, // roughly 182 km
			tolerance: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := haversineDistance(tt.lat1, tt.lng1, tt.lat2, tt.lng2)
			diff := math.Abs(result - tt.expected)
			if diff > tt.tolerance {
				t.Errorf("haversineDistance(%v,%v to %v,%v) = %.2f km, expected %.2f km (±%.2f)",
					tt.lat1, tt.lng1, tt.lat2, tt.lng2, result, tt.expected, tt.tolerance)
			}
		})
	}
}

// TestHaversineDistanceSymmetry tests that distance is the same in both directions
func TestHaversineDistanceSymmetry(t *testing.T) {
	lat1, lng1 := 40.7128, -74.0060 // NYC
	lat2, lng2 := 51.5074, -0.1278  // London

	d1 := haversineDistance(lat1, lng1, lat2, lng2)
	d2 := haversineDistance(lat2, lng2, lat1, lng1)

	if math.Abs(d1-d2) > 0.001 {
		t.Errorf("distance should be symmetric: NYC->London=%.2f, London->NYC=%.2f", d1, d2)
	}
}

// TestAirportToBoundingBox tests bounding box creation
func TestAirportToBoundingBox(t *testing.T) {
	airport := Airport{
		ICAO: "CYYZ",
		Lat:  43.6772,
		Lng:  -79.6306,
	}

	radius := 0.5 // degrees

	bbox := airportToBoundingBox(airport, radius)

	// Check bounds are correct
	if bbox.LatMin != airport.Lat-radius {
		t.Errorf("LatMin: expected %.4f, got %.4f", airport.Lat-radius, bbox.LatMin)
	}
	if bbox.LatMax != airport.Lat+radius {
		t.Errorf("LatMax: expected %.4f, got %.4f", airport.Lat+radius, bbox.LatMax)
	}
	if bbox.LonMin != airport.Lng-radius {
		t.Errorf("LonMin: expected %.4f, got %.4f", airport.Lng-radius, bbox.LonMin)
	}
	if bbox.LonMax != airport.Lng+radius {
		t.Errorf("LonMax: expected %.4f, got %.4f", airport.Lng+radius, bbox.LonMax)
	}
}

// TestLookupAirport tests ICAO lookup (requires airports to be loaded)
func TestLookupAirport(t *testing.T) {
	// Skip if airports not loaded (integration test)
	if len(airportByICAO) == 0 {
		t.Skip("airports not loaded - skipping integration test")
	}

	tests := []struct {
		icao       string
		shouldFind bool
	}{
		{"CYYZ", true},  // Toronto Pearson
		{"KJFK", true},  // JFK
		{"XXXX", false}, // Doesn't exist
		{"cyyz", true},  // Case insensitive
	}

	for _, tt := range tests {
		t.Run(tt.icao, func(t *testing.T) {
			_, found := lookupAirport(tt.icao)
			if found != tt.shouldFind {
				t.Errorf("lookupAirport(%s): found=%v, expected=%v", tt.icao, found, tt.shouldFind)
			}
		})
	}
}
