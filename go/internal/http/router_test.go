package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	handler := NewRouter(config.Config{AllowedOrigins: []string{"http://localhost:5173"}}, nil, nil)
	request := httptest.NewRequest(http.MethodOptions, "/api/health", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, expected %d", response.Code, http.StatusNoContent)
	}
	if actual := response.Header().Get("Access-Control-Allow-Origin"); actual != "http://localhost:5173" {
		t.Fatalf("allow origin = %q", actual)
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
