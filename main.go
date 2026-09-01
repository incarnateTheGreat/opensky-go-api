package main

import (
	"fmt"
	"net/http"
	"os"

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

	// Load OpenSky OAuth2 credentials and fetch access token
	openSkyClientID = os.Getenv("OPENSKY_CLIENT_ID")
	openSkyClientSecret = os.Getenv("OPENSKY_CLIENT_SECRET")
	if openSkyClientID != "" && openSkyClientSecret != "" {
		if err := fetchAccessToken(); err != nil {
			fmt.Printf("⚠️  Failed to get OpenSky access token: %v\n", err)
			fmt.Println("   Arrivals/departures endpoints won't work")
		}
	} else {
		fmt.Println("⚠️  OpenSky credentials not set - arrivals/departures endpoints won't work")
	}

	// Load airports database
	if err := loadAirports("data/airports.csv"); err != nil {
		fmt.Printf("Warning: failed to load airports: %v\n", err)
	} else {
		fmt.Printf("✅ Loaded %d airports (%d medium/large)\n", getAirportCount(), getMediumLargeAirportCount())
	}

	// Create router
	router := gin.Default()

	// Health check
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
			"status":  "healthy",
		})
	})

	// Flight endpoints (real-time)
	router.GET("/flights", getFlights)
	router.GET("/flights/area", getFlightsByArea)
	router.GET("/flights/airport/:icao", getFlightsByAirport) // Flights around airport
	router.GET("/flights/:icao", getFlightByICAO)

	// Airport endpoints
	router.GET("/airports", searchAirportsHandler)          // Search/autocomplete
	router.GET("/airports/:icao", getAirportByICAO)         // Get airport details by ICAO
	router.GET("/airports/:icao/arrivals", getArrivals)     // Historical arrivals
	router.GET("/airports/:icao/departures", getDepartures) // Historical departures

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("🛫 OpenSky API server starting on port %s\n", port)
	router.Run(":" + port)
}
