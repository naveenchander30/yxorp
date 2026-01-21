package middleware

import (
	"expvar"
	"net/http"
	"strconv"
	"time"

	"github.com/yxorp/internal/common"
	"github.com/yxorp/internal/metrics"
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
		rw := common.NewResponseWriter(w)
		next.ServeHTTP(rw, r)

		duration := time.Since(start)
		status := strconv.Itoa(rw.StatusCode)

		// Update expvar metrics (for backward compatibility)
		totalRequests.Add(1)
		totalLatency.Add(int64(duration.Milliseconds()))
		statusCodes.Add(status, 1)

		if rw.StatusCode == http.StatusForbidden || rw.StatusCode == http.StatusTooManyRequests {
			blockedRequests.Add(1)
		}

		// Update Prometheus metrics
		metrics.RecordHTTPRequest(r.Method, r.URL.Path, status, duration)
	})
}
