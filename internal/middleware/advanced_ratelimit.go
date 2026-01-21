package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/yxorp/internal/config"
	"github.com/yxorp/internal/degradation"
	"github.com/yxorp/internal/metrics"
	"github.com/yxorp/pkg/logger"
)

// RateLimitStrategy defines different rate limiting strategies
type RateLimitStrategy int

const (
	// StrategyTokenBucket uses token bucket algorithm (default)
	StrategyTokenBucket RateLimitStrategy = iota
	// StrategySlidingWindow uses sliding window algorithm
	StrategySlidingWindow
	// StrategyFixedWindow uses fixed window algorithm
	StrategyFixedWindow
)

// ClientInfo stores rate limiting information for a client
type ClientInfo struct {
	tokens       float64
	lastUpdate   time.Time
	requestCount int
	windowStart  time.Time
	requests     []time.Time // For sliding window
}

// AdvancedRateLimiter provides sophisticated rate limiting with multiple strategies
type AdvancedRateLimiter struct {
	mu              sync.RWMutex
	clients         map[string]*ClientInfo
	rate            float64 // tokens per second (for token bucket)
	burst           float64 // max tokens (for token bucket)
	windowSize      time.Duration
	maxRequests     int
	cleanupInterval time.Duration
	enabled         bool
	stopCleanup     chan struct{}
	strategy        RateLimitStrategy
	whitelist       map[string]bool
	blacklist       map[string]bool
}

// NewAdvancedRateLimiter creates a new advanced rate limiter
func NewAdvancedRateLimiter(cfg config.RateLimitConfig, strategy RateLimitStrategy) *AdvancedRateLimiter {
	// Convert requests per minute to tokens per second
	rate := float64(cfg.RequestsPerMinute) / 60.0
	if rate <= 0 {
		rate = 1 // Default to something safe if 0
	}

	rl := &AdvancedRateLimiter{
		clients:         make(map[string]*ClientInfo),
		rate:            rate,
		burst:           float64(cfg.RequestsPerMinute),
		windowSize:      time.Minute,
		maxRequests:     cfg.RequestsPerMinute,
		cleanupInterval: 10 * time.Minute,
		enabled:         cfg.Enabled,
		stopCleanup:     make(chan struct{}),
		strategy:        strategy,
		whitelist:       make(map[string]bool),
		blacklist:       make(map[string]bool),
	}

	// Background cleanup routine
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Advanced rate limiter cleanup goroutine panic recovered", "panic", r)
			}
		}()
		rl.cleanup()
	}()

	return rl
}

// Stop stops the rate limiter
func (rl *AdvancedRateLimiter) Stop() {
	close(rl.stopCleanup)
}

// AddToWhitelist adds an IP to the whitelist
func (rl *AdvancedRateLimiter) AddToWhitelist(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.whitelist[ip] = true
}

// AddToBlacklist adds an IP to the blacklist
func (rl *AdvancedRateLimiter) AddToBlacklist(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.blacklist[ip] = true
}

// RemoveFromWhitelist removes an IP from the whitelist
func (rl *AdvancedRateLimiter) RemoveFromWhitelist(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.whitelist, ip)
}

// RemoveFromBlacklist removes an IP from the blacklist
func (rl *AdvancedRateLimiter) RemoveFromBlacklist(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.blacklist, ip)
}

