package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/yxorp/pkg/logger"
)

type State int

const (
	StateClosed   State = iota // Normal operation
	StateOpen                  // Failing, blocking requests
	StateHalfOpen              // Testing recovery
)

type CircuitBreaker struct {
	mu           sync.Mutex
	state        State
	failures     int
	threshold    int
	resetTimeout time.Duration
	lastFailure  time.Time
}

func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold:    threshold,
		resetTimeout: timeout,
		state:        StateClosed,
	}
}

func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateOpen {
		if time.Since(cb.lastFailure) > cb.resetTimeout {
			cb.state = StateHalfOpen
			logger.Info("Circuit Breaker entering Half-Open state")
			return true
		}
		return false
	}
	// Closed or HalfOpen (allow probe)
	return true
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateHalfOpen {
		cb.state = StateClosed
		cb.failures = 0
		logger.Info("Circuit Breaker recovered. Closed.")
	} else if cb.state == StateClosed {
		cb.failures = 0
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateHalfOpen {
		cb.state = StateOpen
		cb.lastFailure = time.Now()
		logger.Warn("Circuit Breaker Half-Open check failed. Re-opening.")
	} else {
		cb.failures++
		if cb.failures >= cb.threshold {
			cb.state = StateOpen
			cb.lastFailure = time.Now()
			logger.Warn("Circuit Breaker tripped to Open state", "failures", cb.failures)
		}
	}
}

func (cb *CircuitBreaker) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !cb.AllowRequest() {
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}

		// Capture status code
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)

		if rw.statusCode >= 500 {
			cb.RecordFailure()
		} else {
			cb.RecordSuccess()
		}
	})
}
