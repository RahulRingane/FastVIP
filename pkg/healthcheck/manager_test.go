package healthcheck

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RahulRingane/FastVIP/pkg/config"
	"go.uber.org/zap"
)

// boolPtr creates a pointer to a bool value.
func boolPtr(b bool) *bool {
	return &b
}

// --- IsHealthy tests ---

func TestIsHealthy_UnknownAddress(t *testing.T) {
	mgr := NewManager(nil, zap.NewNop())
	if !mgr.IsHealthy("192.168.1.1:8080") {
		t.Error("expected unknown address to be considered healthy")
	}
}

func TestIsHealthy_HealthyBackend(t *testing.T) {
	mgr := NewManager(nil, zap.NewNop())
	mgr.mu.Lock()
	mgr.statuses["192.168.1.1:8080"] = &backendStatus{
		address: "192.168.1.1:8080",
		healthy: true,
	}
	mgr.mu.Unlock()

	if !mgr.IsHealthy("192.168.1.1:8080") {
		t.Error("expected healthy backend to return true")
	}
}

func TestIsHealthy_UnhealthyBackend(t *testing.T) {
	mgr := NewManager(nil, zap.NewNop())
	mgr.mu.Lock()
	mgr.statuses["192.168.1.1:8080"] = &backendStatus{
		address: "192.168.1.1:8080",
		healthy: false,
	}
	mgr.mu.Unlock()

	if mgr.IsHealthy("192.168.1.1:8080") {
		t.Error("expected unhealthy backend to return false")
	}
}

// --- UpdateTargets tests ---

