//go:build integration

package server

import (
	"testing"

	"github.com/fastVIP/fastVIP/pkg/lvs"
	"go.uber.org/zap"
)

// newTestServer creates a Server backed by the real Linux IPVS handle.
// Tests must run serially (go test -p 1) because IPVS is a global kernel resource.
// TestMain handles the initial Flush; each test flushes before and after via Cleanup.
func newTestServer(t *testing.T, configPath string) *Server {
	t.Helper()
	logger := zap.NewNop()

	lvsMgr, err := lvs.NewManager(logger)
	if err != nil {
		t.Fatalf("lvs.NewManager failed: %v", err)
	}

	// Flush existing IPVS rules to ensure a clean starting state
	if err := lvsMgr.Flush(); err != nil {
		t.Fatalf("failed to flush IPVS rules before test: %v", err)
	}
	// Register cleanup to flush after test completes.
	// Use a separate handle because the test may close the manager
	// via defer or shutdown() before t.Cleanup runs.
	t.Cleanup(func() {
		cleanupHandle, err := lvs.NewIPVSHandle("")
		if err == nil {
			cleanupHandle.Flush()
			cleanupHandle.Close()
		}
	})

	srv, err := newServerWithManager(configPath, lvsMgr, logger, zap.NewNop())
	if err != nil {
		t.Fatalf("newServerWithManager failed: %v", err)
	}
	return srv
}

// newTestLVSManager creates an LVS Manager backed by the real Linux IPVS handle.
// Tests must run serially (go test -p 1) because IPVS is a global kernel resource.
func newTestLVSManager(t *testing.T) *lvs.Manager {
	t.Helper()
	mgr, err := lvs.NewManager(zap.NewNop())
	if err != nil {
		t.Fatalf("lvs.NewManager failed: %v", err)
	}
	// Flush existing IPVS rules to ensure a clean starting state
	if err := mgr.Flush(); err != nil {
		t.Fatalf("failed to flush IPVS rules before test: %v", err)
	}
	// Register cleanup to flush after test completes.
	t.Cleanup(func() {
		cleanupHandle, err := lvs.NewIPVSHandle("")
		if err == nil {
			cleanupHandle.Flush()
			cleanupHandle.Close()
		}
	})
	return mgr
}
