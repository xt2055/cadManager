package handlers

import (
	"cadguanliq/internal/change"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChangeActiveSessionReturnsActionableConflict(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeChangeError(recorder, change.ErrActiveEditSession)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("got status %d, want 409", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "结束本地编辑") {
		t.Fatalf("missing actionable error: %s", recorder.Body.String())
	}
}
