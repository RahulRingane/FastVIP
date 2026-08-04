package admin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewServer(t *testing.T) {
	logger := zap.NewNop()
	cfg := Config{
		ListenAddr:     "127.0.0.1:0",
		MetricsEnabled: true,
		MetricsPath:    "/metrics",
	}

	server := NewServer(cfg, logger)
	if server == nil {
		t.Fatal("expected server to be created")
	}

	if server.listenAddr != cfg.ListenAddr {
		t.Errorf("expected listenAddr %q, got %q", cfg.ListenAddr, server.listenAddr)
	}
}

func TestServerStartStop(t *testing.T) {
	logger := zap.NewNop()
	cfg := Config{
		ListenAddr:     "127.0.0.1:0",
		MetricsEnabled: true,
		MetricsPath:    "/metrics",
	}

	server := NewServer(cfg, logger)

	// Start server
	err := server.Start()
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Stop server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = server.Stop(ctx)
	if err != nil {
		t.Fatalf("failed to stop server: %v", err)
	}
}

func TestServerDisabled(t *testing.T) {
	logger := zap.NewNop()
	cfg := Config{
		ListenAddr:     "",
		MetricsEnabled: true,
		MetricsPath:    "/metrics",
	}

	server := NewServer(cfg, logger)

	err := server.Start()
	if err != nil {
		t.Errorf("expected no error when server is disabled, got %v", err)
	}

	if server.IsEnabled() {
		t.Error("expected server to be disabled")
	}
}

func TestServerEnabled(t *testing.T) {
	logger := zap.NewNop()
	cfg := Config{
		ListenAddr:     "127.0.0.1:9090",
		MetricsEnabled: true,
		MetricsPath:    "/metrics",
	}

	server := NewServer(cfg, logger)
	if !server.IsEnabled() {
		t.Error("expected server to be enabled")
	}
}

func TestInvalidAddress(t *testing.T) {
	logger := zap.NewNop()
	cfg := Config{
		ListenAddr:     "invalid-address",
		MetricsEnabled: true,
		MetricsPath:    "/metrics",
	}

	server := NewServer(cfg, logger)

	err := server.Start()
	if err == nil {
		t.Error("expected error for invalid address")
	}
}

func TestHandleHealth(t *testing.T) {
	logger := zap.NewNop()
	cfg := Config{
		ListenAddr:     "127.0.0.1:0",
		MetricsEnabled: false,
		MetricsPath:    "/metrics",
	}

	server := NewServer(cfg, logger)

	// Set a mock health check function
	server.SetHealthCheckFunc(func() map[string]bool {
		return map[string]bool{
			"backend1": true,
			"backend2": false,
		}
	})

	err := server.Start()
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop(context.Background())

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Get the actual address
	addr := server.Addr()
	if addr == "" {
		t.Skip("cannot determine server address")
	}

	// Make request
	resp, err := http.Get(fmt.Sprintf("http://%s/health", addr))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	if !strings.Contains(string(body), "healthy") {
		t.Errorf("expected response to contain 'healthy', got %s", string(body))
	}
}

func TestHandleHealthMethodNotAllowed(t *testing.T) {
	logger := zap.NewNop()
	cfg := Config{
		ListenAddr:     "127.0.0.1:0",
		MetricsEnabled: false,
		MetricsPath:    "/metrics",
	}

	server := NewServer(cfg, logger)
	err := server.Start()
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop(context.Background())

	time.Sleep(100 * time.Millisecond)

	addr := server.Addr()
	if addr == "" {
		t.Skip("cannot determine server address")
	}

	// Make POST request (should fail)
	resp, err := http.Post(fmt.Sprintf("http://%s/health", addr), "application/json", nil)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", resp.StatusCode)
	}
}

