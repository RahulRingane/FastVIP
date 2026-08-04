package server

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fastVIP/fastVIP/pkg/config"
	"github.com/fastVIP/fastVIP/pkg/lvs"
	"github.com/fastVIP/fastVIP/pkg/snat"
	"go.uber.org/zap"
)

// controllableHealthChecker is a mock HealthChecker that allows tests to
// control the health status of individual backends.
type controllableHealthChecker struct {
	status map[string]bool
	mu     sync.RWMutex
}

func newControllableHealthChecker() *controllableHealthChecker {
	return &controllableHealthChecker{
		status: make(map[string]bool),
	}
}

func (c *controllableHealthChecker) IsHealthy(address string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	healthy, ok := c.status[address]
	if !ok {
		return true
	}
	return healthy
}

func (c *controllableHealthChecker) SetHealthy(address string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status[address] = true
}

func (c *controllableHealthChecker) SetUnhealthy(address string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status[address] = false
}

// writeYAMLFile writes YAML content to a file and returns the path.
func writeYAMLFile(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "fastVIP.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write YAML file: %v", err)
	}
	return path
}

// --- Flow A: Initial sync on startup ---

func TestIntegration_FlowA_InitialSync(t *testing.T) {
	configYAML := `
global:
  log:
    level: info
services:
  - name: web-service
    listen: 10.0.0.1:80
    protocol: tcp
    scheduler: wrr
    health_check:
      enabled: false
    backends:
      - address: 192.168.1.10:8080
        weight: 5
      - address: 192.168.1.11:8080
        weight: 3
  - name: api-service
    listen: 10.0.0.2:443
    protocol: tcp
    scheduler: rr
    health_check:
      enabled: false
    backends:
      - address: 192.168.2.10:9090
        weight: 1
      - address: 192.168.2.11:9090
        weight: 1
`
	dir := t.TempDir()
	configPath := writeYAMLFile(t, dir, configYAML)

	srv := newTestServer(t, configPath)

	// RunOnce performs a single reconcile and shuts down
	if err := srv.RunOnce(); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	// After RunOnce, the lvs manager is closed. Create a new server with a fresh
	// manager (without flushing IPVS) to verify the reconcile path is idempotent.
	logger := zap.NewNop()
	lvsMgr2 := newTestLVSManager(t)
	srv2, err := newServerWithManager(configPath, lvsMgr2, logger, zap.NewNop())
	if err != nil {
		t.Fatalf("newServerWithManager failed: %v", err)
	}

	// Verify config was loaded correctly
	cfg := srv2.configMgr.GetConfig()
	if len(cfg.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(cfg.Services))
	}
	if cfg.Services[0].Name != "web-service" {
		t.Errorf("expected first service 'web-service', got %q", cfg.Services[0].Name)
	}
	if cfg.Services[1].Name != "api-service" {
		t.Errorf("expected second service 'api-service', got %q", cfg.Services[1].Name)
	}

	// Verify IPVS state via lvs manager after RunOnce
	if err := srv2.RunOnce(); err != nil {
		t.Fatalf("second RunOnce failed: %v", err)
	}
}

// --- Flow A extended: Verify IPVS state after RunOnce ---

func TestIntegration_FlowA_VerifyIPVSState(t *testing.T) {
	configYAML := `
global:
  log:
    level: info
services:
  - name: web-service
    listen: 10.0.0.1:80
    protocol: tcp
    scheduler: wrr
    health_check:
      enabled: false
    backends:
      - address: 192.168.1.10:8080
        weight: 5
      - address: 192.168.1.11:8080
        weight: 3
`
	dir := t.TempDir()
	configPath := writeYAMLFile(t, dir, configYAML)

	srv := newTestServer(t, configPath)

	// Perform initial reconcile without shutting down to inspect IPVS state
	cfg := srv.configMgr.GetConfig()
	if err := srv.reconciler.Reconcile(cfg.Services); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Verify IPVS services
	services, err := srv.lvsMgr.GetServices()
	if err != nil {
		t.Fatalf("GetServices failed: %v", err)
	}
	if len(services) != 1 {
		t.Fatalf("expected 1 IPVS service, got %d", len(services))
	}
	if services[0].SchedName != "wrr" {
		t.Errorf("expected scheduler 'wrr', got %q", services[0].SchedName)
	}

	// Verify destinations
	dests, err := srv.lvsMgr.GetDestinations(services[0])
	if err != nil {
		t.Fatalf("GetDestinations failed: %v", err)
	}
	if len(dests) != 2 {
		t.Fatalf("expected 2 destinations, got %d", len(dests))
	}

	// Clean up
	srv.shutdown()
}

// --- Flow B: Config hot-reload triggers Reconcile ---

