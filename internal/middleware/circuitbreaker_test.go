package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yxorp/pkg/logger"
)

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	logger.Init()

	threshold := 3
	timeout := 50 * time.Millisecond
	cb := NewCircuitBreaker(threshold, timeout)

	// 1. Initially Closed
	if cb.GetState() != StateClosed {
		t.Errorf("Expected StateClosed, got %v", cb.GetState())
	}
	if !cb.AllowRequest() {
		t.Error("Expected AllowRequest to be true in Closed state")
	}

	// 2. Record failures to trip
	cb.RecordFailure() // 1
	cb.RecordFailure() // 2
	if cb.GetState() != StateClosed {
		t.Errorf("Expected StateClosed after 2 failures, got %v", cb.GetState())
	}

	cb.RecordFailure() // 3 -> Trip
	if cb.GetState() != StateOpen {
		t.Errorf("Expected StateOpen after 3 failures, got %v", cb.GetState())
	}
	if cb.AllowRequest() {
		t.Error("Expected AllowRequest to be false in Open state")
	}

	// 3. Wait for reset timeout
	time.Sleep(60 * time.Millisecond)

	// 4. Next request should transition to Half-Open
	if !cb.AllowRequest() {
		t.Error("Expected AllowRequest to be true after reset timeout")
	}
	if cb.GetState() != StateHalfOpen {
		t.Errorf("Expected StateHalfOpen, got %v", cb.GetState())
	}

	// 5. Success in Half-Open should close
	cb.RecordSuccess()
	if cb.GetState() != StateClosed {
		t.Errorf("Expected StateClosed after success in Half-Open, got %v", cb.GetState())
	}

	// 6. Test fail in Half-Open transitions back to Open
	// Trip it first
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.GetState() != StateOpen {
		t.Fatal("Expected StateOpen")
	}

	time.Sleep(60 * time.Millisecond)

	// Transition to Half-Open
	if !cb.AllowRequest() {
		t.Fatal("Expected AllowRequest to be true")
	}

	// Failure in Half-Open
	cb.RecordFailure()
	if cb.GetState() != StateOpen {
		t.Errorf("Expected StateOpen after failure in Half-Open, got %v", cb.GetState())
	}
}

func TestCircuitBreaker_Middleware(t *testing.T) {
	threshold := 2
	timeout := 50 * time.Millisecond
	cb := NewCircuitBreaker(threshold, timeout)

	// Mock server handler
	handlerCode := http.StatusOK
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(handlerCode)
	})

	cbHandler := cb.Middleware(nextHandler)

	// Request helper
	makeReq := func() int {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		cbHandler.ServeHTTP(rec, req)
		return rec.Code
	}

	// 1. Successes
	if code := makeReq(); code != http.StatusOK {
		t.Errorf("Expected 200, got %d", code)
	}
	if code := makeReq(); code != http.StatusOK {
		t.Errorf("Expected 200, got %d", code)
	}

	// 2. Failures
	handlerCode = http.StatusInternalServerError
	if code := makeReq(); code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", code)
	}
	if code := makeReq(); code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", code)
	}

	// Now it should be Open, returning 503
	if code := makeReq(); code != http.StatusServiceUnavailable {
		t.Errorf("Expected 503 Service Unavailable, got %d", code)
	}

	// Wait for reset timeout
	time.Sleep(60 * time.Millisecond)

	// Make request (Half-Open probe)
	handlerCode = http.StatusOK // Backend recovered
	if code := makeReq(); code != http.StatusOK {
		t.Errorf("Expected 200, got %d", code)
	}

	// Circuit should be closed again
	if cb.GetState() != StateClosed {
		t.Errorf("Expected circuit to be closed, got %v", cb.GetState())
	}
}

func TestCircuitBreaker_Metrics(t *testing.T) {
	cb := NewCircuitBreaker(3, 30*time.Second)
	
	metrics := cb.GetMetrics()
	if metrics.State != "closed" {
		t.Errorf("Expected state 'closed', got %s", metrics.State)
	}
	if metrics.Threshold != 3 {
		t.Errorf("Expected threshold 3, got %d", metrics.Threshold)
	}

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	
	metrics = cb.GetMetrics()
	if metrics.State != "open" {
		t.Errorf("Expected state 'open', got %s", metrics.State)
	}
}
