package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChainOrder(t *testing.T) {
	var executionOrder []string

	m1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			executionOrder = append(executionOrder, "m1-start")
			next.ServeHTTP(w, r)
			executionOrder = append(executionOrder, "m1-end")
		})
	}

	m2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			executionOrder = append(executionOrder, "m2-start")
			next.ServeHTTP(w, r)
			executionOrder = append(executionOrder, "m2-end")
		})
	}

	m3 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			executionOrder = append(executionOrder, "m3-start")
			next.ServeHTTP(w, r)
			executionOrder = append(executionOrder, "m3-end")
		})
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		executionOrder = append(executionOrder, "final")
		w.WriteHeader(http.StatusOK)
	})

	// When using Chain(final, m1, m2, m3), the order of application is:
	// final wrapped by m3, wrapped by m2, wrapped by m1.
	// So m1 is the outermost, and m3 is the innermost.
	// Exec order: m1-start, m2-start, m3-start, final, m3-end, m2-end, m1-end.
	handler := Chain(finalHandler, m1, m2, m3)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	expectedOrder := []string{
		"m1-start",
		"m2-start",
		"m3-start",
		"final",
		"m3-end",
		"m2-end",
		"m1-end",
	}

	if len(executionOrder) != len(expectedOrder) {
		t.Fatalf("expected order slice length %d, got %d", len(expectedOrder), len(executionOrder))
	}

	for i, v := range expectedOrder {
		if executionOrder[i] != v {
			t.Errorf("expected step %d to be %q, got %q", i, v, executionOrder[i])
		}
	}
}
