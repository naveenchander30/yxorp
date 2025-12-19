package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yxorp/pkg/logger"
)

func TestRequestLogger(t *testing.T) {
	// Initialize logger to avoid nil pointer if Init() wasn't called (though Init sets a default)
	logger.Init()

	handler := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Errorf("expected status %d, got %d", http.StatusTeapot, rec.Code)
	}

	// We can't easily assert stdout output here without capturing it,
	// but we verified the chain execution and status code propagation.
}
