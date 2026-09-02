package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "opensky-go-api/docs"
)

// =============================================================================
// MAIN - Application entry point
// =============================================================================

// @title OpenSky Go API
// @version 1.0
// @description Flight tracking API with airport search, caching, and rate limiting.
// @BasePath /

func getCORSAllowedOrigins() []string {
	allowedOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
	if allowedOrigins == "" {
		return []string{"http://localhost:5173"}
	}

	parts := strings.Split(allowedOrigins, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		origin := strings.TrimSpace(p)
		if origin != "" {
			origins = append(origins, origin)
		}
	}

	if len(origins) == 0 {
		return []string{"http://localhost:5173"}
	}

	return origins
}

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

	allowedOrigins := getCORSAllowedOrigins()
	router.Use(cors.New(cors.Config{
		AllowOrigins: allowedOrigins,
		AllowMethods: []string{"GET", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
		MaxAge:       12 * time.Hour,
	}))
	fmt.Printf("✅ CORS enabled for origins: %s\n", strings.Join(allowedOrigins, ", "))

	// Apply rate limiting middleware globally
	// 10 requests/second per IP with burst of 20
	router.Use(RateLimitMiddleware(defaultRateLimiter))
	fmt.Println("✅ Rate limiting enabled (10 req/s per IP)")

	// Swagger docs
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// System endpoints
	router.GET("/ping", getPing)
	router.GET("/cache/stats", getCacheStats)
	if os.Getenv("APP_ENV") != "production" {
		router.GET("/debug/cors", getCORSDebug)
		router.GET("/debug/upstream", getUpstreamDebug)
		fmt.Println("✅ CORS debug endpoint enabled: /debug/cors")
		fmt.Println("✅ Upstream debug endpoint enabled: /debug/upstream")
	}

	// Flight endpoints (real-time)
	router.GET("/flights/area", getFlightsByArea)
	router.GET("/flights/airport/:icao", getFlightsByAirport) // Flights around airport

	// Airport endpoints
	router.GET("/airports", searchAirportsHandler)  // Search/autocomplete
	router.GET("/airports/:icao", getAirportByICAO) // Get airport details by ICAO

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("🛫 OpenSky API server starting on port %s\n", port)
	if err := router.Run(":" + port); err != nil {
		fmt.Printf("server failed to start: %v\n", err)
		os.Exit(1)
	}
}
