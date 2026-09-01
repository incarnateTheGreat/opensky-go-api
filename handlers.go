package main

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// =============================================================================
// HTTP HANDLERS
// =============================================================================

// getFlights handles GET /flights
// Supports query params: ?country=US&on_ground=true
func getFlights(c *gin.Context) {
	var params FilterParams

	country := c.Query("country")
	if country != "" {
		params.Country = &country
	}

	onGroundStr := c.Query("on_ground")
	if onGroundStr != "" {
		onGround := onGroundStr == "true"
		params.OnGround = &onGround
	}

	flights, timestamp, err := fetchFlights("")
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	filtered := filterFlights(flights, params)

	c.JSON(http.StatusOK, FlightsResponse{
		Time:    timestamp,
		Count:   len(filtered),
		Flights: filtered,
	})
}

// getFlightByICAO handles GET /flights/:icao
func getFlightByICAO(c *gin.Context) {
	icao := c.Param("icao")

	flights, timestamp, err := fetchFlights(icao)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	if len(flights) == 0 {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("no flight found with ICAO24: %s", icao),
		})
		return
	}

	c.JSON(http.StatusOK, FlightsResponse{
		Time:    timestamp,
		Count:   len(flights),
		Flights: flights,
	})
}

// getFlightsByArea handles GET /flights/area?lamin=...&lamax=...&lomin=...&lomax=...
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
		"arrivals":     fmt.Sprintf("/airports/%s/arrivals", airport.ICAO),
		"departures":   fmt.Sprintf("/airports/%s/departures", airport.ICAO),
	})
}

// getArrivals handles GET /airports/:icao/arrivals
func getArrivals(c *gin.Context) {
	airport := c.Param("icao")
	begin, end := parseTimeRange(c)

	flights, err := fetchArrivals(airport, begin, end)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, HistoricalFlightsResponse{
		Airport: airport,
		Type:    "arrivals",
		Begin:   begin,
		End:     end,
		Count:   len(flights),
		Flights: flights,
	})
}

// getDepartures handles GET /airports/:icao/departures
func getDepartures(c *gin.Context) {
	airport := c.Param("icao")
	begin, end := parseTimeRange(c)

	flights, err := fetchDepartures(airport, begin, end)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, HistoricalFlightsResponse{
		Airport: airport,
		Type:    "departures",
		Begin:   begin,
		End:     end,
		Count:   len(flights),
		Flights: flights,
	})
}

// parseTimeRange extracts begin/end timestamps from query params
// Defaults to last 2 hours, max 7 days
func parseTimeRange(c *gin.Context) (begin, end int64) {
	now := time.Now().Unix()

	end = now
	begin = now - (2 * 60 * 60)

	if beginStr := c.Query("begin"); beginStr != "" {
		if b, err := strconv.ParseInt(beginStr, 10, 64); err == nil {
			begin = b
		}
	}
	if endStr := c.Query("end"); endStr != "" {
		if e, err := strconv.ParseInt(endStr, 10, 64); err == nil {
			end = e
		}
	}

	if begin >= end {
		begin = end - (2 * 60 * 60)
	}

	maxRange := int64(7 * 24 * 60 * 60)
	if end-begin > maxRange {
		begin = end - maxRange
	}

	return begin, end
}

func debugOpenSky(c *gin.Context) {
	results := gin.H{
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
		"credentialsSet": openSkyClientID != "" && openSkyClientSecret != "",
		"hasAccessToken": openSkyAccessToken != "",
	}

	// Test DNS resolution
	addrs, dnsErr := net.LookupHost("auth.opensky-network.org")
	if dnsErr != nil {
		results["dnsResolution"] = gin.H{"error": dnsErr.Error()}
	} else {
		results["dnsResolution"] = gin.H{"addresses": addrs}
	}

	// Test TCP connection
	conn, tcpErr := net.DialTimeout("tcp", "auth.opensky-network.org:443", 10*time.Second)
	if tcpErr != nil {
		results["tcpConnection"] = gin.H{"error": tcpErr.Error()}
	} else {
		conn.Close()
		results["tcpConnection"] = gin.H{"status": "success"}
	}

	// Test HTTPS request (just HEAD, no auth)
	httpClient := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("HEAD", "https://auth.opensky-network.org", nil)
	httpStart := time.Now()
	resp, httpErr := httpClient.Do(req)
	httpDuration := time.Since(httpStart)
	if httpErr != nil {
		results["httpsRequest"] = gin.H{"error": httpErr.Error(), "duration": httpDuration.String()}
	} else {
		resp.Body.Close()
		results["httpsRequest"] = gin.H{"status": resp.StatusCode, "duration": httpDuration.String()}
	}

	// Test token fetch if credentials are set
	if openSkyClientID != "" && openSkyClientSecret != "" {
		tokenStart := time.Now()
		data := fmt.Sprintf("grant_type=client_credentials&client_id=%s&client_secret=%s",
			openSkyClientID, openSkyClientSecret)
		req, _ := http.NewRequest("POST",
			"https://auth.opensky-network.org/auth/realms/opensky-network/protocol/openid-connect/token",
			strings.NewReader(data))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, tokenErr := httpClient.Do(req)
		tokenDuration := time.Since(tokenStart)
		if tokenErr != nil {
			results["tokenFetch"] = gin.H{"error": tokenErr.Error(), "duration": tokenDuration.String()}
		} else {
			resp.Body.Close()
			results["tokenFetch"] = gin.H{"status": resp.StatusCode, "duration": tokenDuration.String()}
		}
	}

	c.JSON(http.StatusOK, results)
}
