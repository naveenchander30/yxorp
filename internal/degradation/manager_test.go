package degradation

import (
	"testing"

	"github.com/yxorp/pkg/logger"
)

func init() {
	// Initialize logger for tests
	logger.Init()
}

func TestNewManager(t *testing.T) {
	mgr := NewManager()
	if mgr == nil {
		t.Fatal("expected manager but got nil")
	}

	// All components should start as healthy
	for _, component := range []Component{
		ComponentRuleEngine,
		ComponentRateLimiter,
		ComponentCircuitBreaker,
		ComponentHealthCheck,
		ComponentMetrics,
		ComponentLogging,
	} {
		if status := mgr.GetComponentStatus(component); status != StatusHealthy {
			t.Errorf("expected component %s to be healthy, got %v", component, status)
		}
	}

	// Mode should be normal
	if mode := mgr.GetMode(); mode != ModeNormal {
		t.Errorf("expected mode to be normal, got %v", mode)
	}
}

func TestSetComponentStatus(t *testing.T) {
	mgr := NewManager()

	// Set one component to degraded
	mgr.SetComponentStatus(ComponentRuleEngine, StatusDegraded)
	if status := mgr.GetComponentStatus(ComponentRuleEngine); status != StatusDegraded {
		t.Errorf("expected status degraded, got %v", status)
	}

	// Mode should be partial degradation
	if mode := mgr.GetMode(); mode != ModePartialDegradation {
		t.Errorf("expected mode partial degradation, got %v", mode)
	}

	// Set component to failed
	mgr.SetComponentStatus(ComponentRuleEngine, StatusFailed)
	if status := mgr.GetComponentStatus(ComponentRuleEngine); status != StatusFailed {
		t.Errorf("expected status failed, got %v", status)
	}

	// Mode should be full degradation
	if mode := mgr.GetMode(); mode != ModeFullDegradation {
		t.Errorf("expected mode full degradation, got %v", mode)
	}

	// Set back to healthy
	mgr.SetComponentStatus(ComponentRuleEngine, StatusHealthy)
	if mode := mgr.GetMode(); mode != ModeNormal {
		t.Errorf("expected mode normal, got %v", mode)
	}
}

func TestIsComponentHealthy(t *testing.T) {
	mgr := NewManager()

	if !mgr.IsComponentHealthy(ComponentRuleEngine) {
		t.Error("expected component to be healthy")
	}

	mgr.SetComponentStatus(ComponentRuleEngine, StatusDegraded)
	if mgr.IsComponentHealthy(ComponentRuleEngine) {
		t.Error("expected component to not be healthy")
	}
}

func TestIsComponentDegraded(t *testing.T) {
	mgr := NewManager()

	if mgr.IsComponentDegraded(ComponentRuleEngine) {
		t.Error("expected component to not be degraded")
	}

	mgr.SetComponentStatus(ComponentRuleEngine, StatusDegraded)
	if !mgr.IsComponentDegraded(ComponentRuleEngine) {
		t.Error("expected component to be degraded")
	}

	mgr.SetComponentStatus(ComponentRuleEngine, StatusFailed)
	if !mgr.IsComponentDegraded(ComponentRuleEngine) {
		t.Error("expected failed component to be considered degraded")
	}
}

func TestDegradationModes(t *testing.T) {
	tests := []struct {
		name          string
		degradedCount int
		failedCount   int
		expectedMode  DegradationMode
	}{
		{
			name:          "all healthy",
			degradedCount: 0,
			failedCount:   0,
			expectedMode:  ModeNormal,
		},
		{
			name:          "one degraded",
			degradedCount: 1,
			failedCount:   0,
			expectedMode:  ModePartialDegradation,
		},
		{
			name:          "two degraded",
			degradedCount: 2,
			failedCount:   0,
			expectedMode:  ModePartialDegradation,
		},
		{
			name:          "three degraded",
			degradedCount: 3,
			failedCount:   0,
			expectedMode:  ModeFullDegradation,
		},
		{
			name:          "one failed",
			degradedCount: 0,
			failedCount:   1,
			expectedMode:  ModeFullDegradation,
		},
		{
			name:          "mixed degradation",
			degradedCount: 2,
			failedCount:   1,
			expectedMode:  ModeFullDegradation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager()

			components := []Component{
				ComponentRuleEngine,
				ComponentRateLimiter,
				ComponentCircuitBreaker,
				ComponentHealthCheck,
				ComponentMetrics,
				ComponentLogging,
			}

			// Set components to degraded
			for i := 0; i < tt.degradedCount && i < len(components); i++ {
				mgr.SetComponentStatus(components[i], StatusDegraded)
			}

			// Set components to failed
			for i := 0; i < tt.failedCount && i < len(components); i++ {
				mgr.SetComponentStatus(components[i], StatusFailed)
			}

			if mode := mgr.GetMode(); mode != tt.expectedMode {
				t.Errorf("expected mode %v, got %v", tt.expectedMode, mode)
			}
		})
	}
}

func TestShouldBypassMethods(t *testing.T) {
	mgr := NewManager()

	// Initially, nothing should be bypassed
	if mgr.ShouldBypassRules() {
		t.Error("expected rules to not be bypassed")
	}
	if mgr.ShouldBypassRateLimit() {
		t.Error("expected rate limit to not be bypassed")
	}
	if mgr.ShouldBypassCircuitBreaker() {
		t.Error("expected circuit breaker to not be bypassed")
	}

	// Degrade rule engine
	mgr.SetComponentStatus(ComponentRuleEngine, StatusDegraded)
	if !mgr.ShouldBypassRules() {
		t.Error("expected rules to be bypassed")
	}

	// Degrade rate limiter
	mgr.SetComponentStatus(ComponentRateLimiter, StatusFailed)
	if !mgr.ShouldBypassRateLimit() {
		t.Error("expected rate limit to be bypassed")
	}

	// Degrade circuit breaker
	mgr.SetComponentStatus(ComponentCircuitBreaker, StatusDegraded)
	if !mgr.ShouldBypassCircuitBreaker() {
		t.Error("expected circuit breaker to be bypassed")
	}
}

func TestGetAllComponentStatuses(t *testing.T) {
	mgr := NewManager()

	mgr.SetComponentStatus(ComponentRuleEngine, StatusDegraded)
	mgr.SetComponentStatus(ComponentRateLimiter, StatusFailed)

	statuses := mgr.GetAllComponentStatuses()

	if len(statuses) != 6 {
		t.Errorf("expected 6 components, got %d", len(statuses))
	}

	if statuses[ComponentRuleEngine] != StatusDegraded {
		t.Error("expected rule engine to be degraded")
	}
	if statuses[ComponentRateLimiter] != StatusFailed {
		t.Error("expected rate limiter to be failed")
	}
	if statuses[ComponentMetrics] != StatusHealthy {
		t.Error("expected metrics to be healthy")
	}
}
