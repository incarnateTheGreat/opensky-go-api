package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// =============================================================================
// AIRPORT DATA - Load and lookup airports from CSV
// =============================================================================

// Airport represents an airport from the OurAirports database
type Airport struct {
	ICAO         string  // ICAO code (from ident column)
	IATA         string  // IATA code (3-letter, e.g., JFK)
	Name         string  // Airport name
	Type         string  // large_airport, medium_airport, small_airport, heliport, etc.
	Lat          float64 // Latitude
	Lng          float64 // Longitude
	Municipality string  // City name
	Country      string  // ISO country code
	Region       string  // ISO region (e.g., US-NY)
}

// SearchableAirport extends Airport with pre-computed lowercase fields for fast search
type SearchableAirport struct {
	Airport
	icaoLower string // Pre-computed lowercase ICAO
	iataLower string // Pre-computed lowercase IATA
	nameLower string // Pre-computed lowercase name
	cityLower string // Pre-computed lowercase municipality
}

// airportByICAO maps ICAO code -> Airport for direct lookups
var airportByICAO map[string]Airport

// airportByIATA maps IATA code -> Airport for direct lookups (e.g., "JFK" -> Airport)
var airportByIATA map[string]Airport

// airportsByCity maps lowercase city name -> slice of airports
// Only includes medium and large airports for practical city lookups
var airportsByCity map[string][]Airport

// airportsByCityWord maps individual words in city names -> slice of searchable airports
// e.g., "new" -> [JFK, EWR, ...], "york" -> [JFK, ...]
var airportsByCityWord map[string][]*SearchableAirport

// allAirports holds all loaded airports for proximity searches
var allAirports []Airport

// searchableAirports holds only medium/large airports with pre-computed search fields
// This is ~4k airports vs 70k total - much faster to iterate
// Can grow dynamically when small airports are accessed (memoization)
var searchableAirports []SearchableAirport

// searchMu protects concurrent access to searchableAirports and promotedAirports
// RWMutex allows multiple concurrent reads, but exclusive writes
var searchMu sync.RWMutex

// promotedAirports tracks which small airports have been promoted to searchable
// Prevents duplicate entries in searchableAirports
var promotedAirports map[string]bool

// loadAirports reads the OurAirports CSV and populates lookup maps
// CSV columns: id,ident,type,name,latitude_deg,longitude_deg,elevation_ft,
//
//	continent,iso_country,iso_region,municipality,scheduled_service,
//	icao_code,iata_code,gps_code,local_code,home_link,wikipedia_link,keywords
func loadAirports(filepath string) error {
	file, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("failed to open airports file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read airports CSV: %w", err)
	}

	// Initialize maps and slices
	airportByICAO = make(map[string]Airport, len(records))
	airportByIATA = make(map[string]Airport)
	airportsByCity = make(map[string][]Airport)
	airportsByCityWord = make(map[string][]*SearchableAirport)
	allAirports = make([]Airport, 0, len(records))
	searchableAirports = make([]SearchableAirport, 0, 5000) // ~4k medium/large airports
	promotedAirports = make(map[string]bool)                // Track memoized small airports

	// Skip header (index 0)
	for i, record := range records {
		if i == 0 {
			continue
		}

		// Need at least 14 columns
		if len(record) < 14 {
			continue
		}

		// Parse coordinates
		lat, err := strconv.ParseFloat(record[4], 64)
		if err != nil {
			continue
		}
		lng, err := strconv.ParseFloat(record[5], 64)
		if err != nil {
			continue
		}

		// Get ICAO from ident column (column 1)
		icao := strings.TrimSpace(record[1])
		if icao == "" {
			continue
		}

		// Get IATA code (column 13)
		iata := strings.Trim(record[13], "\"")

		airport := Airport{
			ICAO:         icao,
			IATA:         iata,
			Name:         record[3],
			Type:         record[2],
			Lat:          lat,
			Lng:          lng,
			Municipality: record[10],
			Country:      record[8],
			Region:       record[9],
		}

		// Add to ICAO lookup (all airports)
		airportByICAO[strings.ToUpper(icao)] = airport
		allAirports = append(allAirports, airport)

		// Add to IATA lookup (if IATA exists)
		if iata != "" {
			airportByIATA[strings.ToUpper(iata)] = airport
		}

		// For medium/large airports, build search indexes
		if airport.Type == "large_airport" || airport.Type == "medium_airport" {
			// Add to city lookup
			if airport.Municipality != "" {
				cityKey := strings.ToLower(airport.Municipality)
				airportsByCity[cityKey] = append(airportsByCity[cityKey], airport)
			}

			// Create searchable airport with pre-computed lowercase fields
			searchable := SearchableAirport{
				Airport:   airport,
				icaoLower: strings.ToLower(icao),
				iataLower: strings.ToLower(iata),
				nameLower: strings.ToLower(airport.Name),
				cityLower: strings.ToLower(airport.Municipality),
			}
			searchableAirports = append(searchableAirports, searchable)
		}
	}

	// Build word index for city names (enables "new" to find "New York")
	// Do this in a second pass so we can use pointers to the final slice positions
	for i := range searchableAirports {
		sa := &searchableAirports[i]
		if sa.cityLower != "" {
			// Split city name into words and index each
			words := strings.Fields(sa.cityLower)
			for _, word := range words {
				// Clean up word (remove punctuation)
				word = strings.Trim(word, ".,;:-'\"")
				if len(word) >= 2 { // Skip very short words
					airportsByCityWord[word] = append(airportsByCityWord[word], sa)
				}
			}
		}
	}

	return nil
}

