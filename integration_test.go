package main

import (
	"os"
	"strings"
	"testing"
)

// =============================================================================
// INTEGRATION TESTS - Testing with real CSV data
// =============================================================================

// These tests load the actual airports.csv file and test real functionality.
// They're slower than unit tests but verify the complete system works.

// Integration tests are commonly skipped in CI with: go test -short ./...
// They run when you do: go test ./...

// TestIntegration_LoadAirportsFromCSV tests loading the real CSV file
func TestIntegration_LoadAirportsFromCSV(t *testing.T) {
	// Skip if running in short mode (CI)
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check if CSV file exists
	if _, err := os.Stat("data/airports.csv"); os.IsNotExist(err) {
		t.Skip("airports.csv not found - skipping integration test")
	}

	// Reset global state
	airportByICAO = nil
	airportByIATA = nil
	airportsByCity = nil
	airportsByCityWord = nil
	allAirports = nil
	searchableAirports = nil
	promotedAirports = nil

	err := loadAirports("data/airports.csv")
	if err != nil {
		t.Fatalf("failed to load airports: %v", err)
	}

	// Verify we loaded a reasonable number of airports
	totalCount := len(airportByICAO)
	if totalCount < 50000 {
		t.Errorf("expected at least 50,000 airports, got %d", totalCount)
	}

	mediumLarge := len(searchableAirports)
	if mediumLarge < 3000 {
		t.Errorf("expected at least 3,000 medium/large airports, got %d", mediumLarge)
	}

	t.Logf("Loaded %d total airports, %d medium/large (searchable)", totalCount, mediumLarge)
}

// TestIntegration_LookupMajorAirports tests looking up well-known airports
func TestIntegration_LookupMajorAirports(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if _, err := os.Stat("data/airports.csv"); os.IsNotExist(err) {
		t.Skip("airports.csv not found")
	}

	// Ensure airports are loaded
	if len(airportByICAO) == 0 {
		if err := loadAirports("data/airports.csv"); err != nil {
			t.Fatalf("failed to load airports: %v", err)
		}
	}

	// Test known major airports
	majorAirports := []struct {
		icao        string
		iata        string
		city        string
		country     string
		airportType string
	}{
		{"KJFK", "JFK", "New York", "US", "large_airport"},
		{"EGLL", "LHR", "London", "GB", "large_airport"},
		{"RJTT", "HND", "Tokyo", "JP", "large_airport"},
		{"LFPG", "CDG", "Paris", "FR", "large_airport"},
		{"KLAX", "LAX", "Los Angeles", "US", "large_airport"},
		{"OMDB", "DXB", "Dubai", "AE", "large_airport"},
		{"CYYZ", "YYZ", "Toronto", "CA", "large_airport"},
		{"VHHH", "HKG", "Hong Kong", "HK", "large_airport"},
		{"WSSS", "SIN", "Singapore", "SG", "large_airport"},
		{"EDDF", "FRA", "Frankfurt", "DE", "large_airport"},
	}

	for _, test := range majorAirports {
		t.Run(test.icao, func(t *testing.T) {
			// Test ICAO lookup
			airport, found := lookupAirport(test.icao)
			if !found {
				t.Errorf("airport %s not found", test.icao)
				return
			}

			// Verify ICAO code
			if airport.ICAO != test.icao {
				t.Errorf("expected ICAO %s, got %s", test.icao, airport.ICAO)
			}

			// Verify IATA code (if expected)
			if test.iata != "" && airport.IATA != test.iata {
				t.Errorf("expected IATA %s, got %s", test.iata, airport.IATA)
			}

			// Verify country
			if airport.Country != test.country {
				t.Errorf("expected country %s, got %s", test.country, airport.Country)
			}

			// Verify type
			if airport.Type != test.airportType {
				t.Errorf("expected type %s, got %s", test.airportType, airport.Type)
			}

			// Verify coordinates are reasonable (not null island)
			if airport.Lat == 0 && airport.Lng == 0 {
				t.Error("coordinates are at null island (0,0)")
			}
		})
	}
}

// TestIntegration_IATALookup tests looking up airports by IATA code
func TestIntegration_IATALookup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if _, err := os.Stat("data/airports.csv"); os.IsNotExist(err) {
		t.Skip("airports.csv not found")
	}

	if len(airportByICAO) == 0 {
		if err := loadAirports("data/airports.csv"); err != nil {
			t.Fatalf("failed to load airports: %v", err)
		}
	}

	// Test some IATA codes
	iataCodes := []string{"JFK", "LAX", "ORD", "DFW", "SFO", "ATL", "MCO", "DEN", "SEA", "MIA"}

	for _, iata := range iataCodes {
		t.Run(iata, func(t *testing.T) {
			airport, found := airportByIATA[iata]
			if !found {
				t.Errorf("IATA code %s not found", iata)
				return
			}

			if airport.IATA != iata {
				t.Errorf("expected IATA %s, got %s", iata, airport.IATA)
			}
		})
	}
}

