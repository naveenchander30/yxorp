package proxy

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/yxorp/internal/middleware"
	"github.com/yxorp/pkg/logger"
)

func init() {
	logger.Init()
}

func TestNewLoadBalancer_Success(t *testing.T) {

	targets := []string{"localhost:3001", "http://localhost:3002"}
	lb, err := NewLoadBalancer(targets, 1024*1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer lb.Stop()

	if len(lb.backends) != 2 {
		t.Errorf("expected 2 backends, got %d", len(lb.backends))
	}

	// Verify schemes
	if lb.backends[0].URL.Scheme != "https" {
		t.Errorf("expected default scheme https, got %s", lb.backends[0].URL.Scheme)
	}
	if lb.backends[1].URL.Scheme != "http" {
		t.Errorf("expected scheme http, got %s", lb.backends[1].URL.Scheme)
	}
}

func TestNewLoadBalancer_InvalidURL(t *testing.T) {
	_, err := NewLoadBalancer([]string{"http://[::1"}, 0)
	if err == nil {
		t.Error("expected error for invalid URL target, got nil")
	}
}

func TestLoadBalancer_GetNextPeer(t *testing.T) {
	targets := []string{"http://localhost:3001", "http://localhost:3002", "http://localhost:3003"}
	lb, err := NewLoadBalancer(targets, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer lb.Stop()

	// Initially all backends healthy
	p1 := lb.GetNextPeer()
	p2 := lb.GetNextPeer()
	p3 := lb.GetNextPeer()
	p4 := lb.GetNextPeer()

	if p1.URL.Host != "localhost:3002" || p2.URL.Host != "localhost:3003" || p3.URL.Host != "localhost:3001" || p4.URL.Host != "localhost:3002" {
		t.Errorf("Expected round robin, got hosts: %s, %s, %s, %s", p1.URL.Host, p2.URL.Host, p3.URL.Host, p4.URL.Host)
	}

	// Make one backend unhealthy
	lb.backends[0].SetAlive(false) // localhost:3001 is now dead
	p5 := lb.GetNextPeer()         // Current starts at index of p4 (1 -> localhost:3002)
	p6 := lb.GetNextPeer()         // Index 2 -> localhost:3003
	p7 := lb.GetNextPeer()         // Index 0 (dead, skips to index 1 -> localhost:3002)

	if p5.URL.Host != "localhost:3003" || p6.URL.Host != "localhost:3002" || p7.URL.Host != "localhost:3003" {
		t.Errorf("Expected skip dead host, got hosts: %s, %s, %s", p5.URL.Host, p6.URL.Host, p7.URL.Host)
	}

	// Trip circuit breaker on localhost:3002 (index 1)
	for i := 0; i < 5; i++ {
		lb.backends[1].CB.RecordFailure()
	}
	if lb.backends[1].CB.GetState() != middleware.StateOpen {
		t.Fatal("Expected cb to be open")
	}

	// Now index 0 is dead, index 1 is cb open, only index 2 (localhost:3003) is allowed
	p8 := lb.GetNextPeer()
	p9 := lb.GetNextPeer()
	if p8.URL.Host != "localhost:3003" || p9.URL.Host != "localhost:3003" {
		t.Errorf("Expected only healthy non-cb-tripped hosts, got: %s, %s", p8.URL.Host, p9.URL.Host)
	}
}

func TestLoadBalancer_ServeHTTP(t *testing.T) {
	// Create a dummy backend server
	var receivedHost string
	var receivedHeaders http.Header

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHost = r.Host
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte("backend- teapot"))
	}))
	defer backend.Close()

	lb, err := NewLoadBalancer([]string{backend.URL}, 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer lb.Stop()

	// 1. Successful proxy and status code capture
	req := httptest.NewRequest("GET", "/test-route", nil)
	req.Header.Set("Cookie", "session=123")
	rec := httptest.NewRecorder()

	lb.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Errorf("Expected status 418, got %d", rec.Code)
	}
	if rec.Body.String() != "backend- teapot" {
		t.Errorf("Expected body, got %q", rec.Body.String())
	}
	if !strings.Contains(receivedHost, "127.0.0.1") {
		t.Errorf("Expected request Host header updated, got %s", receivedHost)
	}
	if receivedHeaders.Get("Cookie") != "session=123" {
		t.Error("Expected Cookie header forwarded")
	}

	// 2. Request body size limit enforcement (Content-Length too large)
	reqLarge := httptest.NewRequest("POST", "/", strings.NewReader("longer than 1024 bytes..."))
	reqLarge.ContentLength = 2000
	recLarge := httptest.NewRecorder()

	lb.ServeHTTP(recLarge, reqLarge)
	if recLarge.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected 413, got %d", recLarge.Code)
	}

	// 3. Request body size limit enforcement (MaxBytesReader read limit)
	largeBody := make([]byte, 2048)
	reqStream := httptest.NewRequest("POST", "/", bytes.NewReader(largeBody))
	recStream := httptest.NewRecorder()

	lb.ServeHTTP(recStream, reqStream)
	// Even without Content-Length, MaxBytesReader will limit reading inside reverse proxy, triggering ErrorHandler to write 413
	if recStream.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected 413 from streaming limit, got %d", recStream.Code)
	}
}

func TestLoadBalancer_AllBackendsDown(t *testing.T) {
	lb, err := NewLoadBalancer([]string{"http://localhost:54321"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer lb.Stop()

	// Mark healthy manually since no real server exists
	lb.backends[0].SetAlive(false)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	lb.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected 503 Service Unavailable, got %d", rec.Code)
	}
}

func TestIsBackendAlive(t *testing.T) {
	// 1. Alive server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	u, _ := strings.CutPrefix(server.URL, "http://")
	targetURL, _ := strings.CutSuffix(u, "/")
	parsed, _ := url.Parse("http://" + targetURL)

	if !isBackendAlive(parsed) {
		t.Error("Expected backend to be alive")
	}

	// 2. Dead server
	deadURL, _ := url.Parse("http://localhost:54321")
	if isBackendAlive(deadURL) {
		t.Error("Expected dead backend to be offline")
	}
}
