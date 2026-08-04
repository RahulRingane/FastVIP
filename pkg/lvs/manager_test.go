package lvs

import (
	"net"
	"testing"
)

func TestManager_CreateService_Success(t *testing.T) {
	mgr := newTestManager(t)
	defer mgr.Close()

	svc := newTestService("10.0.0.1", 80, 6, "rr")
	if err := mgr.CreateService(svc); err != nil {
		t.Fatalf("CreateService failed: %v", err)
	}

	services, err := mgr.GetServices()
	if err != nil {
		t.Fatalf("GetServices failed: %v", err)
	}
	if len(services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(services))
	}
	if services[0].Port != 80 {
		t.Errorf("expected port 80, got %d", services[0].Port)
	}
}

func TestManager_CreateService_Duplicate(t *testing.T) {
	mgr := newTestManager(t)
	defer mgr.Close()

	svc := newTestService("10.0.0.1", 80, 6, "rr")
	if err := mgr.CreateService(svc); err != nil {
		t.Fatalf("first CreateService failed: %v", err)
	}

	if err := mgr.CreateService(svc); err == nil {
		t.Fatal("expected error on duplicate CreateService, got nil")
	}
}

func TestManager_UpdateService_Success(t *testing.T) {
	mgr := newTestManager(t)
	defer mgr.Close()

	svc := newTestService("10.0.0.1", 80, 6, "rr")
	if err := mgr.CreateService(svc); err != nil {
		t.Fatalf("CreateService failed: %v", err)
	}

	updated := newTestService("10.0.0.1", 80, 6, "wrr")
	if err := mgr.UpdateService(updated); err != nil {
		t.Fatalf("UpdateService failed: %v", err)
	}

	services, err := mgr.GetServices()
	if err != nil {
		t.Fatalf("GetServices failed: %v", err)
	}
	if len(services) != 1 {
		t.Fatalf("expected 1 service after update, got %d", len(services))
	}
	if services[0].SchedName != "wrr" {
		t.Errorf("expected scheduler 'wrr', got %q", services[0].SchedName)
	}
}

func TestManager_UpdateService_NonExistent(t *testing.T) {
	mgr := newTestManager(t)
	defer mgr.Close()

	svc := newTestService("10.0.0.1", 80, 6, "rr")
	if err := mgr.UpdateService(svc); err == nil {
		t.Fatal("expected error on updating non-existent service, got nil")
	}
}

func TestManager_DeleteService_Success(t *testing.T) {
	mgr := newTestManager(t)
	defer mgr.Close()

	svc := newTestService("10.0.0.1", 80, 6, "rr")
	if err := mgr.CreateService(svc); err != nil {
		t.Fatalf("CreateService failed: %v", err)
	}

	if err := mgr.DeleteService(svc); err != nil {
		t.Fatalf("DeleteService failed: %v", err)
	}

	services, err := mgr.GetServices()
	if err != nil {
		t.Fatalf("GetServices failed: %v", err)
	}
	if len(services) != 0 {
		t.Fatalf("expected 0 services after delete, got %d", len(services))
	}
}

func TestManager_DeleteService_NonExistent(t *testing.T) {
	mgr := newTestManager(t)
	defer mgr.Close()

	svc := newTestService("10.0.0.1", 80, 6, "rr")
	if err := mgr.DeleteService(svc); err == nil {
		t.Fatal("expected error on deleting non-existent service, got nil")
	}
}

func TestManager_CreateDestination_Success(t *testing.T) {
	mgr := newTestManager(t)
	defer mgr.Close()

	svc := newTestService("10.0.0.1", 80, 6, "rr")
	if err := mgr.CreateService(svc); err != nil {
		t.Fatalf("CreateService failed: %v", err)
	}

	dst := newTestDestination("192.168.1.1", 8080, 5)
	if err := mgr.CreateDestination(svc, dst); err != nil {
		t.Fatalf("CreateDestination failed: %v", err)
	}

	destinations, err := mgr.GetDestinations(svc)
	if err != nil {
		t.Fatalf("GetDestinations failed: %v", err)
	}
	if len(destinations) != 1 {
		t.Fatalf("expected 1 destination, got %d", len(destinations))
	}
	if destinations[0].Weight != 5 {
		t.Errorf("expected weight 5, got %d", destinations[0].Weight)
	}
}

func TestManager_UpdateDestination_Success(t *testing.T) {
	mgr := newTestManager(t)
	defer mgr.Close()

	svc := newTestService("10.0.0.1", 80, 6, "rr")
	if err := mgr.CreateService(svc); err != nil {
		t.Fatalf("CreateService failed: %v", err)
	}

	dst := newTestDestination("192.168.1.1", 8080, 5)
	if err := mgr.CreateDestination(svc, dst); err != nil {
		t.Fatalf("CreateDestination failed: %v", err)
	}

	updatedDst := newTestDestination("192.168.1.1", 8080, 10)
	if err := mgr.UpdateDestination(svc, updatedDst); err != nil {
		t.Fatalf("UpdateDestination failed: %v", err)
	}

	destinations, err := mgr.GetDestinations(svc)
	if err != nil {
		t.Fatalf("GetDestinations failed: %v", err)
	}
	if destinations[0].Weight != 10 {
		t.Errorf("expected weight 10 after update, got %d", destinations[0].Weight)
	}
}

