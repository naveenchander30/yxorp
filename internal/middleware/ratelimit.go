package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/yxorp/internal/config"
	"github.com/yxorp/pkg/logger"
)

type Client struct {
	count     int
	lastReset time.Time
}

type RateLimiter struct {
	mu      sync.Mutex
	clients map[string]*Client
	limit   int
	window  time.Duration
}

func NewRateLimiter(cfg config.RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		clients: make(map[string]*Client),
		limit:   cfg.RequestsPerMinute,
		window:  time.Minute,
	}

	// Background cleanup routine to prevent memory leaks
	go rl.cleanup()

	return rl
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		rl.mu.Lock()
		for ip, client := range rl.clients {
			if time.Since(client.lastReset) > rl.window {
				delete(rl.clients, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			// If we can't parse the IP, fall back to the whole string or handle error
			ip = r.RemoteAddr
		}

		rl.mu.Lock()
		client, exists := rl.clients[ip]
		if !exists {
			client = &Client{
				count:     0,
				lastReset: time.Now(),
			}
			rl.clients[ip] = client
		}

		// Check if window has passed
		if time.Since(client.lastReset) > rl.window {
			client.count = 0
			client.lastReset = time.Now()
		}

		if client.count >= rl.limit {
			rl.mu.Unlock()
			logger.Warn("Rate limit exceeded", "client_ip", ip)
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		client.count++
		rl.mu.Unlock()

		next.ServeHTTP(w, r)
	})
}
