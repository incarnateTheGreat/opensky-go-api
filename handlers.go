package main

import (
	"fmt"
	"net/http"
	"strconv"
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

// getFlightsByCity handles GET /flights/city/:name
func getFlightsByCity(c *gin.Context) {
	name := c.Param("name")

	city, found := lookupCity(name)
	if !found {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("city not found: %s", name),
			"hint":  "try a major city name like 'toronto', 'london', 'tokyo'",
		})
		return
	}

	radius := 0.3
	if radiusStr := c.Query("radius"); radiusStr != "" {
		if r, err := strconv.ParseFloat(radiusStr, 64); err == nil && r > 0 && r < 5 {
			radius = r
		}
	}

	bbox := cityToBoundingBox(city, radius)

	flights, timestamp, err := fetchFlightsByArea(bbox)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"city": gin.H{
			"name":    city.Display,
			"country": city.Country,
			"lat":     city.Lat,
			"lng":     city.Lng,
			"radius":  radius,
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
