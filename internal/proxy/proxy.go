package proxy

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yxorp/internal/middleware"
	"github.com/yxorp/pkg/logger"
)

type Backend struct {
	URL   *url.URL
	Proxy *httputil.ReverseProxy
	Alive bool
	CB    *middleware.CircuitBreaker
	mux   sync.RWMutex
}

func (b *Backend) SetAlive(alive bool) {
	b.mux.Lock()
	b.Alive = alive
	b.mux.Unlock()
}

func (b *Backend) IsAlive() bool {
	b.mux.RLock()
	defer b.mux.RUnlock()
	return b.Alive
}

type LoadBalancer struct {
	backends []*Backend
	current  uint64
}

func NewLoadBalancer(targets []string) (*LoadBalancer, error) {
	var backends []*Backend

	// Default CB settings: 5 failures, 30s timeout
	// In a real app, pass these as config
	cbThreshold := 5
	cbTimeout := 30 * time.Second

	for _, targetURL := range targets {
		target, err := url.Parse(targetURL)
		if err != nil {
			return nil, err
		}

		proxy := httputil.NewSingleHostReverseProxy(target)

		// Customize Director
		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)
			req.Host = target.Host
			req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
			
			// Forward all cookies and session headers
			if cookie := req.Header.Get("Cookie"); cookie != "" {
				req.Header.Set("Cookie", cookie)
			}
		}
		
		// ModifyResponse to check for 500 errors and trigger CB
		proxy.ModifyResponse = func(resp *http.Response) error {
			// This runs AFTER the request is sent to backend
			// We can't easily access the specific backend struct here to call RecordFailure
			// without complex context passing or closure.
			// However, ServeHTTP below drives this.
			// Actually, httputil.ReverseProxy doesn't return error on 500 status, it returns nil error and the response.
			// So we can handle status codes in ServeHTTP wrapper or here.
			// But since we have multiple backends, passing the CB into this closure is cleanest.
			return nil
		}

		cb := middleware.NewCircuitBreaker(cbThreshold, cbTimeout)

		backends = append(backends, &Backend{
			URL:   target,
			Proxy: proxy,
			Alive: true,
			CB:    cb,
		})
	}

	lb := &LoadBalancer{
		backends: backends,
	}

	// Start health check
	go lb.HealthCheck()

	return lb, nil
}

func (lb *LoadBalancer) NextIndex() int {
	return int(atomic.AddUint64(&lb.current, 1) % uint64(len(lb.backends)))
}

func (lb *LoadBalancer) GetNextPeer() *Backend {
	next := lb.NextIndex()
	l := len(lb.backends) + next // start from next and loop
	for i := next; i < l; i++ {
		idx := i % len(lb.backends)
		if lb.backends[idx].IsAlive() && lb.backends[idx].CB.AllowRequest() {
			if i != next {
				atomic.StoreUint64(&lb.current, uint64(idx))
			}
			return lb.backends[idx]
		}
	}
	return nil
}

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	peer := lb.GetNextPeer()
	if peer != nil {
		// Use a custom ResponseWriter to capture the status code
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		peer.Proxy.ServeHTTP(rw, r)
		
		// Update Circuit Breaker based on response
		// Note: httputil.ReverseProxy handles network errors by calling ErrorHandler (logging mostly)
		// and returning 502. We need to catch that too. 
		// Ideally we wrap the ErrorHandler but for now checking status code is a good proxy.
		if rw.statusCode >= 500 {
			peer.CB.RecordFailure()
		} else {
			peer.CB.RecordSuccess()
		}
		return
	}
	// Log which backends are down
	logger.Error("All backends unavailable")
	for _, b := range lb.backends {
		logger.Info("Backend status", "url", b.URL.String(), "alive", b.IsAlive())
	}
	http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
}

// Simple wrapper to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (lb *LoadBalancer) HealthCheck() {
	// Wait 3 seconds before first check to avoid race condition on startup
	time.Sleep(3 * time.Second)
	
	t := time.NewTicker(time.Second * 10)
	for range t.C {
		for _, b := range lb.backends {
			alive := isBackendAlive(b.URL)
			b.SetAlive(alive)
			status := "healthy"
			if !alive {
				status = "down"
				logger.Warn("Backend health check failed", "url", b.URL.String(), "status", status)
			} else {
				logger.Info("Backend health check", "url", b.URL.String(), "status", status)
			}
		}
	}
}

func isBackendAlive(u *url.URL) bool {
	timeout := 2 * time.Second
	
	// Add default port if missing
	host := u.Host
	if u.Port() == "" {
		if u.Scheme == "https" {
			host = host + ":443"
		} else if u.Scheme == "http" {
			host = host + ":80"
		}
	}
	
	conn, err := net.DialTimeout("tcp", host, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