func (rl *AdvancedRateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			// Remove clients that haven't been seen for a while (e.g., 1 hour)
			expiry := time.Now().Add(-1 * time.Hour)
			for ip, client := range rl.clients {
				if client.lastUpdate.Before(expiry) {
					delete(rl.clients, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.stopCleanup:
			return
		}
	}
}

func (rl *AdvancedRateLimiter) getClientIP(r *http.Request) string {
	// Check X-Forwarded-For first
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// XFF can contain multiple IPs, the first one is the client
		ips := strings.Split(xff, ",")
		clientIP := strings.TrimSpace(ips[0])
		if clientIP != "" && net.ParseIP(clientIP) != nil {
			return clientIP
		}
	}

	// Check X-Real-IP
	xri := r.Header.Get("X-Real-IP")
	if xri != "" && net.ParseIP(xri) != nil {
		return xri
	}

	// Fallback to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// Middleware returns the rate limiting middleware
func (rl *AdvancedRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Check if rate limiting should be bypassed due to degradation
		if mgr := r.Context().Value("degradation_manager"); mgr != nil {
			if dm, ok := mgr.(*degradation.Manager); ok && dm.ShouldBypassRateLimit() {
				logger.Warn("Rate limiting bypassed due to degradation", "client_ip", r.RemoteAddr)
				next.ServeHTTP(w, r)
				return
			}
		}

		ip := rl.getClientIP(r)

		// Check whitelist
		rl.mu.RLock()
		if rl.whitelist[ip] {
			rl.mu.RUnlock()
			next.ServeHTTP(w, r)
			return
		}

		// Check blacklist
		if rl.blacklist[ip] {
			rl.mu.RUnlock()
			logger.Warn("Request from blacklisted IP", "client_ip", ip)
			metrics.RecordRateLimitExceeded(ip)
			w.Header().Set("Retry-After", "3600") // 1 hour
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		rl.mu.RUnlock()

		// Apply rate limiting based on strategy
		allowed := false
		switch rl.strategy {
		case StrategyTokenBucket:
			allowed = rl.checkTokenBucket(ip)
		case StrategySlidingWindow:
			allowed = rl.checkSlidingWindow(ip)
		case StrategyFixedWindow:
			allowed = rl.checkFixedWindow(ip)
		default:
			allowed = rl.checkTokenBucket(ip)
		}

		if allowed {
			next.ServeHTTP(w, r)
		} else {
			logger.Warn("Rate limit exceeded", "client_ip", ip, "strategy", rl.strategy)
			metrics.RecordRateLimitExceeded(ip)
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		}
	})
}

func (rl *AdvancedRateLimiter) checkTokenBucket(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	client, exists := rl.clients[ip]
	if !exists {
		client = &ClientInfo{
			tokens:     rl.burst,
			lastUpdate: time.Now(),
		}
		rl.clients[ip] = client
	}

	now := time.Now()
	elapsed := now.Sub(client.lastUpdate).Seconds()

	// Refill tokens
	client.tokens += elapsed * rl.rate
	if client.tokens > rl.burst {
		client.tokens = rl.burst
	}
	client.lastUpdate = now

	if client.tokens >= 1.0 {
		client.tokens -= 1.0
		return true
	}
	return false
}

func (rl *AdvancedRateLimiter) checkSlidingWindow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	client, exists := rl.clients[ip]
	if !exists {
		client = &ClientInfo{
			requests:   make([]time.Time, 0),
			lastUpdate: time.Now(),
		}
		rl.clients[ip] = client
	}

	now := time.Now()
	windowStart := now.Add(-rl.windowSize)

	// Remove old requests outside the window
	newRequests := make([]time.Time, 0)
	for _, reqTime := range client.requests {
		if reqTime.After(windowStart) {
			newRequests = append(newRequests, reqTime)
		}
	}
	client.requests = newRequests
	client.lastUpdate = now

	// Check if we can accept this request
	if len(client.requests) < rl.maxRequests {
		client.requests = append(client.requests, now)
		return true
	}
	return false
}

func (rl *AdvancedRateLimiter) checkFixedWindow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	client, exists := rl.clients[ip]
	if !exists {
		client = &ClientInfo{
			requestCount: 0,
			windowStart:  time.Now(),
			lastUpdate:   time.Now(),
		}
		rl.clients[ip] = client
	}

	now := time.Now()

	// Check if we need to reset the window
	if now.Sub(client.windowStart) >= rl.windowSize {
		client.requestCount = 0
		client.windowStart = now
	}

	client.lastUpdate = now

	// Check if we can accept this request
	if client.requestCount < rl.maxRequests {
		client.requestCount++
		return true
	}
	return false
}
