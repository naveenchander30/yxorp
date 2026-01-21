package middleware

import (
	"net/http"
	"time"

	"github.com/yxorp/internal/common"
	"github.com/yxorp/internal/stats"
	"github.com/yxorp/pkg/logger"
)

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rw := common.NewResponseWriter(w)
		next.ServeHTTP(rw, r)

		latency := time.Since(start)

		action := "ALLOWED"
		if rw.StatusCode == http.StatusForbidden || rw.StatusCode == http.StatusTooManyRequests {
			action = "BLOCKED"
		}

		logger.Info("Request processed",
			"client_ip", r.RemoteAddr,
			"method", r.Method,
			"path", r.URL.Path,
			"status_code", rw.StatusCode,
			"latency", latency.String(),
			"action", action,
		)

		// Send to Dashboard Stats
		stats.AddLog(stats.LogEntry{
			Timestamp:  time.Now().Format(time.RFC3339),
			ClientIP:   r.RemoteAddr,
			Method:     r.Method,
			Path:       r.URL.Path,
			StatusCode: rw.StatusCode,
			Latency:    latency.String(),
			Action:     action,
		})
	})
}
