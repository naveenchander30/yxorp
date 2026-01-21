package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		config         CORSConfig
		method         string
		origin         string
		expectedOrigin string
		expectedStatus int
		checkHeaders   map[string]string
	}{
		{
			name:           "Simple CORS request with wildcard",
			config:         DefaultCORSConfig(),
			method:         "GET",
			origin:         "https://example.com",
			expectedOrigin: "*",
			expectedStatus: http.StatusOK,
		},
		{
			name: "CORS request with specific origin",
			config: CORSConfig{
				AllowOrigins: []string{"https://example.com", "https://test.com"},
				AllowMethods: []string{"GET", "POST"},
				AllowHeaders: []string{"Content-Type"},
				MaxAge:       3600,
			},
			method:         "GET",
			origin:         "https://example.com",
			expectedOrigin: "https://example.com",
			expectedStatus: http.StatusOK,
		},
		{
			name: "CORS preflight request",
			config: CORSConfig{
				AllowOrigins: []string{"*"},
				AllowMethods: []string{"GET", "POST", "PUT"},
				AllowHeaders: []string{"Content-Type", "Authorization"},
				MaxAge:       7200,
			},
			method:         "OPTIONS",
			origin:         "https://example.com",
			expectedOrigin: "*",
			expectedStatus: http.StatusNoContent,
			checkHeaders: map[string]string{
				"Access-Control-Allow-Methods": "GET, POST, PUT",
				"Access-Control-Allow-Headers": "Content-Type, Authorization",
				"Access-Control-Max-Age":       "7200",
			},
		},
		{
			name: "CORS with credentials",
			config: CORSConfig{
				AllowOrigins:     []string{"https://example.com"},
				AllowMethods:     []string{"GET"},
				AllowCredentials: true,
			},
			method:         "GET",
			origin:         "https://example.com",
			expectedOrigin: "https://example.com",
			expectedStatus: http.StatusOK,
			checkHeaders: map[string]string{
				"Access-Control-Allow-Credentials": "true",
			},
		},
		{
			name: "CORS with exposed headers",
			config: CORSConfig{
				AllowOrigins:  []string{"*"},
				ExposeHeaders: []string{"X-Request-ID", "X-Custom-Header"},
			},
			method:         "GET",
			origin:         "https://example.com",
			expectedOrigin: "*",
			expectedStatus: http.StatusOK,
			checkHeaders: map[string]string{
				"Access-Control-Expose-Headers": "X-Request-ID, X-Custom-Header",
			},
		},
		{
			name: "CORS request with disallowed origin",
			config: CORSConfig{
				AllowOrigins: []string{"https://allowed.com"},
				AllowMethods: []string{"GET"},
			},
			method:         "GET",
			origin:         "https://disallowed.com",
			expectedOrigin: "",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := CORSMiddleware(tt.config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			}))

			req := httptest.NewRequest(tt.method, "/test", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rec.Code)
			}

			if tt.expectedOrigin != "" {
				origin := rec.Header().Get("Access-Control-Allow-Origin")
				if origin != tt.expectedOrigin {
					t.Errorf("Expected Access-Control-Allow-Origin %q, got %q", tt.expectedOrigin, origin)
				}
			}

			for header, expectedValue := range tt.checkHeaders {
				actualValue := rec.Header().Get(header)
				if actualValue != expectedValue {
					t.Errorf("Expected %s header %q, got %q", header, expectedValue, actualValue)
				}
			}
		})
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	t.Run("Generates new request ID when not provided", func(t *testing.T) {
		handler := RequestIDMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		requestID := rec.Header().Get("X-Request-ID")
		if requestID == "" {
			t.Error("Expected X-Request-ID header to be set")
		}

		if len(requestID) != 32 { // 16 bytes hex encoded = 32 chars
			t.Errorf("Expected request ID length 32, got %d", len(requestID))
		}
	})

	t.Run("Preserves existing request ID", func(t *testing.T) {
		existingID := "existing-request-id-123"
		handler := RequestIDMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Request-ID", existingID)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		requestID := rec.Header().Get("X-Request-ID")
		if requestID != existingID {
			t.Errorf("Expected request ID %q, got %q", existingID, requestID)
		}
	})
}

func TestSecureHeadersMiddleware(t *testing.T) {
	handler := SecureHeadersMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	expectedHeaders := map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"X-XSS-Protection":          "1; mode=block",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
	}

	for header, expectedValue := range expectedHeaders {
		actualValue := rec.Header().Get(header)
		if actualValue != expectedValue {
			t.Errorf("Expected %s header %q, got %q", header, expectedValue, actualValue)
		}
	}
}

func TestDefaultCORSConfig(t *testing.T) {
	config := DefaultCORSConfig()

	if len(config.AllowOrigins) != 1 || config.AllowOrigins[0] != "*" {
		t.Errorf("Expected AllowOrigins to be [*], got %v", config.AllowOrigins)
	}

	expectedMethods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	if len(config.AllowMethods) != len(expectedMethods) {
		t.Errorf("Expected %d methods, got %d", len(expectedMethods), len(config.AllowMethods))
	}

	if config.MaxAge != 3600 {
		t.Errorf("Expected MaxAge 3600, got %d", config.MaxAge)
	}

	if config.AllowCredentials {
		t.Error("Expected AllowCredentials to be false")
	}
}
