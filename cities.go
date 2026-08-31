package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// =============================================================================
// CITY DATA - Load and lookup cities from CSV
// =============================================================================

// City represents a location from our cities database
type City struct {
	Name        string  // City name (ASCII, lowercase for matching)
	Display     string  // Display name (original casing)
	Lat         float64 // Latitude
	Lng         float64 // Longitude
	Country     string  // Country name
	CountryCode string  // ISO2 country code
}

// cityIndex is our in-memory "database" of cities
// map[string]City - key is lowercase city name for case-insensitive lookup
// This is a package-level variable, initialized once at startup
var cityIndex map[string]City

// loadCities reads the CSV file and populates cityIndex
// Called once at application startup
func loadCities(filepath string) error {
	// Open the CSV file
	file, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("failed to open cities file: %w", err)
	}
	defer file.Close()

	// Create CSV reader
	reader := csv.NewReader(file)

	// Read all records (including header)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read CSV: %w", err)
	}

	// Initialize the map
	// len(records)-1 to exclude header, gives capacity hint
	cityIndex = make(map[string]City, len(records)-1)

	// Skip header row (index 0), process data rows
	// CSV columns: city, city_ascii, lat, lng, country, iso2, iso3, admin_name, capital, population, id
	for i, record := range records {
		if i == 0 {
			continue // Skip header
		}

		// Ensure we have enough columns
		if len(record) < 6 {
			continue
		}

		// Parse lat/lng
		lat, err := strconv.ParseFloat(record[2], 64)
		if err != nil {
			continue // Skip invalid rows
		}
		lng, err := strconv.ParseFloat(record[3], 64)
		if err != nil {
			continue
		}

		// Create city entry
		city := City{
			Name:        strings.ToLower(record[1]), // city_ascii, lowercased
			Display:     record[1],                  // city_ascii, original case
			Lat:         lat,
			Lng:         lng,
			Country:     record[4],
			CountryCode: record[5],
		}

		// Add to index using lowercase name as key
		// Note: This means if there are duplicate city names, the last one wins
		// A more robust solution would use a slice to handle duplicates
		cityIndex[city.Name] = city
	}

	return nil
}

// lookupCity finds a city by name (case-insensitive)
// Returns the city and a boolean indicating if found (Go's "comma ok" idiom)
func lookupCity(name string) (City, bool) {
	city, found := cityIndex[strings.ToLower(name)]
	return city, found
}

// cityToBoundingBox converts a city's lat/lng to a bounding box
// radius is in degrees (roughly: 1 degree ≈ 111 km at equator)
// For city-level queries, 0.3-0.5 degrees is reasonable
func cityToBoundingBox(city City, radius float64) BoundingBox {
	return BoundingBox{
		LatMin: city.Lat - radius,
		LatMax: city.Lat + radius,
		LonMin: city.Lng - radius,
		LonMax: city.Lng + radius,
	}
}

// getCityCount returns the number of cities loaded (for debugging/health checks)
func getCityCount() int {
	return len(cityIndex)
}
