package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRecoveryMiddleware(t *testing.T) {
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("something went critically wrong")
	})

	handler := RecoveryMiddleware(panicHandler)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	// Ensure we recover from the panic and return 500
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Panic was not recovered by middleware: %v", r)
		}
	}()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}

	body := rec.Body.String()
	if body != "Internal Server Error\n" { // default http.Error suffix
		t.Errorf("expected body 'Internal Server Error\\n', got %q", body)
	}
}
