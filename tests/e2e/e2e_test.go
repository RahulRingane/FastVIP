//go:build integration

package e2e

import (
	"bytes"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// --- Test 1: Single service with once mode ---

func TestE2E_OnceMode_SingleService(t *testing.T) {
	flushIPVS(t)
	defer flushIPVS(t)

	configYAML := `
global:
  log_level: info
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
	configPath := writeTestConfig(t, dir, configYAML)

	runEzlbOnce(t, configPath)

	// Verify IPVS state
	services := requireServiceCount(t, 1)

	svc := findServiceByAddress(services, "10.0.0.1", 80)
	if svc == nil {
		t.Fatal("expected to find service 10.0.0.1:80")
	}
	if svc.SchedName != "wrr" {
		t.Errorf("expected scheduler 'wrr', got %q", svc.SchedName)
	}

	destinations := requireDestinationCount(t, svc, 2)

	// Verify destination weights (order may vary)
	weightSet := map[int]bool{}
	for _, dst := range destinations {
		weightSet[dst.Weight] = true
	}
	if !weightSet[5] || !weightSet[3] {
		t.Errorf("expected destination weights {5, 3}, got %v", weightSet)
	}
}

// --- Test 2: Multiple services with different schedulers ---

func TestE2E_OnceMode_MultiService(t *testing.T) {
	flushIPVS(t)
	defer flushIPVS(t)

	configYAML := `
global:
  log_level: info
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
      - address: 192.168.1.11:8080
        weight: 1

  - name: api-service
    listen: 10.0.0.2:443
    protocol: tcp
    scheduler: wrr
    health_check:
      enabled: false
    backends:
      - address: 192.168.2.10:9090
        weight: 5
      - address: 192.168.2.11:9090
        weight: 3

  - name: internal-service
    listen: 10.0.0.3:9090
    protocol: tcp
    scheduler: lc
    health_check:
      enabled: false
    backends:
      - address: 192.168.3.10:7070
        weight: 1
`
	dir := t.TempDir()
	configPath := writeTestConfig(t, dir, configYAML)

	runEzlbOnce(t, configPath)

	// Verify 3 services exist
	services := requireServiceCount(t, 3)

	// Verify each service's scheduler
	testCases := []struct {
		ip        string
		port      uint16
		scheduler string
		destCount int
	}{
		{"10.0.0.1", 80, "rr", 2},
		{"10.0.0.2", 443, "wrr", 2},
		{"10.0.0.3", 9090, "lc", 1},
	}

	for _, tc := range testCases {
		svc := findServiceByAddress(services, tc.ip, tc.port)
		if svc == nil {
			t.Fatalf("expected to find service %s:%d", tc.ip, tc.port)
		}
		if svc.SchedName != tc.scheduler {
			t.Errorf("service %s:%d: expected scheduler %q, got %q",
				tc.ip, tc.port, tc.scheduler, svc.SchedName)
		}
		requireDestinationCount(t, svc, tc.destCount)
	}
}

// --- Test 3: Idempotent execution ---

func TestE2E_OnceMode_Idempotent(t *testing.T) {
	flushIPVS(t)
	defer flushIPVS(t)

	configYAML := `
global:
  log_level: info
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
	configPath := writeTestConfig(t, dir, configYAML)

	// First execution
	runEzlbOnce(t, configPath)

	services1 := requireServiceCount(t, 1)
	dests1 := requireDestinationCount(t, services1[0], 2)

	// Second execution (should be idempotent)
	runEzlbOnce(t, configPath)

	services2 := requireServiceCount(t, 1)
	dests2 := requireDestinationCount(t, services2[0], 2)

	// Verify state is unchanged
	if services1[0].SchedName != services2[0].SchedName {
		t.Errorf("scheduler changed after idempotent run: %q -> %q",
			services1[0].SchedName, services2[0].SchedName)
	}
	if len(dests1) != len(dests2) {
		t.Errorf("destination count changed after idempotent run: %d -> %d",
			len(dests1), len(dests2))
	}
}

// --- Test 4: Config update between two once executions ---

func TestE2E_OnceMode_ConfigUpdate(t *testing.T) {
	flushIPVS(t)
	defer flushIPVS(t)

	initialYAML := `
global:
  log_level: info
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
	configPath := writeTestConfig(t, dir, initialYAML)

	// First execution
	runEzlbOnce(t, configPath)

	services := requireServiceCount(t, 1)
	svc := findServiceByAddress(services, "10.0.0.1", 80)
	if svc == nil {
		t.Fatal("expected to find service 10.0.0.1:80")
	}
	if svc.SchedName != "rr" {
		t.Errorf("expected initial scheduler 'rr', got %q", svc.SchedName)
	}
	requireDestinationCount(t, svc, 2)

	// Update config: change scheduler and add a backend
	updatedYAML := `
global:
  log_level: info
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
	writeTestConfig(t, dir, updatedYAML)

	// Second execution with updated config
	runEzlbOnce(t, configPath)

	services = requireServiceCount(t, 1)
	svc = findServiceByAddress(services, "10.0.0.1", 80)
	if svc == nil {
		t.Fatal("expected to find service 10.0.0.1:80 after update")
	}
	if svc.SchedName != "wrr" {
		t.Errorf("expected updated scheduler 'wrr', got %q", svc.SchedName)
	}
	requireDestinationCount(t, svc, 3)
}

// --- Test 5: Service removal between two once executions ---
// Note: In `once` mode, each execution creates a fresh Reconciler with an empty
// `managed` map. The Reconciler only tracks services it creates during the current
// run, so it will NOT delete services from a previous run that are no longer in
// the config. This test verifies this actual behavior: after removing a service
// from config and running `once` again, the old service still exists in IPVS
// because the new Reconciler doesn't know about it.

func TestE2E_OnceMode_ServiceRemoval(t *testing.T) {
	flushIPVS(t)
	defer flushIPVS(t)

	initialYAML := `
global:
  log_level: info
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

  - name: api-service
    listen: 10.0.0.2:443
    protocol: tcp
    scheduler: wrr
    health_check:
      enabled: false
    backends:
      - address: 192.168.2.10:9090
        weight: 1
`
	dir := t.TempDir()
	configPath := writeTestConfig(t, dir, initialYAML)

	// First execution: creates 2 services
	runEzlbOnce(t, configPath)
	requireServiceCount(t, 2)

	// Update config: remove api-service
	updatedYAML := `
global:
  log_level: info
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
	writeTestConfig(t, dir, updatedYAML)

	// Second execution: the new Reconciler's managed map is empty,
	// so it will create/update web-service but NOT delete api-service.
	// Both services remain in IPVS.
	runEzlbOnce(t, configPath)

	services := getIPVSServices(t)
	if len(services) != 2 {
		t.Fatalf("expected 2 IPVS services (once mode does not clean up unmanaged services), got %d", len(services))
	}

	// Verify web-service still exists and is correct
	webSvc := findServiceByAddress(services, "10.0.0.1", 80)
	if webSvc == nil {
		t.Fatal("expected web-service (10.0.0.1:80) to still exist")
	}

	// Verify api-service still exists (orphaned from previous run)
	apiSvc := findServiceByAddress(services, "10.0.0.2", 443)
	if apiSvc == nil {
		t.Fatal("expected api-service (10.0.0.2:443) to still exist (once mode does not clean up)")
	}
}

// --- Test 6: Invalid config ---

func TestE2E_OnceMode_InvalidConfig(t *testing.T) {
	flushIPVS(t)
	defer flushIPVS(t)

	// Config with no backends (validation should fail)
	invalidYAML := `
global:
  log_level: info
services:
  - name: bad-service
    listen: 10.0.0.1:80
    protocol: tcp
    scheduler: rr
    health_check:
      enabled: false
    backends: []
`
	dir := t.TempDir()
	configPath := writeTestConfig(t, dir, invalidYAML)

	_, stderr := runEzlbOnceExpectFailure(t, configPath)

	if !strings.Contains(stderr, "backend") && !strings.Contains(stderr, "config") {
		t.Errorf("expected error message about backends or config, got stderr: %s", stderr)
	}

	// Verify no IPVS services were created
	requireServiceCount(t, 0)
}

// --- Test 7: Daemon mode with graceful shutdown ---

func TestE2E_DaemonMode_GracefulShutdown(t *testing.T) {
	flushIPVS(t)
	defer flushIPVS(t)

	configYAML := `
global:
  log_level: info
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
	configPath := writeTestConfig(t, dir, configYAML)

	// Start daemon in background
	cmd := runEzlbDaemon(t, configPath)

	// Give the daemon time to start and perform initial reconcile
	time.Sleep(500 * time.Millisecond)

	// Verify IPVS rules were created
	services := getIPVSServices(t)
	if len(services) < 1 {
		t.Fatalf("expected at least 1 IPVS service after daemon start, got %d", len(services))
	}

	svc := findServiceByAddress(services, "10.0.0.1", 80)
	if svc == nil {
		t.Fatal("expected to find service 10.0.0.1:80 after daemon start")
	}

	// Send SIGTERM for graceful shutdown
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("failed to send SIGTERM: %v", err)
	}

	// Wait for process to exit with timeout
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- cmd.Wait()
	}()

	select {
	case err := <-doneCh:
		if err != nil {
			t.Fatalf("daemon exited with error: %v", err)
		}
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		t.Fatal("daemon did not exit within 10 seconds after SIGTERM")
	}
}

