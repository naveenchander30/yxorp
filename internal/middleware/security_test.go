package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yxorp/internal/config"
	"github.com/yxorp/internal/rules"
)

func TestSecurityMiddleware(t *testing.T) {
	cfg := config.SecurityConfig{
		BlockUserAgents: []string{"curl", "wget"},
	}

	// Initialize empty engine for basic test
	engine, _ := rules.NewEngine(nil)

	middleware := SecurityMiddleware(func() config.SecurityConfig { return cfg }, func() *rules.Engine { return engine })
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name           string
		userAgent      string
		expectedStatus int
	}{
		{"Allowed User-Agent", "Mozilla/5.0", http.StatusOK},
		{"Blocked User-Agent (curl)", "curl/7.64.1", http.StatusForbidden},
		{"Blocked User-Agent (wget)", "Wget/1.20.3", http.StatusForbidden},
		{"Blocked User-Agent (case insensitive)", "CURL/7.64.1", http.StatusForbidden},
		{"Empty User-Agent (if configured to block)", "", http.StatusOK}, // Default config doesn't block empty unless specified
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("User-Agent", tt.userAgent)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}
		})
	}
}

func TestSecurityMiddleware_Rules(t *testing.T) {
	cfg := config.SecurityConfig{}
	ruleCfg := []config.SecurityRule{
		{Name: "SQLi", Pattern: "UNION SELECT", Location: "query_params"},
	}
	engine, _ := rules.NewEngine(ruleCfg)

	middleware := SecurityMiddleware(func() config.SecurityConfig { return cfg }, func() *rules.Engine { return engine })
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name           string
		url            string
		expectedStatus int
	}{
		{"Safe Request", "/?q=hello", http.StatusOK},
		{"SQLi Attack", "/?q=UNION%20SELECT", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.url, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}
		})
	}
}
