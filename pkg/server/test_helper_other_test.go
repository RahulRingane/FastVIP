//go:build !integration

package server

import (
	"testing"

	"github.com/RahulRingane/FastVIP/pkg/lvs"
	"go.uber.org/zap"
)

// newTestServer creates a Server backed by the fake in-memory IPVS handle.
func newTestServer(t *testing.T, configPath string) *Server {
	t.Helper()
	logger := zap.NewNop()

	lvsMgr, err := lvs.NewManager(logger)
	if err != nil {
		t.Fatalf("lvs.NewManager failed: %v", err)
	}

	srv, err := newServerWithManager(configPath, lvsMgr, logger, zap.NewNop())
	if err != nil {
		t.Fatalf("newServerWithManager failed: %v", err)
	}
	return srv
}

// newTestLVSManager creates an LVS Manager backed by the fake in-memory IPVS handle.
func newTestLVSManager(t *testing.T) *lvs.Manager {
	t.Helper()
	mgr, err := lvs.NewManager(zap.NewNop())
	if err != nil {
		t.Fatalf("lvs.NewManager failed: %v", err)
	}
	return mgr
}