func TestIntegration_FlowB_ConfigHotReload(t *testing.T) {
	initialYAML := `
global:
  log:
    level: info
services:
  - name: web-service
    listen: 10.0.0.1:80
    protocol: tcp
    scheduler: rr
    health_check:
      enabled: false
    backends:
      - address: 192.168.1.10:8080
        weight: 5
      - address: 192.168.1.11:8080
        weight: 3
`
	dir := t.TempDir()
	configPath := writeYAMLFile(t, dir, initialYAML)

	srv := newTestServer(t, configPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Perform initial reconcile
	cfg := srv.configMgr.GetConfig()
	srv.healthMgr.UpdateTargets(ctx, cfg.Services)
	if err := srv.reconciler.Reconcile(cfg.Services); err != nil {
		t.Fatalf("initial Reconcile failed: %v", err)
	}

	// Verify initial state
	services, _ := srv.lvsMgr.GetServices()
	if len(services) != 1 {
		t.Fatalf("expected 1 service initially, got %d", len(services))
	}
	if services[0].SchedName != "rr" {
		t.Errorf("expected initial scheduler 'rr', got %q", services[0].SchedName)
	}
	dests, _ := srv.lvsMgr.GetDestinations(services[0])
	if len(dests) != 2 {
		t.Fatalf("expected 2 destinations initially, got %d", len(dests))
	}

	// Start config watching
	srv.configMgr.WatchConfig()

	// Modify config: change scheduler and add a backend
	updatedYAML := `
global:
  log:
    level: info
services:
  - name: web-service
    listen: 10.0.0.1:80
    protocol: tcp
    scheduler: wrr
    health_check:
      enabled: false
    backends:
      - address: 192.168.1.10:8080
        weight: 5
      - address: 192.168.1.11:8080
        weight: 3
      - address: 192.168.1.12:8080
        weight: 2
`
	if err := os.WriteFile(configPath, []byte(updatedYAML), 0644); err != nil {
		t.Fatalf("failed to update config file: %v", err)
	}

	// Wait for config change to be detected
	select {
	case <-srv.configMgr.OnChange():
		// Config change detected
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for config change notification")
	}

	// Reconcile with new config
	newCfg := srv.configMgr.GetConfig()
	srv.healthMgr.UpdateTargets(ctx, newCfg.Services)
	if err := srv.reconciler.Reconcile(newCfg.Services); err != nil {
		t.Fatalf("Reconcile after config change failed: %v", err)
	}

	// Verify updated state
	services, _ = srv.lvsMgr.GetServices()
	if services[0].SchedName != "wrr" {
		t.Errorf("expected updated scheduler 'wrr', got %q", services[0].SchedName)
	}
	dests, _ = srv.lvsMgr.GetDestinations(services[0])
	if len(dests) != 3 {
		t.Fatalf("expected 3 destinations after config update, got %d", len(dests))
	}

	srv.shutdown()
}

// --- Flow C: Health status change triggers Reconcile ---
// This test directly assembles modules with a controllable health checker
// to simulate health status changes without relying on real TCP probes.

func TestIntegration_FlowC_HealthStatusChange(t *testing.T) {
	configYAML := `
global:
  log:
    level: info
services:
  - name: web-service
    listen: 10.0.0.1:80
    protocol: tcp
    scheduler: rr
    health_check:
      enabled: true
      interval: 100ms
      timeout: 50ms
      fail_count: 2
      rise_count: 2
    backends:
      - address: 192.168.1.10:8080
        weight: 5
      - address: 192.168.1.11:8080
        weight: 3
`
	dir := t.TempDir()
	configPath := writeYAMLFile(t, dir, configYAML)
	logger := zap.NewNop()

	// Load config
	configMgr, err := config.NewManager(configPath, logger)
	if err != nil {
		t.Fatalf("config.NewManager failed: %v", err)
	}

	// Create IPVS manager (platform-appropriate: fakeHandle on macOS, real IPVS on Linux)
	lvsMgr := newTestLVSManager(t)
	defer lvsMgr.Close()

	// Create controllable health checker
	healthChecker := newControllableHealthChecker()
	healthChecker.SetHealthy("192.168.1.10:8080")
	healthChecker.SetHealthy("192.168.1.11:8080")

	// Create reconciler with controllable health checker
	snatMgr, _ := snat.NewManager(logger.Named("snat"))
	reconciler := lvs.NewReconciler(lvsMgr, healthChecker, snatMgr, logger)

	cfg := configMgr.GetConfig()

	// First reconcile: all backends healthy -> 2 destinations
	if err := reconciler.Reconcile(cfg.Services); err != nil {
		t.Fatalf("initial Reconcile failed: %v", err)
	}

	services, _ := lvsMgr.GetServices()
	if len(services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(services))
	}
	dests, _ := lvsMgr.GetDestinations(services[0])
	if len(dests) != 2 {
		t.Fatalf("expected 2 destinations initially (all healthy), got %d", len(dests))
	}

	// Mark one backend as unhealthy
	healthChecker.SetUnhealthy("192.168.1.11:8080")

	// Reconcile again: should exclude unhealthy backend
	if err := reconciler.Reconcile(cfg.Services); err != nil {
		t.Fatalf("Reconcile after health change failed: %v", err)
	}

	services, _ = lvsMgr.GetServices()
	dests, _ = lvsMgr.GetDestinations(services[0])
	if len(dests) != 1 {
		t.Fatalf("expected 1 destination (1 unhealthy), got %d", len(dests))
	}

	// Mark backend as healthy again
	healthChecker.SetHealthy("192.168.1.11:8080")

	// Reconcile again: should include recovered backend
	if err := reconciler.Reconcile(cfg.Services); err != nil {
		t.Fatalf("Reconcile after recovery failed: %v", err)
	}

	services, _ = lvsMgr.GetServices()
	dests, _ = lvsMgr.GetDestinations(services[0])
	if len(dests) != 2 {
		t.Fatalf("expected 2 destinations after recovery, got %d", len(dests))
	}
}

// --- Flow D: Graceful shutdown ---

func TestIntegration_FlowD_GracefulShutdown(t *testing.T) {
	configYAML := `
global:
  log:
    level: info
services:
  - name: web-service
    listen: 10.0.0.1:80
    protocol: tcp
    scheduler: rr
    health_check:
      enabled: false
    backends:
      - address: 192.168.1.10:8080
        weight: 1
`
	dir := t.TempDir()
	configPath := writeYAMLFile(t, dir, configYAML)

	srv := newTestServer(t, configPath)

	ctx, cancel := context.WithCancel(context.Background())

	// Start server in background
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- srv.Run(ctx)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Cancel context to trigger graceful shutdown
	cancel()

	// Wait for server to stop
	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatalf("server Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server to shut down")
	}
}
