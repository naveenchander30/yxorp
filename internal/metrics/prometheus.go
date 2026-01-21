package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// HTTP Request metrics
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	// WAF-specific metrics
	WAFBlockedRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "waf_blocked_requests_total",
			Help: "Total number of requests blocked by WAF",
		},
		[]string{"rule", "reason"},
	)

	// Rate limiting metrics
	RateLimitExceeded = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limit_exceeded_total",
			Help: "Total number of requests that exceeded rate limit",
		},
		[]string{"client_ip"},
	)

	// Backend metrics
	BackendRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "backend_requests_total",
			Help: "Total number of requests sent to backends",
		},
		[]string{"backend", "status"},
	)

	BackendHealthStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "backend_health_status",
			Help: "Backend health status (1 = healthy, 0 = unhealthy)",
		},
		[]string{"backend"},
	)

	// Circuit breaker metrics
	CircuitBreakerState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "circuit_breaker_state",
			Help: "Circuit breaker state (0 = closed, 1 = open)",
		},
		[]string{"backend"},
	)

	CircuitBreakerFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "circuit_breaker_failures_total",
			Help: "Total number of circuit breaker failures",
		},
		[]string{"backend"},
	)

	// System metrics
	ActiveConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "active_connections",
			Help: "Number of active connections",
		},
	)

	MemoryUsageBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "memory_usage_bytes",
			Help: "Current memory usage in bytes",
		},
	)

	// Config reload metrics
	ConfigReloads = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "config_reloads_total",
			Help: "Total number of configuration reloads",
		},
	)

	ConfigReloadFailures = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "config_reload_failures_total",
			Help: "Total number of failed configuration reloads",
		},
	)
)

// Init registers all metrics with Prometheus
func Init() {
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDuration,
		WAFBlockedRequests,
		RateLimitExceeded,
		BackendRequestsTotal,
		BackendHealthStatus,
		CircuitBreakerState,
		CircuitBreakerFailures,
		ActiveConnections,
		MemoryUsageBytes,
		ConfigReloads,
		ConfigReloadFailures,
	)
}

// Handler returns the Prometheus metrics handler
func Handler() http.Handler {
	return promhttp.Handler()
}

// RecordHTTPRequest records an HTTP request metric
func RecordHTTPRequest(method, path, status string, duration time.Duration) {
	HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
	HTTPRequestDuration.WithLabelValues(method, path, status).Observe(duration.Seconds())
}

// RecordWAFBlock records a WAF block event
func RecordWAFBlock(rule, reason string) {
	WAFBlockedRequests.WithLabelValues(rule, reason).Inc()
}

// RecordRateLimitExceeded records a rate limit exceeded event
func RecordRateLimitExceeded(clientIP string) {
	RateLimitExceeded.WithLabelValues(clientIP).Inc()
}

// RecordBackendRequest records a backend request
func RecordBackendRequest(backend, status string) {
	BackendRequestsTotal.WithLabelValues(backend, status).Inc()
}

// SetBackendHealth sets the backend health status
func SetBackendHealth(backend string, healthy bool) {
	value := 0.0
	if healthy {
		value = 1.0
	}
	BackendHealthStatus.WithLabelValues(backend).Set(value)
}

// SetCircuitBreakerState sets the circuit breaker state
func SetCircuitBreakerState(backend string, open bool) {
	value := 0.0
	if open {
		value = 1.0
	}
	CircuitBreakerState.WithLabelValues(backend).Set(value)
}

// RecordCircuitBreakerFailure records a circuit breaker failure
func RecordCircuitBreakerFailure(backend string) {
	CircuitBreakerFailures.WithLabelValues(backend).Inc()
}

// SetActiveConnections sets the number of active connections
func SetActiveConnections(count float64) {
	ActiveConnections.Set(count)
}

// SetMemoryUsage sets the current memory usage
func SetMemoryUsage(bytes float64) {
	MemoryUsageBytes.Set(bytes)
}

// RecordConfigReload records a config reload event
func RecordConfigReload() {
	ConfigReloads.Inc()
}

// RecordConfigReloadFailure records a config reload failure
func RecordConfigReloadFailure() {
	ConfigReloadFailures.Inc()
}
