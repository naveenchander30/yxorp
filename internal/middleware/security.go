package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"

	"github.com/yxorp/internal/config"
	"github.com/yxorp/internal/degradation"
	"github.com/yxorp/internal/rules"
	"github.com/yxorp/pkg/logger"
)

func SecurityMiddleware(cfgGetter func() config.SecurityConfig, engineGetter func() *rules.Engine) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cfg := cfgGetter()
			ruleEngine := engineGetter()

			// Get degradation manager from context if available
			var degradationMgr *degradation.Manager
			if mgr := r.Context().Value("degradation_manager"); mgr != nil {
				if dm, ok := mgr.(*degradation.Manager); ok {
					degradationMgr = dm
				}
			}

			// 0. Gzip Bomb Protection
			if r.Header.Get("Content-Encoding") == "gzip" {
				maxDecompSize := cfg.MaxDecompressedSize
				if maxDecompSize <= 0 {
					maxDecompSize = 10 * 1024 * 1024 // 10MB default
				}

				// Limit the size of the compressed body to avoid decompression bombs
				r.Body = http.MaxBytesReader(w, r.Body, maxDecompSize)

				gr, err := gzip.NewReader(r.Body)
				if err != nil {
					logger.Warn("Failed to create gzip reader", "error", err)
					http.Error(w, "Bad Request", http.StatusBadRequest)
					return
				}
				defer gr.Close()

				// Read the decompressed body, but limit its size
				var decompressedBody bytes.Buffer
				_, err = io.CopyN(&decompressedBody, gr, maxDecompSize+1)
				if err != nil && err != io.EOF {
					logger.Warn("Failed to decompress request body", "error", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}

				if err == nil {
					logger.Warn("Decompressed request body too large", "client_ip", r.RemoteAddr)
					http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
					return
				}

				// Restore the body for subsequent handlers
				r.Body = io.NopCloser(bytes.NewReader(decompressedBody.Bytes()))
			}

			// 1. User-Agent Blocking
			userAgent := r.UserAgent()
			for _, blockedAgent := range cfg.BlockUserAgents {
				if blockedAgent != "" && strings.Contains(strings.ToLower(userAgent), strings.ToLower(blockedAgent)) {
					logger.Warn("Blocked suspicious User-Agent", "client_ip", r.RemoteAddr, "user_agent", userAgent)
					http.Error(w, "Forbidden", http.StatusForbidden)
					return
				}
			}

			// 2. Rule Engine Inspection
			if ruleEngine != nil {
				// Check if rule engine should be bypassed due to degradation
				if degradationMgr != nil && degradationMgr.ShouldBypassRules() {
					logger.Warn("Rule engine bypassed due to degradation", "client_ip", r.RemoteAddr)
					next.ServeHTTP(w, r)
					return
				}

				var bodyBytes []byte
				// Only read body if we have rules that might check it
				if ruleEngine.HasBodyRules() {
					maxSize := cfg.MaxBodySize
					if maxSize <= 0 {
						maxSize = 10 * 1024 * 1024 // 10MB default
					}

					// Wrap body with MaxBytesReader
					r.Body = http.MaxBytesReader(w, r.Body, maxSize)

					var err error
					bodyBytes, err = io.ReadAll(r.Body)
					if err != nil {
						if err.Error() == "http: request body too large" {
							logger.Warn("Request blocked: body too large", "client_ip", r.RemoteAddr)
							http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
							return
						}
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
