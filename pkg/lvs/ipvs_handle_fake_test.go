//go:build !integration

package lvs

import (
	"sync"
	"testing"
)

func TestFakeHandle_NewAndGetServices(t *testing.T) {
	handle, err := NewIPVSHandle("")
	if err != nil {
		t.Fatalf("NewIPVSHandle failed: %v", err)
	}
	defer handle.Close()

	svc := newTestService("10.0.0.1", 80, 6, "rr")

	if err := handle.NewService(svc); err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	services, err := handle.GetServices()
	if err != nil {
		t.Fatalf("GetServices failed: %v", err)
	}

	if len(services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(services))
	}

	if services[0].Port != 80 {
		t.Errorf("expected port 80, got %d", services[0].Port)
	}
	if services[0].SchedName != "rr" {
		t.Errorf("expected scheduler rr, got %s", services[0].SchedName)
	}
}

func TestFakeHandle_DuplicateServiceCreation(t *testing.T) {
	handle, err := NewIPVSHandle("")
	if err != nil {
		t.Fatalf("NewIPVSHandle failed: %v", err)
	}
	defer handle.Close()

	svc := newTestService("10.0.0.1", 80, 6, "rr")

	if err := handle.NewService(svc); err != nil {
		t.Fatalf("first NewService failed: %v", err)
	}

	if err := handle.NewService(svc); err == nil {
		t.Fatal("expected error on duplicate service creation, got nil")
	}
}

func TestFakeHandle_UpdateService(t *testing.T) {
	handle, err := NewIPVSHandle("")
	if err != nil {
		t.Fatalf("NewIPVSHandle failed: %v", err)
	}
	defer handle.Close()

	svc := newTestService("10.0.0.1", 80, 6, "rr")
	if err := handle.NewService(svc); err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	updated := newTestService("10.0.0.1", 80, 6, "wrr")
	if err := handle.UpdateService(updated); err != nil {
		t.Fatalf("UpdateService failed: %v", err)
	}

	services, err := handle.GetServices()
	if err != nil {
		t.Fatalf("GetServices failed: %v", err)
	}

	if services[0].SchedName != "wrr" {
		t.Errorf("expected scheduler wrr after update, got %s", services[0].SchedName)
	}
}

func TestFakeHandle_UpdateNonExistentService(t *testing.T) {
	handle, err := NewIPVSHandle("")
	if err != nil {
		t.Fatalf("NewIPVSHandle failed: %v", err)
	}
	defer handle.Close()

	svc := newTestService("10.0.0.1", 80, 6, "rr")
	if err := handle.UpdateService(svc); err == nil {
		t.Fatal("expected error on updating non-existent service, got nil")
	}
}

func TestFakeHandle_DeleteService(t *testing.T) {
	handle, err := NewIPVSHandle("")
	if err != nil {
		t.Fatalf("NewIPVSHandle failed: %v", err)
	}
	defer handle.Close()

	svc := newTestService("10.0.0.1", 80, 6, "rr")
	if err := handle.NewService(svc); err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	if err := handle.DelService(svc); err != nil {
		t.Fatalf("DelService failed: %v", err)
	}

	services, err := handle.GetServices()
	if err != nil {
		t.Fatalf("GetServices failed: %v", err)
	}

	if len(services) != 0 {
		t.Fatalf("expected 0 services after delete, got %d", len(services))
	}
}

func TestFakeHandle_DeleteNonExistentService(t *testing.T) {
	handle, err := NewIPVSHandle("")
	if err != nil {
		t.Fatalf("NewIPVSHandle failed: %v", err)
	}
	defer handle.Close()

	svc := newTestService("10.0.0.1", 80, 6, "rr")
	if err := handle.DelService(svc); err == nil {
		t.Fatal("expected error on deleting non-existent service, got nil")
	}
}

func TestFakeHandle_DestinationCRUD(t *testing.T) {
	handle, err := NewIPVSHandle("")
	if err != nil {
		t.Fatalf("NewIPVSHandle failed: %v", err)
	}
	defer handle.Close()

	svc := newTestService("10.0.0.1", 80, 6, "rr")
	if err := handle.NewService(svc); err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	dst := newTestDestination("192.168.1.1", 8080, 100)

	// Create destination
	if err := handle.NewDestination(svc, dst); err != nil {
		t.Fatalf("NewDestination failed: %v", err)
	}

	// Get destinations
	destinations, err := handle.GetDestinations(svc)
	if err != nil {
		t.Fatalf("GetDestinations failed: %v", err)
	}
	if len(destinations) != 1 {
		t.Fatalf("expected 1 destination, got %d", len(destinations))
	}
	if destinations[0].Weight != 100 {
		t.Errorf("expected weight 100, got %d", destinations[0].Weight)
	}

	// Update destination
	updatedDst := newTestDestination("192.168.1.1", 8080, 200)
	if err := handle.UpdateDestination(svc, updatedDst); err != nil {
		t.Fatalf("UpdateDestination failed: %v", err)
	}

	destinations, err = handle.GetDestinations(svc)
	if err != nil {
		t.Fatalf("GetDestinations failed: %v", err)
	}
	if destinations[0].Weight != 200 {
		t.Errorf("expected weight 200 after update, got %d", destinations[0].Weight)
	}

	// Delete destination
	if err := handle.DelDestination(svc, dst); err != nil {
		t.Fatalf("DelDestination failed: %v", err)
	}

	destinations, err = handle.GetDestinations(svc)
	if err != nil {
		t.Fatalf("GetDestinations failed: %v", err)
	}
	if len(destinations) != 0 {
		t.Fatalf("expected 0 destinations after delete, got %d", len(destinations))
	}
}

