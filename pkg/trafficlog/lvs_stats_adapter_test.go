//go:build !integration

package trafficlog

import (
	"net"
	"syscall"
	"testing"

	"github.com/RahulRingane/FastVIP/pkg/lvs"
	"go.uber.org/zap"
)

func TestLVSStatsAdapter_ServiceStats(t *testing.T) {
	// Create a Manager with fake handle and add a service with stats
	mgr, err := lvs.NewManager(zap.NewNop())
	if err != nil {
		t.Fatalf("failed to create LVS manager: %v", err)
	}
	defer mgr.Close()

	svc := &lvs.Service{
		Address:       net.ParseIP("10.0.0.1").To4(),
		Port:          80,
		Protocol:      syscall.IPPROTO_TCP,
		SchedName:     "rr",
		AddressFamily: syscall.AF_INET,
		Netmask:       0xFFFFFFFF,
		Stats: lvs.SvcStats{
			Connections: 100,
			PacketsIn:   200,
			PacketsOut:  150,
			BytesIn:     50000,
			BytesOut:    30000,
		},
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

	// The key format is "ip:port/protocol"
	expectedKey := "10.0.0.1:80/tcp"
	svcStats, ok := stats[expectedKey]
	if !ok {
		// Print actual keys for debugging
		for k := range stats {
			t.Logf("actual key: %q", k)
		}
		t.Fatalf("expected key %q not found", expectedKey)
	}

	// Note: fakeHandle stores the service but GetServices returns a clone.
	// The stats in the clone come from the original service's Stats field.
	// However, fakeHandle's cloneService copies Stats directly, so the values
	// should match what we set above.
	if svcStats.Connections != 100 {
		t.Errorf("expected Connections=100, got %d", svcStats.Connections)
	}
	if svcStats.InPkts != 200 {
		t.Errorf("expected InPkts=200, got %d", svcStats.InPkts)
	}
	if svcStats.OutPkts != 150 {
		t.Errorf("expected OutPkts=150, got %d", svcStats.OutPkts)
	}
	if svcStats.InBytes != 50000 {
		t.Errorf("expected InBytes=50000, got %d", svcStats.InBytes)
	}
	if svcStats.OutBytes != 30000 {
		t.Errorf("expected OutBytes=30000, got %d", svcStats.OutBytes)
	}
}

func TestLVSStatsAdapter_BackendStats(t *testing.T) {
	mgr, err := lvs.NewManager(zap.NewNop())
	if err != nil {
		t.Fatalf("failed to create LVS manager: %v", err)
	}
	defer mgr.Close()

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
		Address:             net.ParseIP("192.168.1.1").To4(),
		Port:                8080,
		Weight:              1,
		ConnectionFlags:     lvs.ConnectionFlagMasq,
		AddressFamily:       syscall.AF_INET,
		ActiveConnections:   7,
		InactiveConnections: 3,
		Stats: lvs.DstStats{
			Connections: 50,
			PacketsIn:   100,
			PacketsOut:  75,
			BytesIn:     25000,
			BytesOut:    15000,
		},
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

	// Key format: "svcKey->dstKey"
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
	if backendStats.Connections != 50 {
		t.Errorf("expected Connections=50, got %d", backendStats.Connections)
	}
	if backendStats.ActiveConnections != 7 {
		t.Errorf("expected ActiveConnections=7, got %d", backendStats.ActiveConnections)
	}
	if backendStats.InactiveConnections != 3 {
		t.Errorf("expected InactiveConnections=3, got %d", backendStats.InactiveConnections)
	}
	if backendStats.CurrentConnections != 10 {
		t.Errorf("expected CurrentConnections=10, got %d", backendStats.CurrentConnections)
	}
	if backendStats.InPkts != 100 {
		t.Errorf("expected InPkts=100, got %d", backendStats.InPkts)
	}
	if backendStats.OutPkts != 75 {
		t.Errorf("expected OutPkts=75, got %d", backendStats.OutPkts)
	}
	if backendStats.InBytes != 25000 {
		t.Errorf("expected InBytes=25000, got %d", backendStats.InBytes)
	}
	if backendStats.OutBytes != 15000 {
		t.Errorf("expected OutBytes=15000, got %d", backendStats.OutBytes)
	}
}

func TestLVSStatsAdapter_EmptyServices(t *testing.T) {
	mgr, err := lvs.NewManager(zap.NewNop())
	if err != nil {
		t.Fatalf("failed to create LVS manager: %v", err)
	}
	defer mgr.Close()

	adapter := NewLVSStatsAdapter(mgr)

	// ServiceStats should return empty map
	svcStats, err := adapter.ServiceStats()
	if err != nil {
		t.Fatalf("ServiceStats() error: %v", err)
	}
	if len(svcStats) != 0 {
		t.Errorf("expected 0 services, got %d", len(svcStats))
	}

	// BackendStats should return empty map
	backendStats, err := adapter.BackendStats()
	if err != nil {
		t.Fatalf("BackendStats() error: %v", err)
	}
	if len(backendStats) != 0 {
		t.Errorf("expected 0 backends, got %d", len(backendStats))
	}
}

func TestLVSStatsAdapter_MultipleServicesAndBackends(t *testing.T) {
	mgr, err := lvs.NewManager(zap.NewNop())
	if err != nil {
		t.Fatalf("failed to create LVS manager: %v", err)
	}
	defer mgr.Close()

	// Create two services
	svc1 := &lvs.Service{
		Address:       net.ParseIP("10.0.0.1").To4(),
		Port:          80,
		Protocol:      syscall.IPPROTO_TCP,
		SchedName:     "rr",
		AddressFamily: syscall.AF_INET,
		Netmask:       0xFFFFFFFF,
		Stats: lvs.SvcStats{
			Connections: 100,
			BytesIn:     50000,
		},
	}
	svc2 := &lvs.Service{
		Address:       net.ParseIP("10.0.0.2").To4(),
		Port:          443,
		Protocol:      syscall.IPPROTO_TCP,
		SchedName:     "wrr",
		AddressFamily: syscall.AF_INET,
		Netmask:       0xFFFFFFFF,
		Stats: lvs.SvcStats{
			Connections: 200,
			BytesIn:     100000,
		},
	}

	if err := mgr.CreateService(svc1); err != nil {
		t.Fatalf("failed to create service1: %v", err)
	}
	if err := mgr.CreateService(svc2); err != nil {
		t.Fatalf("failed to create service2: %v", err)
	}

	// Add backends to svc1
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

	// Verify service stats
	svcStats, err := adapter.ServiceStats()
	if err != nil {
		t.Fatalf("ServiceStats() error: %v", err)
	}
	if len(svcStats) != 2 {
		t.Fatalf("expected 2 services, got %d", len(svcStats))
	}

	// Verify backend stats
	backendStats, err := adapter.BackendStats()
	if err != nil {
		t.Fatalf("BackendStats() error: %v", err)
	}
	if len(backendStats) != 2 {
		t.Fatalf("expected 2 backends, got %d", len(backendStats))
	}
}
