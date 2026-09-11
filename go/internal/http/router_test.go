package httpapi

	import (
		"encoding/json"
		"net/http"
		"net/http/httptest"
		"strings"
		"testing"
	
		"cadguanliq/internal/config"
	)

func TestNewRouterHealth(t *testing.T) {
	handler := NewRouter(config.Config{AllowedOrigins: []string{"*"}}, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("health status = %d, expected %d", response.Code, http.StatusOK)
	}
	var body struct {
		Code int `json:"code"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if body.Code != 0 {
		t.Fatalf("health code = %d, expected 0", body.Code)
	}
}

func TestNewRouterCORSPreflight(t *testing.T) {
	handler := NewRouter(config.Config{AllowedOrigins: []string{"http://127.0.0.1:5173"}}, nil, nil)
	request := httptest.NewRequest(http.MethodOptions, "/api/drawings/6bd3ba06-41c8-4235-a072-831be21ad12a/borrows", nil)
	request.Header.Set("Origin", "http://127.0.0.1:5173")
	request.Header.Set("Access-Control-Request-Method", "POST")
	request.Header.Set("Access-Control-Request-Headers", "content-type,authorization,idempotency-key")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, expected %d", response.Code, http.StatusNoContent)
	}
	if actual := response.Header().Get("Access-Control-Allow-Origin"); actual != "http://127.0.0.1:5173" {
		t.Fatalf("allow origin = %q", actual)
	}
	allowed := strings.ToLower(response.Header().Get("Access-Control-Allow-Headers"))
	for _, header := range []string{"content-type", "authorization", "idempotency-key"} {
		if !strings.Contains(allowed, header) {
			t.Fatalf("allow headers = %q, missing %s", allowed, header)
		}
	}
}

func TestNewRouterMethodNotAllowed(t *testing.T) {
	handler := NewRouter(config.Config{AllowedOrigins: []string{"*"}}, nil, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/health", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method status = %d, expected %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func TestNewRouterLegacyDataRouteRemoved(t *testing.T) {
	handler := NewRouter(config.Config{AllowedOrigins: []string{"*"}}, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/data/structure", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("legacy data route status = %d, expected %d", response.Code, http.StatusNotFound)
	}
}