// lookupAirport finds an airport by ICAO code
// If found and it's a small airport, promotes it to searchable (memoization)
func lookupAirport(icao string) (Airport, bool) {
	airport, found := airportByICAO[strings.ToUpper(icao)]
	if found {
		// Memoize: if it's a small airport, promote it to searchable
		promoteAirport(airport)
	}
	return airport, found
}

// promoteAirport adds a small airport to the searchable set (memoization)
// Thread-safe: uses mutex since Gin handlers run concurrently
// No-op if airport is already medium/large or already promoted
func promoteAirport(airport Airport) {
	// Skip if already in searchable set (medium/large airports)
	if airport.Type == "large_airport" || airport.Type == "medium_airport" {
		return
	}

	icaoUpper := strings.ToUpper(airport.ICAO)

	// Check if already promoted (read lock)
	searchMu.RLock()
	alreadyPromoted := promotedAirports[icaoUpper]
	searchMu.RUnlock()

	if alreadyPromoted {
		return
	}

	// Promote: add to searchable set (write lock)
	searchMu.Lock()
	defer searchMu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have promoted it)
	if promotedAirports[icaoUpper] {
		return
	}

	// Create searchable entry with pre-computed lowercase
	searchable := SearchableAirport{
		Airport:   airport,
		icaoLower: strings.ToLower(airport.ICAO),
		iataLower: strings.ToLower(airport.IATA),
		nameLower: strings.ToLower(airport.Name),
		cityLower: strings.ToLower(airport.Municipality),
	}

	searchableAirports = append(searchableAirports, searchable)
	promotedAirports[icaoUpper] = true

	fmt.Printf("📌 Promoted airport to searchable: %s (%s)\n", airport.ICAO, airport.Name)
}

// lookupAirportsByCity finds airports serving a city (by municipality name)
func lookupAirportsByCity(cityName string) []Airport {
	return airportsByCity[strings.ToLower(cityName)]
}

// findNearbyAirports returns airports within a given radius (in km) of coordinates
// Only returns medium and large airports, sorted by distance
func findNearbyAirports(lat, lng, radiusKm float64) []Airport {
	var nearby []Airport

	for _, airport := range allAirports {
		// Only medium and large airports
		if airport.Type != "large_airport" && airport.Type != "medium_airport" {
			continue
		}

		dist := haversineDistance(lat, lng, airport.Lat, airport.Lng)
		if dist <= radiusKm {
			nearby = append(nearby, airport)
		}
	}

	// Sort by distance
	sort.Slice(nearby, func(i, j int) bool {
		distI := haversineDistance(lat, lng, nearby[i].Lat, nearby[i].Lng)
		distJ := haversineDistance(lat, lng, nearby[j].Lat, nearby[j].Lng)
		return distI < distJ
	})

	return nearby
}

