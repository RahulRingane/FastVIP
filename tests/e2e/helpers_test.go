//go:build integration

package e2e

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/RahulRingane/FastVIP/pkg/lvs"
)

// runEzlbOnce executes `fastVIP once -c configPath` and asserts a successful exit.
// Returns the combined stdout and stderr output.
func runEzlbOnce(t *testing.T, configPath string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(fastVIPBinary, "once", "-c", configPath)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("fastVIP once failed: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	return stdout.String() + stderr.String()
}

// runEzlbOnceExpectFailure executes `fastVIP once -c configPath` and expects a non-zero exit code.
// Returns stdout, stderr, and the error.
func runEzlbOnceExpectFailure(t *testing.T, configPath string) (string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(fastVIPBinary, "once", "-c", configPath)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected fastVIP once to fail, but it succeeded\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
	}
	return stdout.String(), stderr.String()
}

// runEzlbVersion executes `fastVIP -v` and returns the output.
func runEzlbVersion(t *testing.T) string {
	t.Helper()
	var stdout bytes.Buffer
	cmd := exec.Command(fastVIPBinary, "-v")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		t.Fatalf("fastVIP -v failed: %v", err)
	}
	return stdout.String()
}

// runEzlbDaemon starts `fastVIP start -c configPath` in daemon mode and returns the exec.Cmd.
// The caller is responsible for stopping the process.
func runEzlbDaemon(t *testing.T, configPath string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(fastVIPBinary, "start", "-c", configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start fastVIP daemon: %v", err)
	}
	return cmd
}

// writeTestConfig writes YAML content to a config file in the given directory.
func writeTestConfig(t *testing.T, dir, content string) string {
	t.Helper()
	configPath := filepath.Join(dir, "fastVIP.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}
	return configPath
}

// flushIPVS removes all IPVS rules to ensure test isolation.
func flushIPVS(t *testing.T) {
	t.Helper()
	handle, err := lvs.NewIPVSHandle("")
	if err != nil {
		t.Fatalf("failed to create IPVS handle for flush: %v", err)
	}
	defer handle.Close()
	if err := handle.Flush(); err != nil {
		t.Fatalf("failed to flush IPVS rules: %v", err)
	}
}

// getIPVSServices returns all current IPVS services from the kernel.
func getIPVSServices(t *testing.T) []*lvs.Service {
	t.Helper()
	handle, err := lvs.NewIPVSHandle("")
	if err != nil {
		t.Fatalf("failed to create IPVS handle: %v", err)
	}
	defer handle.Close()

	services, err := handle.GetServices()
	if err != nil {
		t.Fatalf("failed to get IPVS services: %v", err)
	}
	return services
}

// getIPVSDestinations returns all destinations for the given IPVS service.
func getIPVSDestinations(t *testing.T, svc *lvs.Service) []*lvs.Destination {
	t.Helper()
	handle, err := lvs.NewIPVSHandle("")
	if err != nil {
		t.Fatalf("failed to create IPVS handle: %v", err)
	}
	defer handle.Close()

	destinations, err := handle.GetDestinations(svc)
	if err != nil {
		t.Fatalf("failed to get IPVS destinations: %v", err)
	}
	return destinations
}

// findServiceByAddress finds an IPVS service matching the given IP and port.
// Returns nil if not found.
func findServiceByAddress(services []*lvs.Service, ipAddress string, port uint16) *lvs.Service {
	targetIP := net.ParseIP(ipAddress)
	for _, svc := range services {
		if svc.Address.Equal(targetIP) && svc.Port == port {
			return svc
		}
	}
	return nil
}

// requireServiceCount asserts the exact number of IPVS services.
func requireServiceCount(t *testing.T, expected int) []*lvs.Service {
	t.Helper()
	services := getIPVSServices(t)
	if len(services) != expected {
		t.Fatalf("expected %d IPVS services, got %d", expected, len(services))
	}
	return services
}

// requireDestinationCount asserts the exact number of destinations for a service.
func requireDestinationCount(t *testing.T, svc *lvs.Service, expected int) []*lvs.Destination {
	t.Helper()
	destinations := getIPVSDestinations(t, svc)
	if len(destinations) != expected {
		t.Fatalf("expected %d destinations for service %s:%d, got %d",
			expected, svc.Address, svc.Port, len(destinations))
	}
	return destinations
}
