package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/yxorp/pkg/logger"
)

// APIAuthMiddleware provides API key authentication for admin endpoints
func APIAuthMiddleware(configAPIKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get API key from environment variable first, fall back to config
			apiKey := os.Getenv("WAF_API_KEY")
			if apiKey == "" {
				apiKey = configAPIKey
			}

			// If no API key is configured, allow all requests (insecure mode)
			if apiKey == "" {
				logger.Warn("API authentication is disabled - no API key configured")
				next.ServeHTTP(w, r)
				return
			}

			// Check Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				logger.Warn("API request without authorization header", "client_ip", r.RemoteAddr, "path", r.URL.Path)
				w.Header().Set("WWW-Authenticate", `Bearer realm="Admin API"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Support both "Bearer <token>" and raw token
			token := strings.TrimPrefix(authHeader, "Bearer ")
			token = strings.TrimSpace(token)

			if token != apiKey {
				logger.Warn("API request with invalid API key", "client_ip", r.RemoteAddr, "path", r.URL.Path)
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			// API key is valid, proceed
			next.ServeHTTP(w, r)
		})
	}
}
