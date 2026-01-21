package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/yxorp/internal/common"
	"github.com/yxorp/pkg/logger"
)

type State int

const (
	StateClosed   State = iota // Normal operation
	StateOpen                  // Failing, blocking requests
	StateHalfOpen              // Testing recovery
)

type CircuitBreaker struct {
	mu            sync.Mutex
	state         State
	failures      int
	successes     int
	threshold     int
	resetTimeout  time.Duration
	lastFailure   time.Time
	totalRequests int64
	totalRejected int64
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

	cb.totalRequests++

	if cb.state == StateOpen {
		if time.Since(cb.lastFailure) > cb.resetTimeout {
			cb.state = StateHalfOpen
			logger.Info("Circuit Breaker entering Half-Open state")
			return true
		}
		cb.totalRejected++
		return false
	}
	// Closed or HalfOpen (allow probe)
	return true
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.successes++

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
		rw := common.NewResponseWriter(w)
		next.ServeHTTP(rw, r)

		if rw.StatusCode >= 500 {
			cb.RecordFailure()
		} else {
			cb.RecordSuccess()
		}
	})
}

// CircuitBreakerMetrics holds circuit breaker statistics
type CircuitBreakerMetrics struct {
	State         string `json:"state"`
	Failures      int    `json:"failures"`
	Successes     int    `json:"successes"`
	Threshold     int    `json:"threshold"`
	TotalRequests int64  `json:"total_requests"`
	TotalRejected int64  `json:"total_rejected"`
}

// GetMetrics returns the current circuit breaker metrics
func (cb *CircuitBreaker) GetMetrics() CircuitBreakerMetrics {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	stateStr := "closed"
	switch cb.state {
	case StateOpen:
		stateStr = "open"
	case StateHalfOpen:
		stateStr = "half-open"
	}

	return CircuitBreakerMetrics{
		State:         stateStr,
		Failures:      cb.failures,
		Successes:     cb.successes,
		Threshold:     cb.threshold,
		TotalRequests: cb.totalRequests,
		TotalRejected: cb.totalRejected,
	}
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}
