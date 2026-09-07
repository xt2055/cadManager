package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminConversionLogsReturnsData(t *testing.T) {
	handler := AdminConversionLogs()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/cad-conversion-logs?limit=50", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Lines []string `json:"lines"`
			Path  string   `json:"path"`
		} `json:"data"`
	}

	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if resp.Code != 0 {
		t.Fatalf("expected code 0, got %d", resp.Code)
	}
}
