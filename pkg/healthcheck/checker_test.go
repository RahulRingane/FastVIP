package healthcheck

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTCPChecker_ConnectionSuccess(t *testing.T) {
	// Start a local TCP listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start TCP listener: %v", err)
	}
	defer listener.Close()

	// Accept connections in background to avoid blocking
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	checker := NewTCPChecker(3 * time.Second)
	if err := checker.Check(listener.Addr().String()); err != nil {
		t.Fatalf("expected successful health check, got error: %v", err)
	}
}

func TestTCPChecker_ConnectionRefused(t *testing.T) {
	// Use a port that is very unlikely to be in use
	checker := NewTCPChecker(1 * time.Second)
	err := checker.Check("127.0.0.1:1")
	if err == nil {
		t.Fatal("expected error for connection refused, got nil")
	}
}

func TestTCPChecker_Timeout(t *testing.T) {
	// Create a listener that accepts but never responds (simulates slow server)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start TCP listener: %v", err)
	}
	defer listener.Close()

	// Use an unreachable address with very short timeout to test timeout behavior
	checker := NewTCPChecker(50 * time.Millisecond)
	// 192.0.2.1 is a TEST-NET address (RFC 5737) that should be unreachable
	err = checker.Check("192.0.2.1:80")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestNewTCPChecker(t *testing.T) {
	timeout := 5 * time.Second
	checker := NewTCPChecker(timeout)
	if checker == nil {
		t.Fatal("expected non-nil checker")
	}
	if checker.timeout != timeout {
		t.Errorf("expected timeout %v, got %v", timeout, checker.timeout)
	}
}

// --- HTTPChecker tests ---

func TestHTTPChecker_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	// Extract host:port from server URL (strip "http://")
	address := server.Listener.Addr().String()
	checker := NewHTTPChecker(3*time.Second, "/healthz", 200)
	if err := checker.Check(address); err != nil {
		t.Fatalf("expected successful HTTP health check, got error: %v", err)
	}
}

func TestHTTPChecker_UnexpectedStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	address := server.Listener.Addr().String()
	checker := NewHTTPChecker(3*time.Second, "/healthz", 200)
	err := checker.Check(address)
	if err == nil {
		t.Fatal("expected error for unexpected HTTP status, got nil")
	}
}

func TestHTTPChecker_ConnectionRefused(t *testing.T) {
	checker := NewHTTPChecker(1*time.Second, "/healthz", 200)
	err := checker.Check("127.0.0.1:1")
	if err == nil {
		t.Fatal("expected error for connection refused, got nil")
	}
}

func TestHTTPChecker_CustomPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/custom/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// Return 404 for other paths
	server := httptest.NewServer(mux)
	defer server.Close()

	address := server.Listener.Addr().String()

	// Check with correct path should succeed
	checker := NewHTTPChecker(3*time.Second, "/custom/health", 200)
	if err := checker.Check(address); err != nil {
		t.Fatalf("expected successful check with custom path, got error: %v", err)
	}

	// Check with wrong path should fail (404 != 200)
	wrongPathChecker := NewHTTPChecker(3*time.Second, "/wrong/path", 200)
	if err := wrongPathChecker.Check(address); err == nil {
		t.Fatal("expected error for wrong path (404), got nil")
	}
}

func TestHTTPChecker_Timeout(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	address := server.Listener.Addr().String()
	checker := NewHTTPChecker(50*time.Millisecond, "/slow", 200)
	err := checker.Check(address)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestNewHTTPChecker(t *testing.T) {
	checker := NewHTTPChecker(5*time.Second, "/health", 200)
	if checker == nil {
		t.Fatal("expected non-nil checker")
	}
	if checker.path != "/health" {
		t.Errorf("expected path '/health', got %q", checker.path)
	}
	if checker.expectedStatus != 200 {
		t.Errorf("expected status 200, got %d", checker.expectedStatus)
	}
	if checker.client.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", checker.client.Timeout)
	}
}