func TestFakeHandle_DuplicateDestination(t *testing.T) {
	handle, err := NewIPVSHandle("")
	if err != nil {
		t.Fatalf("NewIPVSHandle failed: %v", err)
	}
	defer handle.Close()

	svc := newTestService("10.0.0.1", 80, 6, "rr")
	if err := handle.NewService(svc); err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	dst := newTestDestination("192.168.1.1", 8080, 100)
	if err := handle.NewDestination(svc, dst); err != nil {
		t.Fatalf("first NewDestination failed: %v", err)
	}

	if err := handle.NewDestination(svc, dst); err == nil {
		t.Fatal("expected error on duplicate destination creation, got nil")
	}
}

func TestFakeHandle_DestinationOnNonExistentService(t *testing.T) {
	handle, err := NewIPVSHandle("")
	if err != nil {
		t.Fatalf("NewIPVSHandle failed: %v", err)
	}
	defer handle.Close()

	svc := newTestService("10.0.0.1", 80, 6, "rr")
	dst := newTestDestination("192.168.1.1", 8080, 100)

	if err := handle.NewDestination(svc, dst); err == nil {
		t.Fatal("expected error on adding destination to non-existent service, got nil")
	}

	if _, err := handle.GetDestinations(svc); err == nil {
		t.Fatal("expected error on getting destinations from non-existent service, got nil")
	}
}

func TestFakeHandle_Flush(t *testing.T) {
	handle, err := NewIPVSHandle("")
	if err != nil {
		t.Fatalf("NewIPVSHandle failed: %v", err)
	}
	defer handle.Close()

	// Create multiple services with destinations
	for i := 1; i <= 3; i++ {
		svc := newTestService("10.0.0.1", uint16(80+i), 6, "rr")
		if err := handle.NewService(svc); err != nil {
			t.Fatalf("NewService failed: %v", err)
		}
		dst := newTestDestination("192.168.1.1", uint16(8080+i), 100)
		if err := handle.NewDestination(svc, dst); err != nil {
			t.Fatalf("NewDestination failed: %v", err)
		}
	}

	services, err := handle.GetServices()
	if err != nil {
		t.Fatalf("GetServices failed: %v", err)
	}
	if len(services) != 3 {
		t.Fatalf("expected 3 services before flush, got %d", len(services))
	}

	if err := handle.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	services, err = handle.GetServices()
	if err != nil {
		t.Fatalf("GetServices failed: %v", err)
	}
	if len(services) != 0 {
		t.Fatalf("expected 0 services after flush, got %d", len(services))
	}
}

func TestFakeHandle_DeleteServiceCascadesDestinations(t *testing.T) {
	handle, err := NewIPVSHandle("")
	if err != nil {
		t.Fatalf("NewIPVSHandle failed: %v", err)
	}
	defer handle.Close()

	svc := newTestService("10.0.0.1", 80, 6, "rr")
	if err := handle.NewService(svc); err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	dst := newTestDestination("192.168.1.1", 8080, 100)
	if err := handle.NewDestination(svc, dst); err != nil {
		t.Fatalf("NewDestination failed: %v", err)
	}

	if err := handle.DelService(svc); err != nil {
		t.Fatalf("DelService failed: %v", err)
	}

	// After deleting the service, destinations should also be gone
	if _, err := handle.GetDestinations(svc); err == nil {
		t.Fatal("expected error on getting destinations after service deletion, got nil")
	}
}

func TestFakeHandle_MultipleDestinations(t *testing.T) {
	handle, err := NewIPVSHandle("")
	if err != nil {
		t.Fatalf("NewIPVSHandle failed: %v", err)
	}
	defer handle.Close()

	svc := newTestService("10.0.0.1", 80, 6, "wrr")
	if err := handle.NewService(svc); err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	for i := 1; i <= 5; i++ {
		dst := newTestDestination("192.168.1."+string(rune('0'+i)), uint16(8080+i), i*10)
		if err := handle.NewDestination(svc, dst); err != nil {
			t.Fatalf("NewDestination %d failed: %v", i, err)
		}
	}

	destinations, err := handle.GetDestinations(svc)
	if err != nil {
		t.Fatalf("GetDestinations failed: %v", err)
	}
	if len(destinations) != 5 {
		t.Fatalf("expected 5 destinations, got %d", len(destinations))
	}
}

func TestFakeHandle_ConcurrentAccess(t *testing.T) {
	handle, err := NewIPVSHandle("")
	if err != nil {
		t.Fatalf("NewIPVSHandle failed: %v", err)
	}
	defer handle.Close()

	var waitGroup sync.WaitGroup
	concurrency := 20

	// Concurrently create services
	for i := 0; i < concurrency; i++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			svc := newTestService("10.0.0.1", uint16(8000+index), 6, "rr")
			_ = handle.NewService(svc)
		}(i)
	}
	waitGroup.Wait()

	services, err := handle.GetServices()
	if err != nil {
		t.Fatalf("GetServices failed: %v", err)
	}
	if len(services) != concurrency {
		t.Fatalf("expected %d services, got %d", concurrency, len(services))
	}

	// Concurrently add destinations to the first service
	targetService := newTestService("10.0.0.1", 8000, 6, "rr")
	for i := 0; i < concurrency; i++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			dst := newTestDestination("192.168.1.1", uint16(9000+index), 100)
			_ = handle.NewDestination(targetService, dst)
		}(i)
	}
	waitGroup.Wait()

	destinations, err := handle.GetDestinations(targetService)
	if err != nil {
		t.Fatalf("GetDestinations failed: %v", err)
	}
	if len(destinations) != concurrency {
		t.Fatalf("expected %d destinations, got %d", concurrency, len(destinations))
	}
}
