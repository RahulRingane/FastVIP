//go:build integration

package trafficlog

import (
	"net"
	"syscall"
	"testing"

	"github.com/RahulRingane/FastVIP/pkg/lvs"
	"go.uber.org/zap"
)

// newFlushedLVSManager creates an LVS Manager backed by the real Linux IPVS handle
// and flushes all existing rules to ensure a clean starting state.
func newFlushedLVSManager(t *testing.T) *lvs.Manager {
	t.Helper()
	mgr, err := lvs.NewManager(zap.NewNop())
	if err != nil {
		t.Fatalf("failed to create LVS manager: %v", err)
	}
	if err := mgr.Flush(); err != nil {
		t.Fatalf("failed to flush IPVS rules before test: %v", err)
	}
	t.Cleanup(func() {
		handle, err := lvs.NewIPVSHandle("")
		if err == nil {
			handle.Flush()
			handle.Close()
		}
		mgr.Close()
	})
	return mgr
}

// TestLVSStatsAdapter_ServiceStats_Integration verifies that the adapter correctly
// maps IPVS services to ServiceTrafficStats on real Linux IPVS.
// Stats values are not asserted because the kernel initialises them to zero
// for newly created services and they only accumulate from real traffic.
func TestLVSStatsAdapter_ServiceStats_Integration(t *testing.T) {
	mgr := newFlushedLVSManager(t)

	svc := &lvs.Service{
		Address:       net.ParseIP("10.0.0.1").To4(),
		Port:          80,
		Protocol:      syscall.IPPROTO_TCP,
		SchedName:     "rr",
		AddressFamily: syscall.AF_INET,
		Netmask:       0xFFFFFFFF,
	}
	if err := mgr.CreateService(svc); err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	adapter := NewLVSStatsAdapter(mgr)
	stats, err := adapter.ServiceStats()
	if err != nil {
		t.Fatalf("ServiceStats() error: %v", err)
	}

	if len(stats) != 1 {
		t.Fatalf("expected 1 service, got %d", len(stats))
	}

	expectedKey := "10.0.0.1:80/tcp"
	if _, ok := stats[expectedKey]; !ok {
		for k := range stats {
			t.Logf("actual key: %q", k)
		}
		t.Fatalf("expected key %q not found", expectedKey)
	}
}

// TestLVSStatsAdapter_BackendStats_Integration verifies that the adapter correctly
// maps IPVS destinations to BackendTrafficStats on real Linux IPVS.
func TestLVSStatsAdapter_BackendStats_Integration(t *testing.T) {
	mgr := newFlushedLVSManager(t)

	svc := &lvs.Service{
		Address:       net.ParseIP("10.0.0.1").To4(),
		Port:          80,
		Protocol:      syscall.IPPROTO_TCP,
		SchedName:     "rr",
		AddressFamily: syscall.AF_INET,
		Netmask:       0xFFFFFFFF,
	}
	if err := mgr.CreateService(svc); err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	dst := &lvs.Destination{
		Address:         net.ParseIP("192.168.1.1").To4(),
		Port:            8080,
		Weight:          1,
		ConnectionFlags: lvs.ConnectionFlagMasq,
		AddressFamily:   syscall.AF_INET,
	}
	if err := mgr.CreateDestination(svc, dst); err != nil {
		t.Fatalf("failed to create destination: %v", err)
	}

	adapter := NewLVSStatsAdapter(mgr)
	stats, err := adapter.BackendStats()
	if err != nil {
		t.Fatalf("BackendStats() error: %v", err)
	}

	if len(stats) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(stats))
	}

	expectedKey := "10.0.0.1:80/tcp->192.168.1.1:8080"
	backendStats, ok := stats[expectedKey]
	if !ok {
		for k := range stats {
			t.Logf("actual key: %q", k)
		}
		t.Fatalf("expected key %q not found", expectedKey)
	}

	if backendStats.ServiceKey != "10.0.0.1:80/tcp" {
		t.Errorf("expected ServiceKey='10.0.0.1:80/tcp', got %q", backendStats.ServiceKey)
	}
}

// TestLVSStatsAdapter_EmptyServices_Integration verifies that the adapter returns
// empty maps when no IPVS services exist.
func TestLVSStatsAdapter_EmptyServices_Integration(t *testing.T) {
	mgr := newFlushedLVSManager(t)

	adapter := NewLVSStatsAdapter(mgr)

	svcStats, err := adapter.ServiceStats()
	if err != nil {
		t.Fatalf("ServiceStats() error: %v", err)
	}
	if len(svcStats) != 0 {
		t.Errorf("expected 0 services, got %d", len(svcStats))
	}

	backendStats, err := adapter.BackendStats()
	if err != nil {
		t.Fatalf("BackendStats() error: %v", err)
	}
	if len(backendStats) != 0 {
		t.Errorf("expected 0 backends, got %d", len(backendStats))
	}
}

// TestLVSStatsAdapter_MultipleServicesAndBackends_Integration verifies that the adapter
// correctly handles multiple services and backends on real Linux IPVS.
func TestLVSStatsAdapter_MultipleServicesAndBackends_Integration(t *testing.T) {
	mgr := newFlushedLVSManager(t)

	svc1 := &lvs.Service{
		Address:       net.ParseIP("10.0.0.1").To4(),
		Port:          80,
		Protocol:      syscall.IPPROTO_TCP,
		SchedName:     "rr",
		AddressFamily: syscall.AF_INET,
		Netmask:       0xFFFFFFFF,
	}
	svc2 := &lvs.Service{
		Address:       net.ParseIP("10.0.0.2").To4(),
		Port:          443,
		Protocol:      syscall.IPPROTO_TCP,
		SchedName:     "wrr",
		AddressFamily: syscall.AF_INET,
		Netmask:       0xFFFFFFFF,
	}

	if err := mgr.CreateService(svc1); err != nil {
		t.Fatalf("failed to create service1: %v", err)
	}
	if err := mgr.CreateService(svc2); err != nil {
		t.Fatalf("failed to create service2: %v", err)
	}

	dst1 := &lvs.Destination{
		Address:         net.ParseIP("192.168.1.1").To4(),
		Port:            8080,
		Weight:          1,
		ConnectionFlags: lvs.ConnectionFlagMasq,
		AddressFamily:   syscall.AF_INET,
	}
	dst2 := &lvs.Destination{
		Address:         net.ParseIP("192.168.1.2").To4(),
		Port:            8080,
		Weight:          1,
		ConnectionFlags: lvs.ConnectionFlagMasq,
		AddressFamily:   syscall.AF_INET,
	}
	if err := mgr.CreateDestination(svc1, dst1); err != nil {
		t.Fatalf("failed to create destination1: %v", err)
	}
	if err := mgr.CreateDestination(svc1, dst2); err != nil {
		t.Fatalf("failed to create destination2: %v", err)
	}

	adapter := NewLVSStatsAdapter(mgr)

	svcStats, err := adapter.ServiceStats()
	if err != nil {
		t.Fatalf("ServiceStats() error: %v", err)
	}
	if len(svcStats) != 2 {
		t.Fatalf("expected 2 services, got %d", len(svcStats))
	}

	backendStats, err := adapter.BackendStats()
	if err != nil {
		t.Fatalf("BackendStats() error: %v", err)
	}
	if len(backendStats) != 2 {
		t.Fatalf("expected 2 backends, got %d", len(backendStats))
	}
}
