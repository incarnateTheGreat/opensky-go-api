package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func newCORSOnlyRouter(origins []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(cors.New(cors.Config{
		AllowOrigins: origins,
		AllowOriginFunc: func(origin string) bool {
			return isOriginAllowed(origin, origins)
		},
		AllowMethods: []string{"GET", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
		MaxAge:       12 * time.Hour,
	}))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})
	return router
}

func TestIsLocalDevOrigin(t *testing.T) {
	tests := []struct {
		origin string
		want   bool
	}{
		{origin: "http://localhost:5173", want: true},
		{origin: "http://localhost:5197", want: true},
		{origin: "http://127.0.0.1:3000", want: true},
		{origin: "https://app.example.com", want: false},
		{origin: "not-a-url", want: false},
	}

	for _, tt := range tests {
		if got := isLocalDevOrigin(tt.origin); got != tt.want {
			t.Fatalf("isLocalDevOrigin(%q) = %v, want %v", tt.origin, got, tt.want)
		}
	}
}

func TestGetCORSAllowedOrigins_Default(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "")

	origins := getCORSAllowedOrigins()
	if len(origins) != 1 {
		t.Fatalf("expected 1 default origin, got %d", len(origins))
	}
	if origins[0] != "http://localhost:5173" {
		t.Fatalf("expected default origin http://localhost:5173, got %s", origins[0])
	}
}

func TestGetCORSAllowedOrigins_CommaSeparated(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", " http://localhost:5173, https://app.example.com ,, ")

	origins := getCORSAllowedOrigins()
	if len(origins) != 2 {
		t.Fatalf("expected 2 origins, got %d", len(origins))
	}
	if origins[0] != "http://localhost:5173" {
		t.Fatalf("expected first origin http://localhost:5173, got %s", origins[0])
	}
	if origins[1] != "https://app.example.com" {
		t.Fatalf("expected second origin https://app.example.com, got %s", origins[1])
	}
}

func TestCORSMiddleware_AllowsConfiguredOrigin(t *testing.T) {
	router := newCORSOnlyRouter([]string{"http://localhost:5173"})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected Access-Control-Allow-Origin to be set, got %q", got)
	}
}

func TestCORSMiddleware_BlocksUnconfiguredOrigin(t *testing.T) {
	router := newCORSOnlyRouter([]string{"http://localhost:5173"})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 for disallowed origin, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no Access-Control-Allow-Origin header for blocked origin, got %q", got)
	}
}

func TestCORSMiddleware_AllowsLocalhostDifferentPort(t *testing.T) {
	router := newCORSOnlyRouter([]string{"http://localhost:5173"})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "http://localhost:5197")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 for localhost origin, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5197" {
		t.Fatalf("expected Access-Control-Allow-Origin to reflect localhost origin, got %q", got)
	}
}

func TestCORSMiddleware_PreflightOptions(t *testing.T) {
	router := newCORSOnlyRouter([]string{"http://localhost:5173"})

	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected status 204 for preflight, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected Access-Control-Allow-Origin header on preflight, got %q", got)
	}
	allowMethods := w.Header().Get("Access-Control-Allow-Methods")
	if !strings.Contains(allowMethods, "GET") {
		t.Fatalf("expected GET in Access-Control-Allow-Methods, got %q", allowMethods)
	}
}
