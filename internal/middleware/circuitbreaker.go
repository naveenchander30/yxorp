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

func (cb *CircuitBreaker) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cb.mu.Lock()
		if cb.state == StateOpen {
			if time.Since(cb.lastFailure) > cb.resetTimeout {
				cb.state = StateHalfOpen
				logger.Info("Circuit Breaker entering Half-Open state")
			} else {
				cb.mu.Unlock()
				http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
				return
			}
		} else if cb.state == StateHalfOpen {
			// Allow only one request in Half-Open (simplified: we don't strictly enforce "one" concurrent request here,
			// but the first failure will trip it back, and success will close it.
			// For strict single-flight, we'd need more complex logic, but this suffices for a basic CB).
		}
		cb.mu.Unlock()

		// Capture status code
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)

		cb.mu.Lock()
		defer cb.mu.Unlock()

		if rw.statusCode >= 500 {
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
		} else {
			// Success (or 4xx client error)
			if cb.state == StateHalfOpen {
				cb.state = StateClosed
				cb.failures = 0
				logger.Info("Circuit Breaker recovered. Closed.")
			} else if cb.state == StateClosed {
				// Optional: Reset failures on success?
				// Usually we reset only after a period or consecutive successes.
				// For simplicity, let's reset on any success to avoid accumulating failures over days.
				cb.failures = 0
			}
		}
	})
}
