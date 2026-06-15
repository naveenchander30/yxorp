package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yxorp/internal/degradation"
)

func TestDegradationMiddleware(t *testing.T) {
	manager := degradation.NewManager()
	middleware := DegradationMiddleware(manager)

	var contextManager *degradation.Manager
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if val := r.Context().Value("degradation_manager"); val != nil {
			if mgr, ok := val.(*degradation.Manager); ok {
				contextManager = mgr
			}
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware(nextHandler)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	if contextManager == nil {
		t.Fatal("Expected degradation manager in context, got nil")
	}

	if contextManager != manager {
		t.Error("Expected context degradation manager to be the same instance passed to middleware")
	}
}
