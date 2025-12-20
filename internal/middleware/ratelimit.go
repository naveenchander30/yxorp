package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/yxorp/internal/config"
	"github.com/yxorp/pkg/logger"
)

type Client struct {
	tokens     float64
	lastUpdate time.Time
}

type RateLimiter struct {
	mu              sync.Mutex
	clients         map[string]*Client
	rate            float64 // tokens per second
	burst           float64 // max tokens
	cleanupInterval time.Duration
}

func NewRateLimiter(cfg config.RateLimitConfig) *RateLimiter {
	// Convert requests per minute to tokens per second
	rate := float64(cfg.RequestsPerMinute) / 60.0
	if rate <= 0 {
		rate = 1 // Default to something safe if 0
	}

	rl := &RateLimiter{
		clients:         make(map[string]*Client),
		rate:            rate,
		burst:           float64(cfg.RequestsPerMinute), // Burst size = 1 minute worth of requests
		cleanupInterval: 10 * time.Minute,
	}

	// Background cleanup routine
	go rl.cleanup()

	return rl
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cleanupInterval)
	for range ticker.C {
		rl.mu.Lock()
		// Remove clients that haven't been seen for a while (e.g., 1 hour)
		expiry := time.Now().Add(-1 * time.Hour)
		for ip, client := range rl.clients {
			if client.lastUpdate.Before(expiry) {
				delete(rl.clients, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) getClientIP(r *http.Request) string {
	// Check X-Forwarded-For first
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// XFF can contain multiple IPs, the first one is the client
		ips := strings.Split(xff, ",")
		clientIP := strings.TrimSpace(ips[0])
		if clientIP != "" {
			return clientIP
		}
	}

	// Check X-Real-IP
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return xri
	}

	// Fallback to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := rl.getClientIP(r)

		rl.mu.Lock()
		client, exists := rl.clients[ip]
		if !exists {
			client = &Client{
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
			rl.mu.Unlock()
			next.ServeHTTP(w, r)
		} else {
			rl.mu.Unlock()
			logger.Warn("Rate limit exceeded", "client_ip", ip)
			w.Header().Set("Retry-After", "60") // Simple retry hint
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		}
	})
}
