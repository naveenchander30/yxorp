package middleware

import (
	"net/http"
	"time"

	"github.com/yxorp/internal/stats"
	"github.com/yxorp/pkg/logger"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)

		latency := time.Since(start)

		action := "ALLOWED"
		if rw.statusCode == http.StatusForbidden || rw.statusCode == http.StatusTooManyRequests {
			action = "BLOCKED"
		}

		logger.Info("Request processed",
			"client_ip", r.RemoteAddr,
			"method", r.Method,
			"path", r.URL.Path,
			"status_code", rw.statusCode,
			"latency", latency.String(),
			"action", action,
		)

		// Send to Dashboard Stats
		stats.AddLog(stats.LogEntry{
			Timestamp:  time.Now().Format(time.RFC3339),
			ClientIP:   r.RemoteAddr,
			Method:     r.Method,
			Path:       r.URL.Path,
			StatusCode: rw.statusCode,
			Latency:    latency.String(),
			Action:     action,
		})
	})
}