// TestIntegration_SearchByCity tests city-based airport search
func TestIntegration_SearchByCity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if _, err := os.Stat("data/airports.csv"); os.IsNotExist(err) {
		t.Skip("airports.csv not found")
	}

	if len(airportByICAO) == 0 {
		if err := loadAirports("data/airports.csv"); err != nil {
			t.Fatalf("failed to load airports: %v", err)
		}
	}

	// Test city searches
	cityTests := []struct {
		query       string
		minResults  int
		expectICAOs []string // At least one of these should appear
	}{
		{"new york", 2, []string{"KJFK", "KEWR", "KLGA"}},
		{"london", 2, []string{"EGLL", "EGLC", "EGKK"}},
		{"los angeles", 1, []string{"KLAX"}},
		{"chicago", 1, []string{"KORD", "KMDW"}},
		{"toronto", 1, []string{"CYYZ"}},
	}

	for _, test := range cityTests {
		t.Run(test.query, func(t *testing.T) {
			results := searchAirports(test.query, 20)

			if len(results) < test.minResults {
				t.Errorf("expected at least %d results for '%s', got %d",
					test.minResults, test.query, len(results))
			}

			// Check if at least one expected ICAO is in results
			found := false
			resultICAOs := make([]string, len(results))
			for i, r := range results {
				resultICAOs[i] = r.ICAO
				for _, expected := range test.expectICAOs {
					if r.ICAO == expected {
						found = true
						break
					}
				}
			}

			if !found {
				t.Errorf("search for '%s' didn't return any expected airports. Got: %v, expected one of: %v",
					test.query, resultICAOs, test.expectICAOs)
			}
		})
	}
}

// TestIntegration_SearchByICAO tests ICAO code search
func TestIntegration_SearchByICAO(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if _, err := os.Stat("data/airports.csv"); os.IsNotExist(err) {
		t.Skip("airports.csv not found")
	}

	if len(airportByICAO) == 0 {
		if err := loadAirports("data/airports.csv"); err != nil {
			t.Fatalf("failed to load airports: %v", err)
		}
	}

	// Searching for exact ICAO code should return that airport as first result
	icaoTests := []string{"KJFK", "EGLL", "LFPG", "RJTT"}

	for _, icao := range icaoTests {
		t.Run(icao, func(t *testing.T) {
			results := searchAirports(icao, 5)

			if len(results) == 0 {
				t.Errorf("no results for ICAO search '%s'", icao)
				return
			}

			// First result should be exact match
			if results[0].ICAO != icao {
				t.Errorf("first result for '%s' should be exact match, got %s",
					icao, results[0].ICAO)
			}
		})
	}
}

// TestIntegration_SearchByIATA tests IATA code search
func TestIntegration_SearchByIATA(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if _, err := os.Stat("data/airports.csv"); os.IsNotExist(err) {
		t.Skip("airports.csv not found")
	}

	if len(airportByICAO) == 0 {
		if err := loadAirports("data/airports.csv"); err != nil {
			t.Fatalf("failed to load airports: %v", err)
		}
	}

	// Searching for IATA code should return that airport
	iataTests := []struct {
		iata         string
		expectedICAO string
	}{
		{"JFK", "KJFK"},
		{"LHR", "EGLL"},
		{"LAX", "KLAX"},
		{"CDG", "LFPG"},
	}

	for _, test := range iataTests {
		t.Run(test.iata, func(t *testing.T) {
			results := searchAirports(test.iata, 5)

			if len(results) == 0 {
				t.Errorf("no results for IATA search '%s'", test.iata)
				return
			}

			// Should find the correct airport
			found := false
			for _, r := range results {
				if r.ICAO == test.expectedICAO {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("IATA search '%s' didn't return expected airport %s",
					test.iata, test.expectedICAO)
			}
		})
	}
}

// TestIntegration_AirportToBoundingBox tests bounding box creation
func TestIntegration_AirportToBoundingBox(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if _, err := os.Stat("data/airports.csv"); os.IsNotExist(err) {
		t.Skip("airports.csv not found")
	}

	if len(airportByICAO) == 0 {
		if err := loadAirports("data/airports.csv"); err != nil {
			t.Fatalf("failed to load airports: %v", err)
		}
	}

	airport, found := lookupAirport("KJFK")
	if !found {
		t.Fatal("KJFK not found")
	}

	radius := 0.3 // degrees
	bbox := airportToBoundingBox(airport, radius)

	// Verify bounding box dimensions (use tolerance for float comparison)
	expectedRange := radius * 2
	actualRange := bbox.LatMax - bbox.LatMin
	if abs(actualRange-expectedRange) > 0.0001 {
		t.Errorf("latitude range should be %f, got %f", expectedRange, actualRange)
	}

	// Airport should be in center of bounding box
	centerLat := (bbox.LatMin + bbox.LatMax) / 2
	centerLng := (bbox.LonMin + bbox.LonMax) / 2

	if abs(centerLat-airport.Lat) > 0.001 {
		t.Errorf("airport not centered in bounding box (lat)")
	}
	if abs(centerLng-airport.Lng) > 0.001 {
		t.Errorf("airport not centered in bounding box (lng)")
	}
}

// TestIntegration_AirportsNearLocation tests proximity search
func TestIntegration_AirportsNearLocation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if _, err := os.Stat("data/airports.csv"); os.IsNotExist(err) {
		t.Skip("airports.csv not found")
	}

	if len(airportByICAO) == 0 {
		if err := loadAirports("data/airports.csv"); err != nil {
			t.Fatalf("failed to load airports: %v", err)
		}
	}

	// JFK coordinates
	jfkLat, jfkLng := 40.6413, -73.7781

	// Create a bounding box around JFK (about 50km)
	radius := 0.5
	bbox := BoundingBox{
		LatMin: jfkLat - radius,
		LatMax: jfkLat + radius,
		LonMin: jfkLng - radius,
		LonMax: jfkLng + radius,
	}

	// Count airports in the bounding box
	count := 0
	foundJFK := false
	foundLGA := false

	for _, airport := range allAirports {
		if airport.Lat >= bbox.LatMin && airport.Lat <= bbox.LatMax &&
			airport.Lng >= bbox.LonMin && airport.Lng <= bbox.LonMax {
			count++
			if airport.ICAO == "KJFK" {
				foundJFK = true
			}
			if airport.ICAO == "KLGA" {
				foundLGA = true
			}
		}
	}

	if !foundJFK {
		t.Error("JFK should be in bounding box around JFK")
	}

	if !foundLGA {
		t.Error("LGA should be within 50km of JFK")
	}

	t.Logf("Found %d airports within %.1f degrees of JFK", count, radius)
}

