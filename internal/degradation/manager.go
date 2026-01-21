package degradation

import (
	"sync"

	"github.com/yxorp/pkg/logger"
)

// ComponentStatus represents the status of a system component
type ComponentStatus int

const (
	StatusHealthy ComponentStatus = iota
	StatusDegraded
	StatusFailed
)

// Component represents a system component that can be degraded
type Component string

const (
	ComponentRuleEngine     Component = "rule_engine"
	ComponentRateLimiter    Component = "rate_limiter"
	ComponentCircuitBreaker Component = "circuit_breaker"
	ComponentHealthCheck    Component = "health_check"
	ComponentMetrics        Component = "metrics"
	ComponentLogging        Component = "logging"
)

// Manager manages graceful degradation across system components
type Manager struct {
	mu         sync.RWMutex
	components map[Component]ComponentStatus
	mode       DegradationMode
}

// DegradationMode defines the overall system degradation mode
type DegradationMode int

const (
	ModeNormal DegradationMode = iota
	ModePartialDegradation
	ModeFullDegradation
)

// NewManager creates a new degradation manager
func NewManager() *Manager {
	return &Manager{
		components: map[Component]ComponentStatus{
			ComponentRuleEngine:     StatusHealthy,
			ComponentRateLimiter:    StatusHealthy,
			ComponentCircuitBreaker: StatusHealthy,
			ComponentHealthCheck:    StatusHealthy,
			ComponentMetrics:        StatusHealthy,
			ComponentLogging:        StatusHealthy,
		},
		mode: ModeNormal,
	}
}

// SetComponentStatus updates the status of a component
func (m *Manager) SetComponentStatus(component Component, status ComponentStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()

	oldStatus := m.components[component]
	m.components[component] = status

	if oldStatus != status {
		logger.Warn("Component status changed", "component", string(component), "old_status", statusString(oldStatus), "new_status", statusString(status))
	}

	m.updateMode()
}

// GetComponentStatus returns the status of a component
func (m *Manager) GetComponentStatus(component Component) ComponentStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.components[component]
}

// IsComponentHealthy returns true if the component is healthy
func (m *Manager) IsComponentHealthy(component Component) bool {
	return m.GetComponentStatus(component) == StatusHealthy
}

// IsComponentDegraded returns true if the component is degraded
func (m *Manager) IsComponentDegraded(component Component) bool {
	status := m.GetComponentStatus(component)
	return status == StatusDegraded || status == StatusFailed
}

// GetMode returns the current degradation mode
func (m *Manager) GetMode() DegradationMode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mode
}

// updateMode updates the overall system degradation mode based on component statuses
// Should be called with lock held
func (m *Manager) updateMode() {
	failedCount := 0
	degradedCount := 0

	for _, status := range m.components {
		switch status {
		case StatusFailed:
			failedCount++
		case StatusDegraded:
			degradedCount++
		}
	}

	oldMode := m.mode

	// Determine mode based on component health
	if failedCount > 0 || degradedCount >= 3 {
		m.mode = ModeFullDegradation
	} else if degradedCount > 0 {
		m.mode = ModePartialDegradation
	} else {
		m.mode = ModeNormal
	}

	if oldMode != m.mode {
		logger.Warn("System degradation mode changed", "old_mode", modeString(oldMode), "new_mode", modeString(m.mode), "failed_components", failedCount, "degraded_components", degradedCount)
	}
}

// ShouldBypassRules returns true if rule engine checks should be bypassed
func (m *Manager) ShouldBypassRules() bool {
	return m.IsComponentDegraded(ComponentRuleEngine)
}

// ShouldBypassRateLimit returns true if rate limiting should be bypassed
func (m *Manager) ShouldBypassRateLimit() bool {
	return m.IsComponentDegraded(ComponentRateLimiter)
}

// ShouldBypassCircuitBreaker returns true if circuit breaker should be bypassed
func (m *Manager) ShouldBypassCircuitBreaker() bool {
	return m.IsComponentDegraded(ComponentCircuitBreaker)
}

// GetAllComponentStatuses returns the status of all components
func (m *Manager) GetAllComponentStatuses() map[Component]ComponentStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[Component]ComponentStatus)
	for k, v := range m.components {
		result[k] = v
	}
	return result
}

// Helper functions for string representation

func statusString(status ComponentStatus) string {
	switch status {
	case StatusHealthy:
		return "healthy"
	case StatusDegraded:
		return "degraded"
	case StatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

func modeString(mode DegradationMode) string {
	switch mode {
	case ModeNormal:
		return "normal"
	case ModePartialDegradation:
		return "partial_degradation"
	case ModeFullDegradation:
		return "full_degradation"
	default:
		return "unknown"
	}
}