// haversineDistance calculates the distance between two points in km
// using the Haversine formula
func haversineDistance(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKm = 6371.0

	// Convert to radians
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLng := (lng2 - lng1) * math.Pi / 180

	// Haversine formula
	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLng/2)*math.Sin(deltaLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusKm * c
}

// getAirportCount returns the number of loaded airports
func getAirportCount() int {
	return len(airportByICAO)
}

// getMediumLargeAirportCount returns count of searchable airports
// Includes promoted small airports
func getMediumLargeAirportCount() int {
	searchMu.RLock()
	defer searchMu.RUnlock()
	return len(searchableAirports)
}

// lookupAirportByIATA finds an airport by IATA code (e.g., "JFK")
// If found and it's a small airport, promotes it to searchable (memoization)
func lookupAirportByIATA(iata string) (Airport, bool) {
	airport, found := airportByIATA[strings.ToUpper(iata)]
	if found {
		promoteAirport(airport)
	}
	return airport, found
}

// getPromotedCount returns the number of small airports promoted to searchable
func getPromotedCount() int {
	searchMu.RLock()
	defer searchMu.RUnlock()
	return len(promotedAirports)
}

// searchAirports finds airports matching a query string
// Uses pre-built indexes for fast lookups:
// - O(1) for exact ICAO/IATA matches
// - O(1) for exact city word matches
// - O(n) for substring matches, but n is only ~4k searchable airports
// Thread-safe: uses RLock for concurrent read access
func searchAirports(query string, maxResults int) []Airport {
	if query == "" {
		return nil
	}

	query = strings.ToLower(query)
	seen := make(map[string]bool) // Track ICAO codes we've already added
	var results []Airport

	// Priority 1: Exact ICAO match (O(1) map lookup)
	// Note: lookupAirport may promote small airports, but we only add to results
	// if it's medium/large (keeps autocomplete focused on major airports)
	if airport, found := airportByICAO[strings.ToUpper(query)]; found {
		if airport.Type == "large_airport" || airport.Type == "medium_airport" {
			results = append(results, airport)
			seen[airport.ICAO] = true
		}
	}

	// Priority 2: Exact IATA match (O(1) map lookup)
	if len(results) < maxResults {
		if airport, found := lookupAirportByIATA(query); found {
			if !seen[airport.ICAO] {
				results = append(results, airport)
				seen[airport.ICAO] = true
			}
		}
	}

	// Acquire read lock for accessing searchableAirports and airportsByCityWord
	searchMu.RLock()
	defer searchMu.RUnlock()

	// Priority 3: Exact city word match (O(1) map lookup)
	// e.g., "toronto" finds CYYZ, CYTZ directly
	if len(results) < maxResults {
		if matches := airportsByCityWord[query]; len(matches) > 0 {
			for _, sa := range matches {
				if len(results) >= maxResults {
					break
				}
				if !seen[sa.ICAO] {
					results = append(results, sa.Airport)
					seen[sa.ICAO] = true
				}
			}
		}
	}

	// Priority 4: Substring search through pre-filtered searchable airports
	// Only ~4k airports to scan, with pre-computed lowercase strings
	// Includes any dynamically promoted small airports
	if len(results) < maxResults {
		for i := range searchableAirports {
			if len(results) >= maxResults {
				break
			}
			sa := &searchableAirports[i]

			// Skip if already added
			if seen[sa.ICAO] {
				continue
			}

			// Check substring matches (no allocation - using pre-computed lowercase)
			if strings.Contains(sa.icaoLower, query) ||
				strings.Contains(sa.iataLower, query) ||
				strings.Contains(sa.nameLower, query) ||
				strings.Contains(sa.cityLower, query) {
				results = append(results, sa.Airport)
				seen[sa.ICAO] = true
			}
		}
	}

	return results
}

// airportToBoundingBox creates a bounding box around an airport
// radius is in degrees (roughly: 1 degree ≈ 111 km at equator)
func airportToBoundingBox(airport Airport, radius float64) BoundingBox {
	return BoundingBox{
		LatMin: airport.Lat - radius,
		LatMax: airport.Lat + radius,
		LonMin: airport.Lng - radius,
		LonMax: airport.Lng + radius,
	}
}
