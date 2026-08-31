package main

// =============================================================================
// FILTERS - Apply query param filters to flight data
// =============================================================================

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
			continue
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
