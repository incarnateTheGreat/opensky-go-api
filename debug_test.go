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

func TestGetUpstreamDebug(t *testing.T) {
	primaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"time":123,"states":[]}`)); err != nil {
			t.Fatalf("failed writing response: %v", err)
		}
	}))
	defer primaryServer.Close()

	fallbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer fallbackServer.Close()

	originalBaseURL := openSkyBaseURL
	originalFallbackBaseURL := openSkyFallbackBaseURL
	defer func() {
		openSkyBaseURL = originalBaseURL
		openSkyFallbackBaseURL = originalFallbackBaseURL
	}()

	openSkyBaseURL = primaryServer.URL
	openSkyFallbackBaseURL = fallbackServer.URL

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/debug/upstream", getUpstreamDebug)

	req := httptest.NewRequest(http.MethodGet, "/debug/upstream", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if _, ok := body["probePath"]; !ok {
		t.Fatal("expected probePath in response")
	}
	if _, ok := body["timeoutMs"]; !ok {
		t.Fatal("expected timeoutMs in response")
	}

	primary, ok := body["primary"].(map[string]interface{})
	if !ok {
		t.Fatal("expected primary object in response")
	}
	fallback, ok := body["fallback"].(map[string]interface{})
	if !ok {
		t.Fatal("expected fallback object in response")
	}

	if success, ok := primary["success"].(bool); !ok || !success {
		t.Fatalf("expected primary.success=true, got %v", primary["success"])
	}
	if success, ok := fallback["success"].(bool); !ok || success {
		t.Fatalf("expected fallback.success=false, got %v", fallback["success"])
	}
}
