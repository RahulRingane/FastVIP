//go:build !integration

package lvs

import (
	"net"
	"testing"

	"go.uber.org/zap"
)

// newTestManager creates a Manager backed by the fake in-memory IPVS handle.
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	mgr, err := NewManager(zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	return mgr
}

func newTestService(address string, port uint16, protocol uint16, scheduler string) *Service {
	return &Service{
		Address:       net.ParseIP(address),
		Protocol:      protocol,
		Port:          port,
		SchedName:     scheduler,
		AddressFamily: 2, // AF_INET
		Netmask:       0xFFFFFFFF,
	}
}

func newTestDestination(address string, port uint16, weight int) *Destination {
	return &Destination{
		Address:         net.ParseIP(address),
		Port:            port,
		Weight:          weight,
		ConnectionFlags: ConnectionFlagMasq,
		AddressFamily:   2, // AF_INET
	}
}
