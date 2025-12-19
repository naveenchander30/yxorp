package proxy

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yxorp/pkg/logger"
)

type Backend struct {
	URL   *url.URL
	Proxy *httputil.ReverseProxy
	Alive bool
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
		
		// Ensure response headers (including Set-Cookie) are forwarded
		proxy.ModifyResponse = func(resp *http.Response) error {
			return nil
		}

		backends = append(backends, &Backend{
			URL:   target,
			Proxy: proxy,
			Alive: true,
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
		if lb.backends[idx].IsAlive() {
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
		peer.Proxy.ServeHTTP(w, r)
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
