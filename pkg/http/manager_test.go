package http

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPManager_ProxyAndRoundRobin(t *testing.T) {
	// Backend 1
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "backend-1")
	}))
	defer backend1.Close()

	// Backend 2
	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "backend-2")
	}))
	defer backend2.Close()

	// Create HTTP service
	service := &Service{
		Name:   "test",
		Listen: "127.0.0.1:0",
		Backends: []string{
			backend1.Listener.Addr().String(),
			backend2.Listener.Addr().String(),
		},
	}

	// Create manager
	manager := NewManager()

	if err := manager.AddService(service); err != nil {
		t.Fatalf("failed to add service: %v", err)
	}

	// Start HTTP load balancer
	if err := manager.StartService(service.Name); err != nil {
		t.Fatalf("failed to start service: %v", err)
	}
	defer manager.StopService(service.Name)

	// Get actual address selected by OS
	addr, err := manager.ServiceAddr(service.Name)
	if err != nil {
		t.Fatalf("failed to get service address: %v", err)
	}

	// Send requests through the load balancer
	client := &http.Client{}

	expected := []string{
		"backend-1",
		"backend-2",
		"backend-1",
		"backend-2",
	}

	for i, want := range expected {
		resp, err := client.Get("http://" + addr)
		if err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}

		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("request %d failed reading response: %v", i+1, err)
		}

		got := string(body)

		if got != want {
			t.Errorf(
				"request %d: expected %q, got %q",
				i+1,
				want,
				got,
			)
		}
	}
}
