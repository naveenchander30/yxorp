package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/yxorp/internal/config"
	"github.com/yxorp/internal/rules"
	"github.com/yxorp/pkg/logger"
)

func SecurityMiddleware(cfgGetter func() config.SecurityConfig, engineGetter func() *rules.Engine) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cfg := cfgGetter()
			ruleEngine := engineGetter()

			// 1. User-Agent Blocking
			userAgent := r.UserAgent()

			// Block empty User-Agent if configured (implied by "empty strings" in requirements)
			// The requirement says "Block suspicious User-Agent strings (e.g., curl, wget, empty strings)".
			if userAgent == "" {
				// Check if empty string is in the block list or if we should block it by default.
				// For now, let's assume if "empty strings" is mentioned, we should block it.
				// But let's check the config list first.
				// Actually, let's just check if the user agent contains any of the blocked strings.
			}

			for _, blockedAgent := range cfg.BlockUserAgents {
				if blockedAgent == "" && userAgent == "" {
					logger.Warn("Blocked suspicious User-Agent", "client_ip", r.RemoteAddr, "user_agent", "empty")
					http.Error(w, "Forbidden", http.StatusForbidden)
					return
				}
				if blockedAgent != "" && strings.Contains(strings.ToLower(userAgent), strings.ToLower(blockedAgent)) {
					logger.Warn("Blocked suspicious User-Agent", "client_ip", r.RemoteAddr, "user_agent", userAgent)
					http.Error(w, "Forbidden", http.StatusForbidden)
					return
				}
			}

			// 2. Rule Engine Inspection
			if ruleEngine != nil {
				var bodyBytes []byte
				// Only read body if method implies a body and we have rules that might check it
				// For simplicity, we read it if it's not GET/HEAD/DELETE/OPTIONS
				if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
					var err error
					bodyBytes, err = io.ReadAll(r.Body)
					if err != nil {
						logger.Error("Failed to read request body", "error", err)
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
						return
					}
					// Restore the body for the next handler
					r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				}

				matched, ruleName := ruleEngine.Check(r, bodyBytes)
				if matched {
					logger.Warn("Request blocked by security rule", "client_ip", r.RemoteAddr, "rule", ruleName)
					http.Error(w, "Forbidden", http.StatusForbidden)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
