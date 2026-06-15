package metrics

import (
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

var initOnce sync.Once

func initTestMetrics() {
	initOnce.Do(func() {
		defer func() {
			_ = recover() // Ignore duplicate registration panics
		}()
		Init()
	})
}

func getCounterValue(counter *prometheus.CounterVec, labels ...string) float64 {
	var m dto.Metric
	if err := counter.WithLabelValues(labels...).Write(&m); err != nil {
		return 0
	}
	return m.GetCounter().GetValue()
}

func getGaugeValue(gauge *prometheus.GaugeVec, labels ...string) float64 {
	var m dto.Metric
	if err := gauge.WithLabelValues(labels...).Write(&m); err != nil {
		return 0
	}
	return m.GetGauge().GetValue()
}

func TestPrometheusMetrics(t *testing.T) {
	initTestMetrics()

	// 1. HTTP Request Metrics
	initialVal := getCounterValue(HTTPRequestsTotal, "GET", "/test-path", "200")
	RecordHTTPRequest("GET", "/test-path", "200", 50*time.Millisecond)
	newVal := getCounterValue(HTTPRequestsTotal, "GET", "/test-path", "200")
	if newVal != initialVal+1 {
		t.Errorf("Expected HTTPRequestsTotal to increment by 1, got %f -> %f", initialVal, newVal)
	}

	// 2. WAF Block Metrics
	initialVal = getCounterValue(WAFBlockedRequests, "sql_injection", "matched_security_rule")
	RecordWAFBlock("sql_injection", "matched_security_rule")
	newVal = getCounterValue(WAFBlockedRequests, "sql_injection", "matched_security_rule")
	if newVal != initialVal+1 {
		t.Errorf("Expected WAFBlockedRequests to increment by 1, got %f -> %f", initialVal, newVal)
	}

	// 3. Rate Limit Exceeded
	initialVal = getCounterValue(RateLimitExceeded, "192.168.1.1")
	RecordRateLimitExceeded("192.168.1.1")
	newVal = getCounterValue(RateLimitExceeded, "192.168.1.1")
	if newVal != initialVal+1 {
		t.Errorf("Expected RateLimitExceeded to increment, got %f -> %f", initialVal, newVal)
	}

	// 4. Backend Requests
	initialVal = getCounterValue(BackendRequestsTotal, "http://backend1:80", "200")
	RecordBackendRequest("http://backend1:80", "200")
	newVal = getCounterValue(BackendRequestsTotal, "http://backend1:80", "200")
	if newVal != initialVal+1 {
		t.Errorf("Expected BackendRequestsTotal to increment, got %f -> %f", initialVal, newVal)
	}

	// 5. Backend Health
	SetBackendHealth("http://backend1:80", true)
	if val := getGaugeValue(BackendHealthStatus, "http://backend1:80"); val != 1.0 {
		t.Errorf("Expected BackendHealthStatus 1.0, got %f", val)
	}
	SetBackendHealth("http://backend1:80", false)
	if val := getGaugeValue(BackendHealthStatus, "http://backend1:80"); val != 0.0 {
		t.Errorf("Expected BackendHealthStatus 0.0, got %f", val)
	}

	// 6. Circuit Breaker State
	SetCircuitBreakerState("http://backend1:80", true)
	if val := getGaugeValue(CircuitBreakerState, "http://backend1:80"); val != 1.0 {
		t.Errorf("Expected CircuitBreakerState 1.0, got %f", val)
	}
	SetCircuitBreakerState("http://backend1:80", false)
	if val := getGaugeValue(CircuitBreakerState, "http://backend1:80"); val != 0.0 {
		t.Errorf("Expected CircuitBreakerState 0.0, got %f", val)
	}

	// 7. Circuit Breaker Failures
	initialVal = getCounterValue(CircuitBreakerFailures, "http://backend1:80")
	RecordCircuitBreakerFailure("http://backend1:80")
	newVal = getCounterValue(CircuitBreakerFailures, "http://backend1:80")
	if newVal != initialVal+1 {
		t.Errorf("Expected CircuitBreakerFailures to increment, got %f -> %f", initialVal, newVal)
	}

	// 8. Active Connections
	// Since ActiveConnections is a plain Gauge (not GaugeVec), we read it via dto directly:
	var m dto.Metric
	if err := ActiveConnections.Write(&m); err == nil {
		initialConn := m.GetGauge().GetValue()
		ActiveConnections.Inc()
		ActiveConnections.Write(&m)
		newConn := m.GetGauge().GetValue()
		if newConn != initialConn+1 {
			t.Errorf("Expected ActiveConnections to increment, got %f -> %f", initialConn, newConn)
		}
	}

	// 9. Memory Usage
	SetMemoryUsage(1024 * 1024)
	if err := MemoryUsageBytes.Write(&m); err == nil {
		if val := m.GetGauge().GetValue(); val != 1024*1024 {
			t.Errorf("Expected memory usage 1MB, got %f", val)
		}
	}
}
