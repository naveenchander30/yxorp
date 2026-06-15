package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestAPIAuthMiddleware(t *testing.T) {
	// Clean environment variable before starting test
	origEnvKey := os.Getenv("WAF_API_KEY")
	defer func() {
		os.Setenv("WAF_API_KEY", origEnvKey)
	}()
	os.Unsetenv("WAF_API_KEY")

	tests := []struct {
		name           string
		envKey         string
		configKey      string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "Valid API key in environment",
			envKey:         "secret-env-key",
			configKey:      "secret-cfg-key",
			authHeader:     "Bearer secret-env-key",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Valid API key fallback to config",
			envKey:         "",
			configKey:      "secret-cfg-key",
			authHeader:     "Bearer secret-cfg-key",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Raw token (no Bearer prefix) support",
			envKey:         "secret-env-key",
			configKey:      "",
			authHeader:     "secret-env-key",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Missing auth header",
			envKey:         "secret-env-key",
			configKey:      "",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Invalid API key",
			envKey:         "secret-env-key",
			configKey:      "",
			authHeader:     "Bearer wrong-key",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "No API key configured (insecure mode)",
			envKey:         "",
			configKey:      "",
			authHeader:     "",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envKey != "" {
				os.Setenv("WAF_API_KEY", tt.envKey)
			} else {
				os.Unsetenv("WAF_API_KEY")
			}

			middleware := APIAuthMiddleware(tt.configKey)
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/api/stats", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}

			if tt.expectedStatus == http.StatusUnauthorized {
				wwwAuth := rec.Header().Get("WWW-Authenticate")
				if wwwAuth != `Bearer realm="Admin API"` {
					t.Errorf("expected WWW-Authenticate header, got %q", wwwAuth)
				}
			}
		})
	}
}
