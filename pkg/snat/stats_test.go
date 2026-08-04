package snat

import (
	"testing"
)

func TestSNATRuleStats_ZeroValue(t *testing.T) {
	var stats SNATRuleStats
	if stats.Packets != 0 || stats.Bytes != 0 {
		t.Error("zero-value SNATRuleStats should have all fields as 0")
	}
}

func TestSNATRuleStats_Values(t *testing.T) {
	stats := SNATRuleStats{
		Packets: 12345,
		Bytes:   6789012,
	}
	if stats.Packets != 12345 {
		t.Errorf("expected Packets=12345, got %d", stats.Packets)
	}
	if stats.Bytes != 6789012 {
		t.Errorf("expected Bytes=6789012, got %d", stats.Bytes)
	}
}

func TestStatsProvider_TypeAssertion_NonProvider(t *testing.T) {
	// Verify the type assertion pattern used by the collector:
	// a Manager that does NOT implement StatsProvider should yield ok=false.
	// We use a local stub so this test compiles on all platforms — on Linux,
	// NewManager returns linuxManager which intentionally implements StatsProvider,
	// and FakeManager is only compiled under !integration.
	type noStatsManager struct{ Manager }
	var mgr Manager = &noStatsManager{}

	_, ok := mgr.(StatsProvider)
	if ok {
		t.Error("manager without Stats() should NOT implement StatsProvider")
	}
}

func TestStatsProvider_InterfaceCompliance(t *testing.T) {
	// Verify StatsProvider interface can be satisfied by a mock
	var _ StatsProvider = &mockStatsProvider{}
}

// mockStatsProvider is a test helper that implements StatsProvider.
type mockStatsProvider struct {
	stats map[string]SNATRuleStats
	err   error
}

func (m *mockStatsProvider) Stats() (map[string]SNATRuleStats, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.stats, nil
}

func TestMockStatsProvider_ReturnsStats(t *testing.T) {
	mock := &mockStatsProvider{
		stats: map[string]SNATRuleStats{
			"192.168.1.1:8080/tcp": {Packets: 100, Bytes: 50000},
			"192.168.1.2:9090/udp": {Packets: 200, Bytes: 100000},
		},
	}

	stats, err := mock.Stats()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(stats) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(stats))
	}

	s1, ok := stats["192.168.1.1:8080/tcp"]
	if !ok {
		t.Fatal("expected key '192.168.1.1:8080/tcp'")
	}
	if s1.Packets != 100 {
		t.Errorf("expected Packets=100, got %d", s1.Packets)
	}
	if s1.Bytes != 50000 {
		t.Errorf("expected Bytes=50000, got %d", s1.Bytes)
	}
}
