package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

// =============================================================================
// MAIN - Application entry point
// =============================================================================

func main() {
	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		fmt.Println("No .env file found - using system environment variables")
	}

	// Load airports database
	if err := loadAirports("data/airports.csv"); err != nil {
		fmt.Printf("Warning: failed to load airports: %v\n", err)
	} else {
		fmt.Printf("✅ Loaded %d airports (%d medium/large)\n", getAirportCount(), getMediumLargeAirportCount())
	}

	// Start cache cleanup routine (background goroutine)
	flightCache.StartCleanupRoutine(1 * time.Minute)
	fmt.Println("✅ Cache cleanup routine started")

	// Create router
	router := gin.Default()

	// Apply rate limiting middleware globally
	// 10 requests/second per IP with burst of 20
	router.Use(RateLimitMiddleware(defaultRateLimiter))
	fmt.Println("✅ Rate limiting enabled (10 req/s per IP)")

	// Health check (rate limited but lightweight)
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
			"status":  "healthy",
		})
	})

	// Cache stats (for debugging)
	router.GET("/cache/stats", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"flightCache": gin.H{
				"entries": flightCache.Size(),
				"ttl":     FlightCacheTTL.String(),
			},
		})
	})

	// Flight endpoints (real-time)
	router.GET("/flights", getFlights)
	router.GET("/flights/area", getFlightsByArea)
	router.GET("/flights/airport/:icao", getFlightsByAirport) // Flights around airport
	router.GET("/flights/:icao", getFlightByICAO)

	// Airport endpoints
	router.GET("/airports", searchAirportsHandler)  // Search/autocomplete
	router.GET("/airports/:icao", getAirportByICAO) // Get airport details by ICAO

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("🛫 OpenSky API server starting on port %s\n", port)
	router.Run(":" + port)
}
