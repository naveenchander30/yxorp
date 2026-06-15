package proxy

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yxorp/internal/common"
	"github.com/yxorp/internal/metrics"
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

// BackendInfo contains backend status information
type BackendInfo struct {
	URL          string `json:"url"`
	Alive        bool   `json:"alive"`
	CircuitOpen  bool   `json:"circuit_open"`
	FailureCount int    `json:"failure_count"`
}

type LoadBalancer struct {
	backends       []*Backend
	current        uint64
	maxRequestSize int64
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

func NewLoadBalancer(targets []string, maxRequestSize int64) (*LoadBalancer, error) {
	var backends []*Backend

	// Default CB settings: 5 failures, 30s timeout
	// In a real app, pass these as config
	cbThreshold := 5
	cbTimeout := 30 * time.Second

	ctx, cancel := context.WithCancel(context.Background())

	for _, targetURL := range targets {
		if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
			// Assume HTTPS if no scheme provided, as it's safer/more common for modern web
			targetURL = "https://" + targetURL
		}

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

		// Custom ErrorHandler to return 413 or 502/503 properly
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			logger.Error("Proxy error", "url", target.String(), "error", err)
			if err == http.ErrBodyReadAfterClose || strings.Contains(err.Error(), "request body too large") {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				w.Write([]byte("Request Entity Too Large"))
				return
			}
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte("Bad Gateway"))
		}

		// ModifyResponse to check for 500 errors and trigger CB
		proxy.ModifyResponse = func(resp *http.Response) error {
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
		backends:       backends,
		maxRequestSize: maxRequestSize,
		ctx:            ctx,
		cancel:         cancel,
	}

	// Start health check
	lb.wg.Add(1)
	go lb.HealthCheck()

	return lb, nil
}

// Stop gracefully stops the load balancer and waits for health checks to complete
func (lb *LoadBalancer) Stop() {
	if lb.cancel != nil {
		lb.cancel()
	}
	lb.wg.Wait()
}

// GetBackendMetrics returns metrics for all backends
func (lb *LoadBalancer) GetBackendMetrics() []map[string]interface{} {
	var metrics []map[string]interface{}
	for _, backend := range lb.backends {
		cbMetrics := backend.CB.GetMetrics()
		metrics = append(metrics, map[string]interface{}{
			"url":             backend.URL.String(),
			"alive":           backend.IsAlive(),
			"circuit_breaker": cbMetrics,
		})
	}
	return metrics
}

// GetBackends returns the slice of backends
func (lb *LoadBalancer) GetBackends() []*Backend {
	return lb.backends
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
	// Apply request size limit if configured
	if lb.maxRequestSize > 0 && r.ContentLength > lb.maxRequestSize {
		logger.Warn("Request body too large", "content_length", r.ContentLength, "max_size", lb.maxRequestSize, "client_ip", r.RemoteAddr)
		http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
		return
	}

	// Wrap the body with MaxBytesReader to enforce the limit even if Content-Length is missing
	if lb.maxRequestSize > 0 && r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, lb.maxRequestSize)
	}

	peer := lb.GetNextPeer()
	if peer != nil {
		// Use a custom ResponseWriter to capture the status code
		rw := common.NewResponseWriter(w)
		peer.Proxy.ServeHTTP(rw, r)

		// Record Prometheus metric for backend request
		metrics.RecordBackendRequest(peer.URL.String(), strconv.Itoa(rw.StatusCode))

		// Update Circuit Breaker based on response
		// Note: httputil.ReverseProxy handles network errors by calling ErrorHandler (logging mostly)
		// and returning 502. We need to catch that too.
		// Ideally we wrap the ErrorHandler but for now checking status code is a good proxy.
		if rw.StatusCode >= 500 {
			peer.CB.RecordFailure()
			metrics.RecordCircuitBreakerFailure(peer.URL.String())
		} else {
			peer.CB.RecordSuccess()
		}
		metrics.SetCircuitBreakerState(peer.URL.String(), peer.CB.GetState() == middleware.StateOpen)
		return
	}
	// Log which backends are down
	logger.Error("All backends unavailable")
	for _, b := range lb.backends {
		logger.Info("Backend status", "url", b.URL.String(), "alive", b.IsAlive())
	}
	http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
}

func (lb *LoadBalancer) HealthCheck() {
	defer lb.wg.Done()

	// Wait 3 seconds before first check to avoid race condition on startup
	select {
	case <-time.After(3 * time.Second):
	case <-lb.ctx.Done():
		return
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-lb.ctx.Done():
			logger.Info("Health check stopped")
			return
		case <-ticker.C:
			var wg sync.WaitGroup
			for _, b := range lb.backends {
				wg.Add(1)
				go func(backend *Backend) {
					defer func() {
						if r := recover(); r != nil {
							logger.Error("Backend health check goroutine panic recovered", "panic", r, "url", backend.URL.String())
						}
						wg.Done()
					}()
					alive := isBackendAlive(backend.URL)
					backend.SetAlive(alive)
					metrics.SetBackendHealth(backend.URL.String(), alive)
					status := "healthy"
					if !alive {
						status = "down"
						logger.Warn("Backend health check failed", "url", backend.URL.String(), "status", status)
					} else {
						logger.Info("Backend health check", "url", backend.URL.String(), "status", status)
					}
				}(b)
			}
			wg.Wait()
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
