package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGetCORSDebug(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/debug/cors", getCORSDebug)

	req := httptest.NewRequest(http.MethodGet, "/debug/cors", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if body["origin"] != "http://localhost:5173" {
		t.Fatalf("expected origin header in debug response, got %q", body["origin"])
	}
	if body["accessControlRequestMethod"] != "GET" {
		t.Fatalf("expected Access-Control-Request-Method GET, got %q", body["accessControlRequestMethod"])
	}
	if body["path"] != "/debug/cors" {
		t.Fatalf("expected path /debug/cors, got %q", body["path"])
	}
}
