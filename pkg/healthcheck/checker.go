package healthcheck

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Checker defines the interface for health check probes.
// This abstraction allows extending with different probe types (e.g., UDP) in the future.
type Checker interface {
	Check(address string) error
}

// TCPChecker implements health checking via TCP connection attempts.
type TCPChecker struct {
	timeout time.Duration
}

// NewTCPChecker creates a new TCPChecker with the given timeout.
func NewTCPChecker(timeout time.Duration) *TCPChecker {
	return &TCPChecker{
		timeout: timeout,
	}
}

// Check attempts to establish a TCP connection to the given address.
// Returns nil if the connection succeeds (healthy), or an error if it fails (unhealthy).
func (c *TCPChecker) Check(address string) error {
	conn, err := net.DialTimeout("tcp", address, c.timeout)
	if err != nil {
		return fmt.Errorf("tcp health check failed for %s: %w", address, err)
	}
	conn.Close()
	return nil
}

// HTTPChecker implements health checking via HTTP GET requests.
type HTTPChecker struct {
	client         *http.Client
	path           string
	expectedStatus int
}

// NewHTTPChecker creates a new HTTPChecker with the given parameters.
func NewHTTPChecker(timeout time.Duration, path string, expectedStatus int) *HTTPChecker {
	return &HTTPChecker{
		client: &http.Client{
			Timeout: timeout,
		},
		path:           path,
		expectedStatus: expectedStatus,
	}
}

// Check sends an HTTP GET request to the given address and verifies the response status code.
// Returns nil if the status code matches the expected value, or an error otherwise.
func (c *HTTPChecker) Check(address string) error {
	url := fmt.Sprintf("http://%s%s", address, c.path)
	resp, err := c.client.Get(url)
	if err != nil {
		return fmt.Errorf("http health check failed for %s: %w", address, err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode != c.expectedStatus {
		return fmt.Errorf("http health check failed for %s: expected status %d, got %d",
			address, c.expectedStatus, resp.StatusCode)
	}
	return nil
}