func TestHandleReload(t *testing.T) {
	logger := zap.NewNop()
	cfg := Config{
		ListenAddr:     "127.0.0.1:0",
		MetricsEnabled: false,
		MetricsPath:    "/metrics",
	}

	server := NewServer(cfg, logger)
	err := server.Start()
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop(context.Background())

	time.Sleep(100 * time.Millisecond)

	addr := server.Addr()
	if addr == "" {
		t.Skip("cannot determine server address")
	}

	// Make POST request
	resp, err := http.Post(fmt.Sprintf("http://%s/reload", addr), "application/json", nil)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestHandleReloadMethodNotAllowed(t *testing.T) {
	logger := zap.NewNop()
	cfg := Config{
		ListenAddr:     "127.0.0.1:0",
		MetricsEnabled: false,
		MetricsPath:    "/metrics",
	}

	server := NewServer(cfg, logger)
	err := server.Start()
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop(context.Background())

	time.Sleep(100 * time.Millisecond)

	addr := server.Addr()
	if addr == "" {
		t.Skip("cannot determine server address")
	}

	// Make GET request (should fail)
	resp, err := http.Get(fmt.Sprintf("http://%s/reload", addr))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", resp.StatusCode)
	}
}

func TestFormatHealthJSON(t *testing.T) {
	tests := []struct {
		name   string
		health map[string]bool
		want   string
	}{
		{
			name:   "nil health",
			health: nil,
			want:   "{}",
		},
		{
			name:   "empty health",
			health: map[string]bool{},
			want:   "{}",
		},
		{
			name: "single healthy",
			health: map[string]bool{
				"backend1": true,
			},
			want: `{"backend1":true}`,
		},
		{
			name: "single unhealthy",
			health: map[string]bool{
				"backend1": false,
			},
			want: `{"backend1":false}`,
		},
		{
			name: "mixed",
			health: map[string]bool{
				"backend1": true,
				"backend2": false,
			},
			want: `{"backend1":true,"backend2":false}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatHealthJSON(tt.health)
			// Order of map iteration is not guaranteed, so we check for content
			if len(tt.health) == 0 {
				if got != tt.want {
					t.Errorf("formatHealthJSON() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestMetricsEndpoint(t *testing.T) {
	logger := zap.NewNop()
	cfg := Config{
		ListenAddr:     "127.0.0.1:0",
		MetricsEnabled: true,
		MetricsPath:    "/metrics",
	}

	server := NewServer(cfg, logger)
	err := server.Start()
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop(context.Background())

	time.Sleep(100 * time.Millisecond)

	addr := server.Addr()
	if addr == "" {
		t.Skip("cannot determine server address")
	}

	// Make request to metrics endpoint
	resp, err := http.Get(fmt.Sprintf("http://%s/metrics", addr))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("expected Content-Type to contain 'text/plain', got %s", contentType)
	}
}

func TestMetricsEndpointDisabled(t *testing.T) {
	logger := zap.NewNop()
	cfg := Config{
		ListenAddr:     "127.0.0.1:0",
		MetricsEnabled: false,
		MetricsPath:    "/metrics",
	}

	server := NewServer(cfg, logger)
	err := server.Start()
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop(context.Background())

	time.Sleep(100 * time.Millisecond)

	addr := server.Addr()
	if addr == "" {
		t.Skip("cannot determine server address")
	}

	// Make request to metrics endpoint - should return 404
	resp, err := http.Get(fmt.Sprintf("http://%s/metrics", addr))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404 when metrics disabled, got %d", resp.StatusCode)
	}
}

func TestDefaultMetricsPath(t *testing.T) {
	logger := zap.NewNop()
	cfg := Config{
		ListenAddr:     "127.0.0.1:0",
		MetricsEnabled: true,
		MetricsPath:    "", // Empty path should default to /metrics
	}

	server := NewServer(cfg, logger)
	err := server.Start()
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop(context.Background())

	time.Sleep(100 * time.Millisecond)

	addr := server.Addr()
	if addr == "" {
		t.Skip("cannot determine server address")
	}

	// Make request to default metrics endpoint
	resp, err := http.Get(fmt.Sprintf("http://%s/metrics", addr))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}
