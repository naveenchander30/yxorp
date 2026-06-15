package common

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResponseWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := NewResponseWriter(rec)

	// 1. Check default status code
	if rw.StatusCode != http.StatusOK {
		t.Errorf("Expected default status %d, got %d", http.StatusOK, rw.StatusCode)
	}

	// 2. WriteHeader and check captured value
	rw.WriteHeader(http.StatusTeapot)
	if rw.StatusCode != http.StatusTeapot {
		t.Errorf("Expected captured status %d, got %d", http.StatusTeapot, rw.StatusCode)
	}

	// 3. Verify it was written to the underlying recorder
	if rec.Code != http.StatusTeapot {
		t.Errorf("Expected recorder status %d, got %d", http.StatusTeapot, rec.Code)
	}

	// 4. Test Write method forwards data
	payload := []byte("hello world")
	n, err := rw.Write(payload)
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if n != len(payload) {
		t.Errorf("expected to write %d bytes, wrote %d", len(payload), n)
	}

	if rec.Body.String() != "hello world" {
		t.Errorf("expected body 'hello world', got %q", rec.Body.String())
	}
}