func TestManager_DeleteDestination_Success(t *testing.T) {
	mgr := newTestManager(t)
	defer mgr.Close()

	svc := newTestService("10.0.0.1", 80, 6, "rr")
	if err := mgr.CreateService(svc); err != nil {
		t.Fatalf("CreateService failed: %v", err)
	}

	dst := newTestDestination("192.168.1.1", 8080, 5)
	if err := mgr.CreateDestination(svc, dst); err != nil {
		t.Fatalf("CreateDestination failed: %v", err)
	}

	if err := mgr.DeleteDestination(svc, dst); err != nil {
		t.Fatalf("DeleteDestination failed: %v", err)
	}

	destinations, err := mgr.GetDestinations(svc)
	if err != nil {
		t.Fatalf("GetDestinations failed: %v", err)
	}
	if len(destinations) != 0 {
		t.Fatalf("expected 0 destinations after delete, got %d", len(destinations))
	}
}

func TestManager_MultiServiceMultiDestination_Isolation(t *testing.T) {
	mgr := newTestManager(t)
	defer mgr.Close()

	svc1 := newTestService("10.0.0.1", 80, 6, "rr")
	svc2 := newTestService("10.0.0.2", 443, 6, "wrr")

	if err := mgr.CreateService(svc1); err != nil {
		t.Fatalf("CreateService svc1 failed: %v", err)
	}
	if err := mgr.CreateService(svc2); err != nil {
		t.Fatalf("CreateService svc2 failed: %v", err)
	}

	// Add 2 destinations to svc1
	dst1a := newTestDestination("192.168.1.1", 8080, 5)
	dst1b := newTestDestination("192.168.1.2", 8080, 3)
	if err := mgr.CreateDestination(svc1, dst1a); err != nil {
		t.Fatalf("CreateDestination dst1a failed: %v", err)
	}
	if err := mgr.CreateDestination(svc1, dst1b); err != nil {
		t.Fatalf("CreateDestination dst1b failed: %v", err)
	}

	// Add 2 destinations to svc2
	dst2a := newTestDestination("192.168.2.1", 9090, 7)
	dst2b := newTestDestination("192.168.2.2", 9090, 2)
	if err := mgr.CreateDestination(svc2, dst2a); err != nil {
		t.Fatalf("CreateDestination dst2a failed: %v", err)
	}
	if err := mgr.CreateDestination(svc2, dst2b); err != nil {
		t.Fatalf("CreateDestination dst2b failed: %v", err)
	}

	// Verify services
	services, err := mgr.GetServices()
	if err != nil {
		t.Fatalf("GetServices failed: %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}

	// Verify svc1 destinations
	dests1, err := mgr.GetDestinations(svc1)
	if err != nil {
		t.Fatalf("GetDestinations svc1 failed: %v", err)
	}
	if len(dests1) != 2 {
		t.Fatalf("expected 2 destinations for svc1, got %d", len(dests1))
	}

	// Verify svc2 destinations
	dests2, err := mgr.GetDestinations(svc2)
	if err != nil {
		t.Fatalf("GetDestinations svc2 failed: %v", err)
	}
	if len(dests2) != 2 {
		t.Fatalf("expected 2 destinations for svc2, got %d", len(dests2))
	}

	// Delete svc1 should not affect svc2
	if err := mgr.DeleteService(svc1); err != nil {
		t.Fatalf("DeleteService svc1 failed: %v", err)
	}

	services, err = mgr.GetServices()
	if err != nil {
		t.Fatalf("GetServices after delete failed: %v", err)
	}
	if len(services) != 1 {
		t.Fatalf("expected 1 service after deleting svc1, got %d", len(services))
	}

	// svc2 destinations should still be intact
	dests2, err = mgr.GetDestinations(svc2)
	if err != nil {
		t.Fatalf("GetDestinations svc2 after svc1 delete failed: %v", err)
	}
	if len(dests2) != 2 {
		t.Fatalf("expected 2 destinations for svc2 after svc1 delete, got %d", len(dests2))
	}

	// Verify destination IPs for svc2
	destAddrs := make(map[string]bool)
	for _, d := range dests2 {
		destAddrs[net.IP.String(d.Address)] = true
	}
	if !destAddrs["192.168.2.1"] || !destAddrs["192.168.2.2"] {
		t.Errorf("svc2 destinations have unexpected addresses: %v", destAddrs)
	}
}