func TestUpdateTargets_RegisterBackend(t *testing.T) {
	mgr := NewManager(nil, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	services := []config.ServiceConfig{
		{
			Name:     "svc1",
			Listen:   "10.0.0.1:80",
			Protocol: "tcp",
			HealthCheck: config.HealthCheckConfig{
				Enabled:  boolPtr(true),
				Interval: "100ms",
				Timeout:  "50ms",
			},
			Backends: []config.BackendConfig{
				{Address: "192.168.1.1:8080", Weight: 1},
			},
		},
	}

	mgr.UpdateTargets(ctx, services)

	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	if _, exists := mgr.statuses["192.168.1.1:8080"]; !exists {
		t.Fatal("expected backend to be registered in statuses")
	}
	if !mgr.statuses["192.168.1.1:8080"].healthy {
		t.Error("expected initial status to be healthy")
	}
}

func TestUpdateTargets_RemoveBackend(t *testing.T) {
	mgr := NewManager(nil, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// First: register a backend
	services1 := []config.ServiceConfig{
		{
			Name:     "svc1",
			Listen:   "10.0.0.1:80",
			Protocol: "tcp",
			HealthCheck: config.HealthCheckConfig{
				Enabled:  boolPtr(true),
				Interval: "100ms",
				Timeout:  "50ms",
			},
			Backends: []config.BackendConfig{
				{Address: "192.168.1.1:8080", Weight: 1},
				{Address: "192.168.1.2:8080", Weight: 1},
			},
		},
	}
	mgr.UpdateTargets(ctx, services1)

	// Second: remove one backend
	services2 := []config.ServiceConfig{
		{
			Name:     "svc1",
			Listen:   "10.0.0.1:80",
			Protocol: "tcp",
			HealthCheck: config.HealthCheckConfig{
				Enabled:  boolPtr(true),
				Interval: "100ms",
				Timeout:  "50ms",
			},
			Backends: []config.BackendConfig{
				{Address: "192.168.1.1:8080", Weight: 1},
			},
		},
	}
	mgr.UpdateTargets(ctx, services2)

	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	if _, exists := mgr.statuses["192.168.1.2:8080"]; exists {
		t.Error("expected removed backend to be cleaned up from statuses")
	}
	if _, exists := mgr.statuses["192.168.1.1:8080"]; !exists {
		t.Error("expected remaining backend to still be in statuses")
	}
}

func TestUpdateTargets_DisabledHealthCheck(t *testing.T) {
	mgr := NewManager(nil, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	services := []config.ServiceConfig{
		{
			Name:     "svc1",
			Listen:   "10.0.0.1:80",
			Protocol: "tcp",
			HealthCheck: config.HealthCheckConfig{
				Enabled: boolPtr(false),
			},
			Backends: []config.BackendConfig{
				{Address: "192.168.1.1:8080", Weight: 1},
			},
		},
	}

	mgr.UpdateTargets(ctx, services)

	// Backend should not be tracked when health check is disabled
	mgr.mu.RLock()
	_, exists := mgr.statuses["192.168.1.1:8080"]
	mgr.mu.RUnlock()

	if exists {
		t.Error("expected backend not to be tracked when health check is disabled")
	}

	// But IsHealthy should return true for untracked backends
	if !mgr.IsHealthy("192.168.1.1:8080") {
		t.Error("expected untracked backend to be considered healthy")
	}
}

func TestUpdateTargets_EnabledToDisabledTransition(t *testing.T) {
	mgr := NewManager(nil, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// First: enable health check
	services1 := []config.ServiceConfig{
		{
			Name:     "svc1",
			Listen:   "10.0.0.1:80",
			Protocol: "tcp",
			HealthCheck: config.HealthCheckConfig{
				Enabled:  boolPtr(true),
				Interval: "100ms",
				Timeout:  "50ms",
			},
			Backends: []config.BackendConfig{
				{Address: "192.168.1.1:8080", Weight: 1},
			},
		},
	}
	mgr.UpdateTargets(ctx, services1)

	mgr.mu.RLock()
	_, tracked := mgr.statuses["192.168.1.1:8080"]
	mgr.mu.RUnlock()
	if !tracked {
		t.Fatal("expected backend to be tracked when health check is enabled")
	}

	// Second: disable health check
	services2 := []config.ServiceConfig{
		{
			Name:     "svc1",
			Listen:   "10.0.0.1:80",
			Protocol: "tcp",
			HealthCheck: config.HealthCheckConfig{
				Enabled: boolPtr(false),
			},
			Backends: []config.BackendConfig{
				{Address: "192.168.1.1:8080", Weight: 1},
			},
		},
	}
	mgr.UpdateTargets(ctx, services2)

	mgr.mu.RLock()
	_, stillTracked := mgr.statuses["192.168.1.1:8080"]
	mgr.mu.RUnlock()
	if stillTracked {
		t.Error("expected backend to be untracked after disabling health check")
	}
}

// --- handleCheckResult tests ---

func TestHandleCheckResult_ConsecutiveFailsMarkUnhealthy(t *testing.T) {
	var onChangeCalled atomic.Int32
	mgr := NewManager(func() {
		onChangeCalled.Add(1)
	}, zap.NewNop())

	svcCheck := &serviceCheckConfig{
		failCount: 3,
		riseCount: 2,
		enabled:   true,
	}

	// Manually inject a backend status
	mgr.mu.Lock()
	mgr.statuses["192.168.1.1:8080"] = &backendStatus{
		address: "192.168.1.1:8080",
		healthy: true,
	}
	mgr.mu.Unlock()

	checkErr := fmt.Errorf("connection refused")

	// Fail 1 and 2: should still be healthy
	mgr.handleCheckResult("192.168.1.1:8080", checkErr, svcCheck)
	mgr.handleCheckResult("192.168.1.1:8080", checkErr, svcCheck)

	mgr.mu.RLock()
	stillHealthy := mgr.statuses["192.168.1.1:8080"].healthy
	mgr.mu.RUnlock()
	if !stillHealthy {
		t.Error("expected backend to still be healthy after 2 failures (threshold is 3)")
	}

	// Fail 3: should become unhealthy
	mgr.handleCheckResult("192.168.1.1:8080", checkErr, svcCheck)

	mgr.mu.RLock()
	nowUnhealthy := !mgr.statuses["192.168.1.1:8080"].healthy
	mgr.mu.RUnlock()
	if !nowUnhealthy {
		t.Error("expected backend to be unhealthy after 3 consecutive failures")
	}

	if onChangeCalled.Load() != 1 {
		t.Errorf("expected onChange to be called once, got %d", onChangeCalled.Load())
	}
}

func TestHandleCheckResult_ConsecutiveSuccessMarkHealthy(t *testing.T) {
	var onChangeCalled atomic.Int32
	mgr := NewManager(func() {
		onChangeCalled.Add(1)
	}, zap.NewNop())

	svcCheck := &serviceCheckConfig{
		failCount: 3,
		riseCount: 2,
		enabled:   true,
	}

	// Start with unhealthy backend
	mgr.mu.Lock()
	mgr.statuses["192.168.1.1:8080"] = &backendStatus{
		address: "192.168.1.1:8080",
		healthy: false,
	}
	mgr.mu.Unlock()

	// Success 1: should still be unhealthy
	mgr.handleCheckResult("192.168.1.1:8080", nil, svcCheck)

	mgr.mu.RLock()
	stillUnhealthy := !mgr.statuses["192.168.1.1:8080"].healthy
	mgr.mu.RUnlock()
	if !stillUnhealthy {
		t.Error("expected backend to still be unhealthy after 1 success (threshold is 2)")
	}

	// Success 2: should become healthy
	mgr.handleCheckResult("192.168.1.1:8080", nil, svcCheck)

	mgr.mu.RLock()
	nowHealthy := mgr.statuses["192.168.1.1:8080"].healthy
	mgr.mu.RUnlock()
	if !nowHealthy {
		t.Error("expected backend to be healthy after 2 consecutive successes")
	}

	if onChangeCalled.Load() != 1 {
		t.Errorf("expected onChange to be called once, got %d", onChangeCalled.Load())
	}
}

func TestHandleCheckResult_NoChangeNoCallback(t *testing.T) {
	var onChangeCalled atomic.Int32
	mgr := NewManager(func() {
		onChangeCalled.Add(1)
	}, zap.NewNop())

	svcCheck := &serviceCheckConfig{
		failCount: 3,
		riseCount: 2,
		enabled:   true,
	}

	// Healthy backend, successful check -> no state change
	mgr.mu.Lock()
	mgr.statuses["192.168.1.1:8080"] = &backendStatus{
		address: "192.168.1.1:8080",
		healthy: true,
	}
	mgr.mu.Unlock()

	mgr.handleCheckResult("192.168.1.1:8080", nil, svcCheck)

	if onChangeCalled.Load() != 0 {
		t.Errorf("expected onChange not to be called when status doesn't change, got %d", onChangeCalled.Load())
	}
}

func TestHandleCheckResult_FailResetsConsecutiveOK(t *testing.T) {
	mgr := NewManager(nil, zap.NewNop())

	svcCheck := &serviceCheckConfig{
		failCount: 3,
		riseCount: 3,
		enabled:   true,
	}

	mgr.mu.Lock()
	mgr.statuses["192.168.1.1:8080"] = &backendStatus{
		address: "192.168.1.1:8080",
		healthy: false,
	}
	mgr.mu.Unlock()

	// 2 successes, then 1 failure should reset the counter
	mgr.handleCheckResult("192.168.1.1:8080", nil, svcCheck)
	mgr.handleCheckResult("192.168.1.1:8080", nil, svcCheck)
	mgr.handleCheckResult("192.168.1.1:8080", fmt.Errorf("fail"), svcCheck)

	mgr.mu.RLock()
	status := mgr.statuses["192.168.1.1:8080"]
	consecutiveOK := status.consecutiveOK
	consecutiveFails := status.consecutiveFails
	mgr.mu.RUnlock()

	if consecutiveOK != 0 {
		t.Errorf("expected consecutiveOK to be reset to 0, got %d", consecutiveOK)
	}
	if consecutiveFails != 1 {
		t.Errorf("expected consecutiveFails to be 1, got %d", consecutiveFails)
	}
}

func TestHandleCheckResult_UnknownAddressIgnored(t *testing.T) {
	mgr := NewManager(nil, zap.NewNop())

	svcCheck := &serviceCheckConfig{
		failCount: 3,
		riseCount: 2,
		enabled:   true,
	}

	// Should not panic or error for unknown address
	mgr.handleCheckResult("unknown:1234", nil, svcCheck)
}

// --- Stop tests ---

func TestStop_ClearsAllState(t *testing.T) {
	mgr := NewManager(nil, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	services := []config.ServiceConfig{
		{
			Name:     "svc1",
			Listen:   "10.0.0.1:80",
			Protocol: "tcp",
			HealthCheck: config.HealthCheckConfig{
				Enabled:  boolPtr(true),
				Interval: "100ms",
				Timeout:  "50ms",
			},
			Backends: []config.BackendConfig{
				{Address: "192.168.1.1:8080", Weight: 1},
				{Address: "192.168.1.2:8080", Weight: 1},
			},
		},
	}

	mgr.UpdateTargets(ctx, services)

	// Verify backends are tracked
	mgr.mu.RLock()
	statusCount := len(mgr.statuses)
	mgr.mu.RUnlock()
	if statusCount != 2 {
		t.Fatalf("expected 2 tracked backends, got %d", statusCount)
	}

	mgr.Stop()

	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	if len(mgr.statuses) != 0 {
		t.Errorf("expected 0 statuses after Stop, got %d", len(mgr.statuses))
	}
	if len(mgr.services) != 0 {
		t.Errorf("expected 0 services after Stop, got %d", len(mgr.services))
	}
}

// --- Integration-style test: full lifecycle ---

func TestManager_FullLifecycle(t *testing.T) {
	var onChangeCalled atomic.Int32
	mgr := NewManager(func() {
		onChangeCalled.Add(1)
	}, zap.NewNop())

	svcCheck := &serviceCheckConfig{
		failCount: 2,
		riseCount: 2,
		enabled:   true,
	}

	// Register backend manually
	mgr.mu.Lock()
	mgr.statuses["192.168.1.1:8080"] = &backendStatus{
		address: "192.168.1.1:8080",
		healthy: true,
	}
	mgr.mu.Unlock()

	// Verify initially healthy
	if !mgr.IsHealthy("192.168.1.1:8080") {
		t.Fatal("expected initially healthy")
	}

	// Fail twice -> unhealthy
	checkErr := fmt.Errorf("connection refused")
	mgr.handleCheckResult("192.168.1.1:8080", checkErr, svcCheck)
	mgr.handleCheckResult("192.168.1.1:8080", checkErr, svcCheck)

	if mgr.IsHealthy("192.168.1.1:8080") {
		t.Fatal("expected unhealthy after 2 failures")
	}

	// Succeed twice -> healthy again
	mgr.handleCheckResult("192.168.1.1:8080", nil, svcCheck)
	mgr.handleCheckResult("192.168.1.1:8080", nil, svcCheck)

	if !mgr.IsHealthy("192.168.1.1:8080") {
		t.Fatal("expected healthy after 2 successes")
	}

	// onChange should have been called twice (unhealthy transition + healthy transition)
	if onChangeCalled.Load() != 2 {
		t.Errorf("expected onChange to be called 2 times, got %d", onChangeCalled.Load())
	}

	// Allow goroutines to settle
	time.Sleep(10 * time.Millisecond)
}
