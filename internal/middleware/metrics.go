package middleware

import (
	"expvar"
	"net/http"
	"strconv"
	"time"
)

var (
	// Global metrics
	totalRequests   = expvar.NewInt("requests_total")
	blockedRequests = expvar.NewInt("requests_blocked")
	totalLatency    = expvar.NewInt("latency_total_ms")
	statusCodes     = expvar.NewMap("status_codes")
)

func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)

		// Update metrics
		totalRequests.Add(1)
		totalLatency.Add(int64(time.Since(start).Milliseconds()))
		statusCodes.Add(strconv.Itoa(rw.statusCode), 1)

		if rw.statusCode == http.StatusForbidden || rw.statusCode == http.StatusTooManyRequests {
			blockedRequests.Add(1)
		}
	})
}
