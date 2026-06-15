package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yxorp/internal/config"
	"github.com/yxorp/internal/degradation"
	"github.com/yxorp/internal/metrics"
	"github.com/yxorp/internal/middleware"
	"github.com/yxorp/internal/proxy"
	"github.com/yxorp/internal/rules"
	"github.com/yxorp/internal/server"
	"github.com/yxorp/internal/stats"
	"github.com/yxorp/pkg/logger"
)

var initE2EMetricsOnce sync.Once

func initE2ETestMetrics() {
	initE2EMetricsOnce.Do(func() {
		defer func() {
			_ = recover() // Ignore duplicate registration panics
		}()
		metrics.Init()
	})
}

func TestE2EIntegration(t *testing.T) {
	logger.Init()
	initE2ETestMetrics()

	// 1. Spin up 3 mock backend servers
	backends := make([]*httptest.Server, 3)
	backendURLs := make([]string, 3)
	var backendHits [3]int64
	var backendResponseCode int32 = http.StatusOK

	for i := 0; i < 3; i++ {
		idx := i
		backends[idx] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&backendHits[idx], 1)

			code := int(atomic.LoadInt32(&backendResponseCode))
			if code != http.StatusOK {
				w.WriteHeader(code)
				w.Write([]byte("backend failure response"))
				return
			}

			if r.URL.Path == "/health" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
				return
			}

			response := map[string]interface{}{
				"server":    fmt.Sprintf("Backend Server %d", idx+1),
				"timestamp": time.Now().Format(time.RFC3339),
				"method":    r.Method,
				"path":      r.URL.Path,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		backendURLs[idx] = backends[idx].URL
	}
	defer func() {
		for _, b := range backends {
			b.Close()
		}
	}()

	// 2. Setup configuration
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:         "18080",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
			APIKey:       "e2e-secret-key",
		},
		Proxy: config.ProxyConfig{
			Targets:        backendURLs,
			MaxRequestSize: 1024,
		},
		Security: config.SecurityConfig{
			MaxBodySize:         2048,
			MaxDecompressedSize: 4096,
			BlockUserAgents:     []string{"curl/7.0"},
			RateLimit: config.RateLimitConfig{
				Enabled:           true,
				RequestsPerMinute: 50,
			},
			Rules: []config.SecurityRule{
				{Name: "sqli", Pattern: "(?i)(UNION\\s+SELECT|DROP\\s+TABLE)", Location: "query_params"},
			},
		},
	}

	// 3. Initialize components
	degradationMgr := degradation.NewManager()
	cfgManager := config.NewManager(cfg)

	rp, err := proxy.NewLoadBalancer(cfg.Proxy.Targets, cfg.Proxy.MaxRequestSize)
	if err != nil {
		t.Fatalf("failed to create load balancer: %v", err)
	}
	defer rp.Stop()

	ruleEngine, err := rules.NewEngine(cfg.Security.Rules)
	if err != nil {
		t.Fatalf("failed to create rule engine: %v", err)
	}

	var engineMu sync.RWMutex
	currentEngine := ruleEngine

	rateLimiter := middleware.NewRateLimiter(cfg.Security.RateLimit)
	defer rateLimiter.Stop()

	// 4. Build WAF middleware chain
	finalHandler := middleware.Chain(
		rp,
		middleware.RecoveryMiddleware,
		middleware.DegradationMiddleware(degradationMgr),
		middleware.RequestIDMiddleware(),
		middleware.SecureHeadersMiddleware(),
		middleware.GzipMiddleware(),
		middleware.MetricsMiddleware,
		rateLimiter.Middleware,
		middleware.SecurityMiddleware(
			func() config.SecurityConfig { return cfgManager.Get().Security },
			func() *rules.Engine {
				engineMu.RLock()
				defer engineMu.RUnlock()
				return currentEngine
			},
		),
		middleware.RequestLogger,
	)

	// 5. Start WAF Server
	srv := server.NewServer(cfg.Server, finalHandler)
	go func() {
		_ = srv.Start()
	}()
	defer srv.Shutdown(context.Background())

	// 6. Start isolated Admin/Metrics Server
	adminMux := http.NewServeMux()
	apiAuth := middleware.APIAuthMiddleware(cfg.Server.APIKey)

	adminMux.Handle("/metrics", metrics.Handler())
	adminMux.Handle("/api/logs", apiAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats.GetRecentLogs())
	})))
	adminMux.Handle("/api/stats", apiAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats.GetSystemStats())
	})))
	adminMux.Handle("/api/rules", apiAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfgManager.Get().Security.Rules)
	})))
	adminMux.Handle("/api/backends", apiAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rp.GetBackendMetrics())
	})))

	adminSrv := &http.Server{
		Addr:    ":18081",
		Handler: adminMux,
	}
	go func() {
		_ = adminSrv.ListenAndServe()
	}()
	defer adminSrv.Shutdown(context.Background())

	// Give servers a moment to start
	time.Sleep(150 * time.Millisecond)

	client := &http.Client{}

	// --- TEST CASE 1: Load Balancing ---
	t.Run("Load Balancing (Round-Robin)", func(t *testing.T) {
		// Reset hits
		for i := 0; i < 3; i++ {
			atomic.StoreInt64(&backendHits[i], 0)
		}

		for i := 0; i < 6; i++ {
			req, _ := http.NewRequest("GET", "http://localhost:18080/", nil)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected status 200, got %d", resp.StatusCode)
			}
		}

		// Verify round-robin: each backend should be hit exactly 2 times
		for idx, hits := range backendHits {
			if hits != 2 {
				t.Errorf("Expected backend %d to have 2 hits, got %d", idx+1, hits)
			}
		}
	})

	// --- TEST CASE 2: Health Checks / Active Failover ---
	t.Run("Health Checks & Active Failover", func(t *testing.T) {
		// Mock mark backend 1 as offline
		backendsList := rp.GetBackends()
		backendsList[0].SetAlive(false)

		// Reset hits
		for i := 0; i < 3; i++ {
			atomic.StoreInt64(&backendHits[i], 0)
		}

		// Make 4 requests
		for i := 0; i < 4; i++ {
			req, _ := http.NewRequest("GET", "http://localhost:18080/", nil)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			resp.Body.Close()
		}

		// Backend 1 should have 0 hits, backend 2 and 3 should share the hits (2 each)
		if hits := atomic.LoadInt64(&backendHits[0]); hits != 0 {
			t.Errorf("Expected 0 hits on backend 1 (offline), got %d", hits)
		}
		if hits := atomic.LoadInt64(&backendHits[1]); hits != 2 {
			t.Errorf("Expected 2 hits on backend 2, got %d", hits)
		}
		if hits := atomic.LoadInt64(&backendHits[2]); hits != 2 {
			t.Errorf("Expected 2 hits on backend 3, got %d", hits)
		}

		// Restore backend 1
		backendsList[0].SetAlive(true)
	})

	// --- TEST CASE 3: WAF Block (SQL Injection) ---
	t.Run("WAF SQL Injection Prevention", func(t *testing.T) {
		maliciousURL := "http://localhost:18080/api/users?id=1%20UNION%20SELECT%20*%20FROM%20users"
		req, _ := http.NewRequest("GET", maliciousURL, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden on SQLi payload, got %d", resp.StatusCode)
		}
	})

	// --- TEST CASE 5: User-Agent Blocking ---
	t.Run("User-Agent Blocking", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "http://localhost:18080/", nil)
		req.Header.Set("User-Agent", "curl/7.0")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden for blocked User-Agent, got %d", resp.StatusCode)
		}
	})

	// --- TEST CASE 6: Security Headers ---
	t.Run("Security Headers", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "http://localhost:18080/", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		expectedHeaders := map[string]string{
			"X-Content-Type-Options":    "nosniff",
			"X-Frame-Options":           "DENY",
			"X-XSS-Protection":          "1; mode=block",
			"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		}

		for k, v := range expectedHeaders {
			if got := resp.Header.Get(k); got != v {
				t.Errorf("Expected header %s: %q, got %q", k, v, got)
			}
		}

		if resp.Header.Get("X-Request-ID") == "" {
			t.Error("Expected X-Request-ID header to be set")
		}
	})

	// --- TEST CASE 7: Gzip Compression ---
	t.Run("Gzip Compression", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "http://localhost:18080/", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if encoding := resp.Header.Get("Content-Encoding"); encoding != "gzip" {
			t.Errorf("Expected Content-Encoding: gzip, got %q", encoding)
		}

		// Verify we can decompress it
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			t.Fatalf("Failed to create gzip reader: %v", err)
		}
		defer gr.Close()

		bodyBytes, err := io.ReadAll(gr)
		if err != nil {
			t.Fatalf("Failed to read decompressed response: %v", err)
		}

		if !strings.Contains(string(bodyBytes), "Backend Server") {
			t.Errorf("Response body does not contain expected text, got: %q", string(bodyBytes))
		}
	})

	// --- TEST CASE 8: Circuit Breaker ---
	t.Run("Circuit Breaker Tripping", func(t *testing.T) {
		// Set backend to return 500
		atomic.StoreInt32(&backendResponseCode, http.StatusInternalServerError)

		// Make 15 requests to trip circuit breaker on all 3 backends (threshold = 5 per backend)
		for i := 0; i < 15; i++ {
			req, _ := http.NewRequest("GET", "http://localhost:18080/", nil)
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
			}
		}

		// 16th request should fail instantly with 503 Service Unavailable (Circuit Open)
		req, _ := http.NewRequest("GET", "http://localhost:18080/", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("Expected 503 Service Unavailable from open circuit, got %d", resp.StatusCode)
		}

		// Restore backend
		atomic.StoreInt32(&backendResponseCode, http.StatusOK)
		
		// Manually reset CB state for subsequent tests
		backendsList := rp.GetBackends()
		for _, b := range backendsList {
			b.CB.RecordSuccess() // transition back to closed
		}
	})

	// --- TEST CASE 9: Dashboard Admin API & Metrics ---
	t.Run("Dashboard Admin API & Metrics", func(t *testing.T) {
		endpoints := []string{"stats", "rules", "backends", "logs"}
		
		for _, ep := range endpoints {
			req, _ := http.NewRequest("GET", fmt.Sprintf("http://localhost:18081/api/%s", ep), nil)
			req.Header.Set("Authorization", "Bearer e2e-secret-key")
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Admin endpoint /api/%s request failed: %v", ep, err)
			}
			resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected 200 OK for /api/%s, got %d", ep, resp.StatusCode)
			}
		}

		// Check prometheus metrics
		req, _ := http.NewRequest("GET", "http://localhost:18081/metrics", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Metrics endpoint request failed: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK for /metrics, got %d", resp.StatusCode)
		}
		if !strings.Contains(string(body), "http_requests_total") {
			t.Error("Expected prometheus metrics format to contain http_requests_total")
		}
	})

	// --- TEST CASE 10: Request Tracing ---
	t.Run("Request Tracing", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "http://localhost:18080/", nil)
		req.Header.Set("X-Request-ID", "custom-trace-id-999")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		if traceID := resp.Header.Get("X-Request-ID"); traceID != "custom-trace-id-999" {
			t.Errorf("Expected trace ID 'custom-trace-id-999', got %q", traceID)
		}
	})

	// --- TEST CASE 4: Rate Limiting ---
	t.Run("Rate Limiting (Token Bucket)", func(t *testing.T) {
		// Rate limit is 50 RPM (refill is slow). Max burst is 50.
		// We have already consumed ~25 tokens in other tests.
		// So we send 50 requests: some of the early ones will succeed,
		// but by the end we are guaranteed to exceed the 50 token limit.
		var got429 bool
		for i := 0; i < 50; i++ {
			req, _ := http.NewRequest("GET", "http://localhost:18080/", nil)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request %d failed: %v", i, err)
			}
			if resp.StatusCode == http.StatusTooManyRequests {
				got429 = true
			}
			resp.Body.Close()
		}

		if !got429 {
			t.Error("Expected to get at least one 429 Too Many Requests response, but got none")
		}
	})
}