// --- Test 8: Daemon mode with cleanup_on_exit: true (default) — IPVS rules removed after exit ---

func TestE2E_DaemonMode_CleanupOnExit_True(t *testing.T) {
	flushIPVS(t)
	defer flushIPVS(t)

	configYAML := `
global:
  log_level: info
  cleanup_on_exit: true
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
	configPath := writeTestConfig(t, dir, configYAML)

	cmd := runEzlbDaemon(t, configPath)

	// Wait for initial reconcile
	time.Sleep(500 * time.Millisecond)

	// Verify IPVS service was created
	services := getIPVSServices(t)
	if len(services) < 1 {
		t.Fatalf("expected at least 1 IPVS service after daemon start, got %d", len(services))
	}
	if findServiceByAddress(services, "10.0.0.1", 80) == nil {
		t.Fatal("expected to find service 10.0.0.1:80 after daemon start")
	}

	// Send SIGTERM for graceful shutdown
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("failed to send SIGTERM: %v", err)
	}

	doneCh := make(chan error, 1)
	go func() { doneCh <- cmd.Wait() }()
	select {
	case err := <-doneCh:
		if err != nil {
			t.Fatalf("daemon exited with error: %v", err)
		}
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		t.Fatal("daemon did not exit within 10 seconds after SIGTERM")
	}

	// Verify IPVS service was cleaned up
	services = getIPVSServices(t)
	if findServiceByAddress(services, "10.0.0.1", 80) != nil {
		t.Error("expected service 10.0.0.1:80 to be removed after daemon exit with cleanup_on_exit: true")
	}
}

// --- Test 9: Daemon mode with cleanup_on_exit: false — IPVS rules preserved after exit ---

func TestE2E_DaemonMode_CleanupOnExit_False(t *testing.T) {
	flushIPVS(t)
	defer flushIPVS(t)

	configYAML := `
global:
  log_level: info
  cleanup_on_exit: false
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
	configPath := writeTestConfig(t, dir, configYAML)

	cmd := runEzlbDaemon(t, configPath)

	// Wait for initial reconcile
	time.Sleep(500 * time.Millisecond)

	// Verify IPVS service was created
	services := getIPVSServices(t)
	if len(services) < 1 {
		t.Fatalf("expected at least 1 IPVS service after daemon start, got %d", len(services))
	}
	if findServiceByAddress(services, "10.0.0.1", 80) == nil {
		t.Fatal("expected to find service 10.0.0.1:80 after daemon start")
	}

	// Send SIGTERM for graceful shutdown
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("failed to send SIGTERM: %v", err)
	}

	doneCh := make(chan error, 1)
	go func() { doneCh <- cmd.Wait() }()
	select {
	case err := <-doneCh:
		if err != nil {
			t.Fatalf("daemon exited with error: %v", err)
		}
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		t.Fatal("daemon did not exit within 10 seconds after SIGTERM")
	}

	// Verify IPVS service is still present
	services = getIPVSServices(t)
	if findServiceByAddress(services, "10.0.0.1", 80) == nil {
		t.Error("expected service 10.0.0.1:80 to be preserved after daemon exit with cleanup_on_exit: false")
	}
}

// --- Test 10: Version command ---

func TestE2E_Version(t *testing.T) {
	var stdout bytes.Buffer
	cmd := exec.Command(fastVIPBinary, "-v")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		t.Fatalf("fastVIP -v failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Version:") {
		t.Errorf("expected output to contain 'Version:', got %q", output)
	}
}