// TestIntegration_SmallAirportPromotion tests the memoization feature
func TestIntegration_SmallAirportPromotion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if _, err := os.Stat("data/airports.csv"); os.IsNotExist(err) {
		t.Skip("airports.csv not found")
	}

	if len(airportByICAO) == 0 {
		if err := loadAirports("data/airports.csv"); err != nil {
			t.Fatalf("failed to load airports: %v", err)
		}
	}

	// Find a small airport
	var smallAirportICAO string
	for icao, airport := range airportByICAO {
		if airport.Type == "small_airport" || airport.Type == "heliport" {
			smallAirportICAO = icao
			break
		}
	}

	if smallAirportICAO == "" {
		t.Skip("no small airports found")
	}

	// Clear promoted airports tracking
	promotedAirports = make(map[string]bool)
	initialSearchable := len(searchableAirports)

	// Look up the small airport (should trigger promotion)
	_, found := lookupAirport(smallAirportICAO)
	if !found {
		t.Fatalf("small airport %s not found", smallAirportICAO)
	}

	// Verify it was promoted
	if !promotedAirports[strings.ToUpper(smallAirportICAO)] {
		t.Error("small airport should have been promoted")
	}

	// Second lookup should not add it again
	_, _ = lookupAirport(smallAirportICAO)

	// Check that searchableAirports grew by exactly 1
	if len(searchableAirports) != initialSearchable+1 {
		t.Errorf("expected searchableAirports to grow by 1, was %d now %d",
			initialSearchable, len(searchableAirports))
	}
}

// TestIntegration_HaversineRealDistance tests haversine with known distances
func TestIntegration_HaversineRealDistance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	if _, err := os.Stat("data/airports.csv"); os.IsNotExist(err) {
		t.Skip("airports.csv not found")
	}

	if len(airportByICAO) == 0 {
		if err := loadAirports("data/airports.csv"); err != nil {
			t.Fatalf("failed to load airports: %v", err)
		}
	}

	// Test known distances between major airports
	// JFK to LAX is approximately 3983 km
	jfk, _ := lookupAirport("KJFK")
	lax, _ := lookupAirport("KLAX")

	distance := haversineDistance(jfk.Lat, jfk.Lng, lax.Lat, lax.Lng)

	// Should be approximately 3983 km (allow 2% margin)
	expected := 3983.0
	margin := expected * 0.02

	if distance < expected-margin || distance > expected+margin {
		t.Errorf("JFK-LAX distance should be ~%.0f km, got %.0f km", expected, distance)
	}

	t.Logf("JFK to LAX: %.0f km (expected ~%.0f km)", distance, expected)

	// JFK to LHR is approximately 5555 km
	lhr, _ := lookupAirport("EGLL")
	distance2 := haversineDistance(jfk.Lat, jfk.Lng, lhr.Lat, lhr.Lng)

	expected2 := 5555.0
	margin2 := expected2 * 0.02

	if distance2 < expected2-margin2 || distance2 > expected2+margin2 {
		t.Errorf("JFK-LHR distance should be ~%.0f km, got %.0f km", expected2, distance2)
	}

	t.Logf("JFK to LHR: %.0f km (expected ~%.0f km)", distance2, expected2)
}

// abs returns absolute value of float64
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
