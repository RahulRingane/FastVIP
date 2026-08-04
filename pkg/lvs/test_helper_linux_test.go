//go:build integration

package lvs

import (
	"net"
	"testing"

	"go.uber.org/zap"
)

// newTestManager creates a Manager backed by the real Linux IPVS handle.
// Tests must run serially (go test -p 1) because IPVS is a global kernel resource.
// TestMain handles the initial Flush; each test flushes before and after via Cleanup.
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	mgr, err := NewManager(zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	// Flush existing IPVS rules to ensure a clean starting state
	if err := mgr.Flush(); err != nil {
		t.Fatalf("failed to flush IPVS rules before test: %v", err)
	}
	// Register cleanup to flush after test completes.
	// Use a separate handle because the test may call mgr.Close()
	// via defer before t.Cleanup runs (Go executes defers before Cleanup).
	t.Cleanup(func() {
		cleanupHandle, err := NewIPVSHandle("")
		if err == nil {
			cleanupHandle.Flush()
			cleanupHandle.Close()
		}
	})
	return mgr
}

func newTestService(address string, port uint16, protocol uint16, scheduler string) *Service {
	ip := net.ParseIP(address)
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}
	return &Service{
		Address:       ip,
		Protocol:      protocol,
		Port:          port,
		SchedName:     scheduler,
		AddressFamily: 2, // AF_INET
		Netmask:       0xFFFFFFFF,
	}
}

func newTestDestination(address string, port uint16, weight int) *Destination {
	ip := net.ParseIP(address)
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}
	return &Destination{
		Address:         ip,
		Port:            port,
		Weight:          weight,
		ConnectionFlags: ConnectionFlagMasq,
		AddressFamily:   2, // AF_INET
	}
}
