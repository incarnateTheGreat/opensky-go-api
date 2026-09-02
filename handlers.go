package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// =============================================================================
// HTTP HANDLERS
// =============================================================================

// getPing handles GET /ping
// @Summary Health check
// @Description Basic health check endpoint
// @Tags system
// @Produce json
// @Success 200 {object} PingResponse
// @Router /ping [get]
func getPing(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "pong",
		"status":  "healthy",
	})
}

// getCacheStats handles GET /cache/stats
// @Summary Cache stats
// @Description Returns current in-memory cache statistics
// @Tags system
// @Produce json
// @Success 200 {object} CacheStatsResponse
// @Router /cache/stats [get]
func getCacheStats(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"flightCache": gin.H{
			"entries": flightCache.Size(),
			"ttl":     FlightCacheTTL.String(),
		},
		"upstream": getOpenSkyFailoverStats(),
	})
}

// getCORSDebug handles GET /debug/cors
// This endpoint is registered only outside production to help diagnose CORS issues.
func getCORSDebug(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"origin":                      c.GetHeader("Origin"),
		"accessControlRequestMethod":  c.GetHeader("Access-Control-Request-Method"),
		"accessControlRequestHeaders": c.GetHeader("Access-Control-Request-Headers"),
		"method":                      c.Request.Method,
		"path":                        c.Request.URL.Path,
	})
}

// getFlightsByArea handles GET /flights/area?lamin=...&lamax=...&lomin=...&lomax=...
// @Summary List flights in area
// @Description Get flights within a geographic bounding box
// @Tags flights
// @Produce json
// @Param lamin query number true "Minimum latitude"
// @Param lamax query number true "Maximum latitude"
// @Param lomin query number true "Minimum longitude"
// @Param lomax query number true "Maximum longitude"
// @Success 200 {object} FlightsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /flights/area [get]
func getFlightsByArea(c *gin.Context) {
	laminStr := c.Query("lamin")
	lamaxStr := c.Query("lamax")
	lominStr := c.Query("lomin")
	lomaxStr := c.Query("lomax")

	if laminStr == "" || lamaxStr == "" || lominStr == "" || lomaxStr == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "missing required params: lamin, lamax, lomin, lomax",
			"example": "/flights/area?lamin=45.0&lamax=47.0&lomin=-123.0&lomax=-121.0",
		})
		return
	}

	lamin, err := strconv.ParseFloat(laminStr, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "lamin must be a valid number"})
		return
	}

	lamax, err := strconv.ParseFloat(lamaxStr, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "lamax must be a valid number"})
		return
	}

	lomin, err := strconv.ParseFloat(lominStr, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "lomin must be a valid number"})
		return
	}

	lomax, err := strconv.ParseFloat(lomaxStr, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "lomax must be a valid number"})
		return
	}

	if lamin >= lamax {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "lamin must be less than lamax"})
		return
	}
	if lomin >= lomax {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "lomin must be less than lomax"})
		return
	}

	bbox := BoundingBox{
		LatMin: lamin,
		LatMax: lamax,
		LonMin: lomin,
		LonMax: lomax,
	}

	flights, timestamp, err := fetchFlightsByArea(bbox)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, FlightsResponse{
		Time:    timestamp,
		Count:   len(flights),
		Flights: flights,
	})
}

// searchAirportsHandler handles GET /airports?q=...
// Autocomplete search for airports by ICAO, IATA, name, or city
// @Summary Search airports
// @Description Search airports by ICAO, IATA, airport name, or city
// @Tags airports
// @Produce json
// @Param q query string true "Search query"
// @Param limit query int false "Max results (1-50, default 10)"
// @Success 200 {object} AirportSearchResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /airports [get]
func searchAirportsHandler(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "missing required query parameter 'q'",
			"hint":  "example: /airports?q=toronto",
		})
		return
	}

	// Parse limit (default 10, max 50)
	limit := 10
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 50 {
			limit = l
		}
	}

	airports := searchAirports(query, limit)

	if len(airports) == 0 {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("no airports found for '%s'", query),
			"hint":  "try searching by city name, airport name, or ICAO/IATA code",
		})
		return
	}

	results := make([]gin.H, len(airports))
	for i, a := range airports {
		results[i] = gin.H{
			"icao":         a.ICAO,
			"iata":         a.IATA,
			"name":         a.Name,
			"municipality": a.Municipality,
			"country":      a.Country,
			"lat":          a.Lat,
			"lng":          a.Lng,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"query":   query,
		"count":   len(airports),
		"results": results,
	})
}

// getFlightsByAirport handles GET /flights/airport/:icao
// Returns real-time flights in bounding box around an airport
// Optional query param: ?radius=0.5 (degrees, default 0.3)
// @Summary List flights near airport
// @Description Get flights near an airport using a radius-based bounding box
// @Tags flights
// @Produce json
// @Param icao path string true "Airport ICAO code"
// @Param radius query number false "Radius in degrees (0-5, default 0.3)"
// @Success 200 {object} FlightsByAirportResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /flights/airport/{icao} [get]
func getFlightsByAirport(c *gin.Context) {
	icao := strings.ToUpper(c.Param("icao"))

	airport, found := lookupAirport(icao)
	if !found {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("airport not found: %s", icao),
			"hint":  "use 4-letter ICAO code (e.g., KJFK, EGLL, CYYZ)",
		})
		return
	}

	// Parse radius (default 0.3 degrees ≈ 33km)
	radius := 0.3
	if radiusStr := c.Query("radius"); radiusStr != "" {
		if r, err := strconv.ParseFloat(radiusStr, 64); err == nil && r > 0 && r < 5 {
			radius = r
		}
	}

	bbox := airportToBoundingBox(airport, radius)

	flights, timestamp, err := fetchFlightsByArea(bbox)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"airport": gin.H{
			"icao":         airport.ICAO,
			"iata":         airport.IATA,
			"name":         airport.Name,
			"municipality": airport.Municipality,
			"country":      airport.Country,
			"lat":          airport.Lat,
			"lng":          airport.Lng,
			"radius":       radius,
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

// getAirportByICAO handles GET /airports/:icao
// Returns details for a specific airport by ICAO code
// @Summary Get airport by ICAO
// @Description Get airport details by 4-letter ICAO code
// @Tags airports
// @Produce json
// @Param icao path string true "Airport ICAO code"
// @Success 200 {object} AirportResponse
// @Failure 404 {object} ErrorResponse
// @Router /airports/{icao} [get]
func getAirportByICAO(c *gin.Context) {
	icao := strings.ToUpper(c.Param("icao"))

	airport, found := lookupAirport(icao)
	if !found {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("airport not found: %s", icao),
			"hint":  "use 4-letter ICAO code (e.g., KJFK, EGLL, CYYZ)",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"icao":         airport.ICAO,
		"iata":         airport.IATA,
		"name":         airport.Name,
		"type":         airport.Type,
		"municipality": airport.Municipality,
		"country":      airport.Country,
		"region":       airport.Region,
		"lat":          airport.Lat,
		"lng":          airport.Lng,
	})
}
