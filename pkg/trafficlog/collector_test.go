package trafficlog

import (
	"fmt"
	"testing"
	"time"

	"github.com/RahulRingane/FastVIP/pkg/config"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// gatherMetricValue scans the default Prometheus registry (which is what
// pkg/metrics's promauto-registered collectors publish to) for a single
// metric family/label-set combination and returns its counter/gauge value.
// pkg/metrics keeps its underlying CounterVec/GaugeVec variables unexported,
// so this is the only way for an external package's test to check the exact
// value recorded for a specific label set.
func gatherMetricValue(t *testing.T, name string, labels map[string]string) (float64, bool) {
	t.Helper()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	for _, mf := range families {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			got := make(map[string]string, len(m.GetLabel()))
			for _, lp := range m.GetLabel() {
				got[lp.GetName()] = lp.GetValue()
			}

			matched := true
			for k, v := range labels {
				if got[k] != v {
					matched = false
					break
				}
			}
			if !matched {
				continue
			}

			if m.Counter != nil {
				return m.Counter.GetValue(), true
			}
			if m.Gauge != nil {
				return m.Gauge.GetValue(), true
			}
		}
	}
	return 0, false
}

// fakeLVSStatsProvider is a mock implementation of LVSStatsProvider for testing.
type fakeLVSStatsProvider struct {
	serviceStats map[string]ServiceTrafficStats
	backendStats map[string]BackendTrafficStats
	serviceErr   error
	backendErr   error
}

func (f *fakeLVSStatsProvider) ServiceStats() (map[string]ServiceTrafficStats, error) {
	if f.serviceErr != nil {
		return nil, f.serviceErr
	}
	return f.serviceStats, nil
}

func (f *fakeLVSStatsProvider) BackendStats() (map[string]BackendTrafficStats, error) {
	if f.backendErr != nil {
		return nil, f.backendErr
	}
	return f.backendStats, nil
}

func boolPtr(b bool) *bool {
	return &b
}

func newTestTrafficConfig(enabled bool, interval string) config.TrafficLogConfig {
	return config.TrafficLogConfig{
		Enabled:  boolPtr(enabled),
		Interval: interval,
	}
}

func newTestServiceConfig(name, listen, protocol, scheduler string, trafficLog *bool) config.ServiceConfig {
	return config.ServiceConfig{
		Name:       name,
		Listen:     listen,
		Protocol:   protocol,
		Scheduler:  scheduler,
		TrafficLog: trafficLog,
	}
}

func TestCollector_DefaultDisabled_NoLogs(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	trafficLogger := zap.New(core)

	lvsProvider := &fakeLVSStatsProvider{
		serviceStats: map[string]ServiceTrafficStats{
			"10.0.0.1:80/tcp": {Connections: 100, InBytes: 50000, OutBytes: 30000},
		},
	}

	// nil TrafficLog means disabled by default
	services := []config.ServiceConfig{
		newTestServiceConfig("web", "10.0.0.1:80", "tcp", "rr", nil),
	}
	trafficCfg := newTestTrafficConfig(true, "15s")

	c := NewCollector(lvsProvider, trafficLogger, zap.NewNop(), services, trafficCfg)
	c.collect()

	if logs.Len() != 0 {
		t.Errorf("expected 0 log entries when traffic_log is nil (disabled), got %d", logs.Len())
		for _, entry := range logs.All() {
			t.Logf("  unexpected log: %s %v", entry.Message, entry.ContextMap())
		}
	}
}

func TestCollector_RawStatsLogging(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	trafficLogger := zap.New(core)

	lvsProvider := &fakeLVSStatsProvider{
		serviceStats: map[string]ServiceTrafficStats{
			"10.0.0.1:80/tcp": {Connections: 100, InPkts: 200, OutPkts: 150, InBytes: 50000, OutBytes: 30000},
		},
		backendStats: map[string]BackendTrafficStats{
			"10.0.0.1:80/tcp->192.168.1.1:8080": {
				ServiceKey:          "10.0.0.1:80/tcp",
				Connections:         60,
				ActiveConnections:   5,
				InactiveConnections: 2,
				CurrentConnections:  7,
				InPkts:              120,
				OutPkts:             90,
				InBytes:             28000,
				OutBytes:            17000,
			},
			"10.0.0.1:80/tcp->192.168.1.2:8080": {
				ServiceKey:          "10.0.0.1:80/tcp",
				Connections:         40,
				ActiveConnections:   3,
				InactiveConnections: 1,
				CurrentConnections:  4,
				InPkts:              80,
				OutPkts:             60,
				InBytes:             22000,
				OutBytes:            13000,
			},
		},
	}

	// Enable traffic logging
	services := []config.ServiceConfig{
		newTestServiceConfig("web", "10.0.0.1:80", "tcp", "rr", boolPtr(true)),
	}
	trafficCfg := newTestTrafficConfig(true, "15s")

	c := NewCollector(lvsProvider, trafficLogger, zap.NewNop(), services, trafficCfg)
	c.collect()

	if logs.Len() != 3 {
		t.Fatalf("expected 3 log entries (1 service + 2 backends), got %d", logs.Len())
	}

	var serviceLogFields map[string]interface{}
	backendLogCount := 0
	for _, entry := range logs.All() {
		fields := entry.ContextMap()
		if fields["service"] != "web" {
			t.Errorf("expected service='web', got %v", fields["service"])
		}

		switch fields["type"] {
		case "service":
			serviceLogFields = fields
		case "backend":
			backendLogCount++
			if _, ok := fields["current_connections"]; !ok {
				t.Errorf("expected backend log to include current_connections, got %v", fields)
			}
			if _, ok := fields["active_connections"]; !ok {
				t.Errorf("expected backend log to include active_connections, got %v", fields)
			}
			if _, ok := fields["inactive_connections"]; !ok {
				t.Errorf("expected backend log to include inactive_connections, got %v", fields)
			}
		}
	}

	if backendLogCount != 2 {
		t.Fatalf("expected 2 backend log entries, got %d", backendLogCount)
	}
	if serviceLogFields == nil {
		t.Fatal("expected service log entry")
	}
	if serviceLogFields["current_connections"] != uint64(11) {
		t.Errorf("expected service current_connections=11, got %v", serviceLogFields["current_connections"])
	}
	if serviceLogFields["active_connections"] != uint64(8) {
		t.Errorf("expected service active_connections=8, got %v", serviceLogFields["active_connections"])
	}
	if serviceLogFields["inactive_connections"] != uint64(3) {
		t.Errorf("expected service inactive_connections=3, got %v", serviceLogFields["inactive_connections"])
	}
}

func TestCollector_TrafficLogFalse(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	trafficLogger := zap.New(core)

	lvsProvider := &fakeLVSStatsProvider{
		serviceStats: map[string]ServiceTrafficStats{
			"10.0.0.1:80/tcp":  {Connections: 100, InBytes: 50000, OutBytes: 30000},
			"10.0.0.2:443/tcp": {Connections: 200, InBytes: 100000, OutBytes: 60000},
			"10.0.0.3:53/udp":  {Connections: 50, InBytes: 10000, OutBytes: 5000},
		},
	}

	services := []config.ServiceConfig{
		newTestServiceConfig("web", "10.0.0.1:80", "tcp", "rr", boolPtr(true)),   // enabled
		newTestServiceConfig("api", "10.0.0.2:443", "tcp", "rr", boolPtr(false)), // disabled
		newTestServiceConfig("dns", "10.0.0.3:53", "udp", "rr", nil),             // disabled (nil = default)
	}
	trafficCfg := newTestTrafficConfig(true, "15s")

	c := NewCollector(lvsProvider, trafficLogger, zap.NewNop(), services, trafficCfg)
	c.collect()

	// Only "web" (true) should produce logs; "api" (false) and "dns" (nil) should not
	if logs.Len() != 1 {
		t.Fatalf("expected 1 log entry (only web with traffic_log=true), got %d", logs.Len())
	}

	entry := logs.All()[0]
	fields := entry.ContextMap()
	if fields["service"] != "web" {
		t.Errorf("expected service='web', got %v", fields["service"])
	}
}

func TestCollector_TrafficLogExplicitFalse(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	trafficLogger := zap.New(core)

	lvsProvider := &fakeLVSStatsProvider{
		serviceStats: map[string]ServiceTrafficStats{
			"10.0.0.1:80/tcp": {Connections: 100, InBytes: 50000, OutBytes: 30000},
		},
	}

	services := []config.ServiceConfig{
		newTestServiceConfig("web", "10.0.0.1:80", "tcp", "rr", boolPtr(false)),
	}
	trafficCfg := newTestTrafficConfig(true, "15s")

	c := NewCollector(lvsProvider, trafficLogger, zap.NewNop(), services, trafficCfg)
	c.collect()

	if logs.Len() != 0 {
		t.Errorf("expected 0 log entries for traffic_log=false, got %d", logs.Len())
	}
}




func TestCollector_UpdateConfig(t *testing.T) {
	lvsProvider := &fakeLVSStatsProvider{
		serviceStats: map[string]ServiceTrafficStats{
			"10.0.0.1:80/tcp": {Connections: 100, InBytes: 50000, OutBytes: 30000},
		},
	}

	services := []config.ServiceConfig{
		newTestServiceConfig("web", "10.0.0.1:80", "tcp", "rr", nil),
	}
	trafficCfg := newTestTrafficConfig(true, "15s")

	c := NewCollector(lvsProvider, zap.NewNop(), zap.NewNop(), services, trafficCfg)

	// Update config
	newServices := []config.ServiceConfig{
		newTestServiceConfig("web", "10.0.0.1:80", "tcp", "rr", boolPtr(true)),
		newTestServiceConfig("api", "10.0.0.2:443", "tcp", "wrr", nil),
	}
	newTrafficCfg := newTestTrafficConfig(true, "30s")

	c.UpdateConfig(newServices, newTrafficCfg)

	// Verify config was updated
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.services) != 2 {
		t.Errorf("expected 2 services after update, got %d", len(c.services))
	}
	if c.trafficCfg.GetInterval() != 30*time.Second {
		t.Errorf("expected interval 30s after update, got %v", c.trafficCfg.GetInterval())
	}
}

func TestCollector_StartStop(t *testing.T) {
	lvsProvider := &fakeLVSStatsProvider{
		serviceStats: make(map[string]ServiceTrafficStats),
	}

	services := []config.ServiceConfig{}
	trafficCfg := newTestTrafficConfig(true, "5s")

	c := NewCollector(lvsProvider, zap.NewNop(), zap.NewNop(), services, trafficCfg)

	// Start and stop should not panic
	c.Start()

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	c.Stop()

	// Verify stopped channel is closed
	select {
	case <-c.stopped:
		// OK
	default:
		t.Error("stopped channel should be closed after Stop()")
	}
}

func TestBuildServiceConfigMap(t *testing.T) {
	services := []config.ServiceConfig{
		newTestServiceConfig("web", "10.0.0.1:80", "tcp", "rr", boolPtr(true)),
		newTestServiceConfig("api", "10.0.0.2:443", "tcp", "wrr", boolPtr(false)),
		newTestServiceConfig("dns", "10.0.0.3:53", "udp", "rr", nil),
	}

	result := buildServiceConfigMap(services)

	if len(result) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result))
	}

	// Verify key format matches IPVS format: "listen/protocol"
	if svc, ok := result["10.0.0.1:80/tcp"]; !ok {
		t.Error("expected key '10.0.0.1:80/tcp'")
	} else if svc.Name != "web" {
		t.Errorf("expected name 'web', got %q", svc.Name)
	}

	if svc, ok := result["10.0.0.2:443/tcp"]; !ok {
		t.Error("expected key '10.0.0.2:443/tcp'")
	} else if svc.TrafficLog == nil || *svc.TrafficLog != false {
		t.Errorf("expected TrafficLog=false, got %v", svc.TrafficLog)
	}

	if _, ok := result["10.0.0.3:53/udp"]; !ok {
		t.Error("expected key '10.0.0.3:53/udp'")
	}
}

func TestIsTrafficLogEnabled(t *testing.T) {
	tests := []struct {
		value    *bool
		name     string
		expected bool
	}{
		{nil, "nil means disabled", false},
		{boolPtr(false), "false means disabled", false},
		{boolPtr(true), "true means enabled", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTrafficLogEnabled(tt.value)
			if result != tt.expected {
				t.Errorf("isTrafficLogEnabled(%v) = %v, want %v", tt.value, result, tt.expected)
			}
		})
	}
}

func TestCollector_StatsProviderError(t *testing.T) {
	sysCore, sysLogs := observer.New(zapcore.DebugLevel)
	systemLogger := zap.New(sysCore)

	trafficCore, trafficLogs := observer.New(zapcore.DebugLevel)
	trafficLogger := zap.New(trafficCore)

	lvsProvider := &fakeLVSStatsProvider{
		serviceErr: fmt.Errorf("ipvs connection failed"),
	}

	services := []config.ServiceConfig{}
	trafficCfg := newTestTrafficConfig(true, "15s")

	c := NewCollector(lvsProvider, trafficLogger, systemLogger, services, trafficCfg)

	// Should not panic, should log warning
	c.collect()

	// System logger should have warnings about failed stats collection
	if sysLogs.Len() == 0 {
		t.Error("expected system logger warnings about failed stats collection")
	}

	// Traffic logger should have no entries (no valid data)
	if trafficLogs.Len() != 0 {
		t.Errorf("expected 0 traffic log entries, got %d", trafficLogs.Len())
	}
}

// TestCollector_UpdateMetrics_DeltaAcrossPolls verifies that updateMetrics
// adds only the per-poll delta of the raw cumulative IPVS counters, not the
// whole cumulative total every tick. Two consecutive polls with increasing
// cumulative values should leave the Prometheus counter equal to the final
// cumulative value, not the sum of two full cumulative totals.
func TestCollector_UpdateMetrics_DeltaAcrossPolls(t *testing.T) {
	services := []config.ServiceConfig{
		newTestServiceConfig("delta-svc", "10.99.0.1:80", "tcp", "rr", nil),
	}
	c := NewCollector(&fakeLVSStatsProvider{}, zap.NewNop(), zap.NewNop(), services, newTestTrafficConfig(true, "15s"))

	svcLabels := map[string]string{"service": "delta-svc", "listen": "10.99.0.1:80", "protocol": "tcp"}
	backendLabels := map[string]string{"service": "delta-svc", "backend": "10.99.0.2:8080", "protocol": "tcp"}

	// First poll: raw cumulative counters as IPVS would report them fresh.
	c.updateMetrics(&TrafficSnapshot{
		Services: map[string]ServiceTrafficStats{
			"10.99.0.1:80/tcp": {Connections: 100, InBytes: 5000, OutBytes: 3000, InPkts: 50, OutPkts: 40},
		},
		Backends: map[string]BackendTrafficStats{
			"10.99.0.1:80/tcp->10.99.0.2:8080": {
				ServiceKey: "10.99.0.1:80/tcp", Connections: 60, InBytes: 2800, OutBytes: 1700,
			},
		},
	})

	if v, ok := gatherMetricValue(t, "fastVIP_service_connections_total", svcLabels); !ok || v != 100 {
		t.Fatalf("after first poll: expected service connections=100, got %v (found=%v)", v, ok)
	}
	if v, ok := gatherMetricValue(t, "fastVIP_backend_connections_total", backendLabels); !ok || v != 60 {
		t.Fatalf("after first poll: expected backend connections=60, got %v (found=%v)", v, ok)
	}

	// Second poll: cumulative counters advanced by 30/60 connections respectively.
	c.updateMetrics(&TrafficSnapshot{
		Services: map[string]ServiceTrafficStats{
			"10.99.0.1:80/tcp": {Connections: 130, InBytes: 5500, OutBytes: 3300, InPkts: 55, OutPkts: 44},
		},
		Backends: map[string]BackendTrafficStats{
			"10.99.0.1:80/tcp->10.99.0.2:8080": {
				ServiceKey: "10.99.0.1:80/tcp", Connections: 90, InBytes: 3100, OutBytes: 1900,
			},
		},
	})

	// The Prometheus counter must equal the latest raw cumulative value, i.e.
	// only the delta (30 and 30) was added on the second poll, not another 130/90.
	if v, ok := gatherMetricValue(t, "fastVIP_service_connections_total", svcLabels); !ok || v != 130 {
		t.Fatalf("after second poll: expected service connections=130 (delta-only), got %v (found=%v)", v, ok)
	}
	if v, ok := gatherMetricValue(t, "fastVIP_backend_connections_total", backendLabels); !ok || v != 90 {
		t.Fatalf("after second poll: expected backend connections=90 (delta-only), got %v (found=%v)", v, ok)
	}
	if v, ok := gatherMetricValue(t, "fastVIP_service_bytes_in_total", svcLabels); !ok || v != 5500 {
		t.Fatalf("after second poll: expected service bytes_in=5500 (delta-only), got %v (found=%v)", v, ok)
	}
}

// TestCollector_UpdateMetrics_CounterReset verifies the IPVS counter-reset
// case: when a service/destination is recreated by a reconcile, its raw
// cumulative counters restart from zero. The next poll must treat the lower
// value as the delta directly, rather than computing a negative delta or
// wrapping around as an unsigned integer.
func TestCollector_UpdateMetrics_CounterReset(t *testing.T) {
	services := []config.ServiceConfig{
		newTestServiceConfig("reset-svc", "10.99.0.3:80", "tcp", "rr", nil),
	}
	c := NewCollector(&fakeLVSStatsProvider{}, zap.NewNop(), zap.NewNop(), services, newTestTrafficConfig(true, "15s"))

	svcLabels := map[string]string{"service": "reset-svc", "listen": "10.99.0.3:80", "protocol": "tcp"}
	backendLabels := map[string]string{"service": "reset-svc", "backend": "10.99.0.4:8080", "protocol": "tcp"}

	// First poll establishes a baseline cumulative value.
	c.updateMetrics(&TrafficSnapshot{
		Services: map[string]ServiceTrafficStats{
			"10.99.0.3:80/tcp": {Connections: 50, InBytes: 4000, OutBytes: 2000},
		},
		Backends: map[string]BackendTrafficStats{
			"10.99.0.3:80/tcp->10.99.0.4:8080": {
				ServiceKey: "10.99.0.3:80/tcp", Connections: 50, InBytes: 4000, OutBytes: 2000,
			},
		},
	})

	if v, ok := gatherMetricValue(t, "fastVIP_service_connections_total", svcLabels); !ok || v != 50 {
		t.Fatalf("after first poll: expected service connections=50, got %v (found=%v)", v, ok)
	}

	// Second poll: IPVS recreated the service/destination, so the raw
	// cumulative counters reset to a value lower than what was last seen.
	c.updateMetrics(&TrafficSnapshot{
		Services: map[string]ServiceTrafficStats{
			"10.99.0.3:80/tcp": {Connections: 5, InBytes: 300, OutBytes: 150},
		},
		Backends: map[string]BackendTrafficStats{
			"10.99.0.3:80/tcp->10.99.0.4:8080": {
				ServiceKey: "10.99.0.3:80/tcp", Connections: 5, InBytes: 300, OutBytes: 150,
			},
		},
	})

	// The post-reset value (5) must be added as-is (50 + 5 = 55), never
	// treated as a negative delta or wrapped into a huge uint64.
	if v, ok := gatherMetricValue(t, "fastVIP_service_connections_total", svcLabels); !ok || v != 55 {
		t.Fatalf("after counter reset: expected service connections=55, got %v (found=%v)", v, ok)
	}
	if v, ok := gatherMetricValue(t, "fastVIP_backend_connections_total", backendLabels); !ok || v != 55 {
		t.Fatalf("after counter reset: expected backend connections=55, got %v (found=%v)", v, ok)
	}
}

// TestCollector_UpdateMetrics_VanishedKeyRevival verifies the IPVS
// trash-list revival case: a backend/service key that disappears from the
// snapshot for a few polls (e.g. a backend fails its health check and the
// reconciler removes its destination) and then reappears with a HIGHER
// cumulative value than before it vanished must add only the difference,
// not the whole retained value. This models the measured production bug:
// value 100 before removal, backend recovers with the destination revived
// by the kernel at 120, so the reappearance poll must add exactly 20.
func TestCollector_UpdateMetrics_VanishedKeyRevival(t *testing.T) {
	services := []config.ServiceConfig{
		newTestServiceConfig("revive-svc", "10.99.0.5:80", "tcp", "rr", nil),
	}
	c := NewCollector(&fakeLVSStatsProvider{}, zap.NewNop(), zap.NewNop(), services, newTestTrafficConfig(true, "15s"))

	svcLabels := map[string]string{"service": "revive-svc", "listen": "10.99.0.5:80", "protocol": "tcp"}
	backendLabels := map[string]string{"service": "revive-svc", "backend": "10.99.0.6:8080", "protocol": "tcp"}

	// Poll 1: baseline cumulative value of 100.
	c.updateMetrics(&TrafficSnapshot{
		Services: map[string]ServiceTrafficStats{
			"10.99.0.5:80/tcp": {Connections: 100},
		},
		Backends: map[string]BackendTrafficStats{
			"10.99.0.5:80/tcp->10.99.0.6:8080": {ServiceKey: "10.99.0.5:80/tcp", Connections: 100},
		},
	})
	if v, ok := gatherMetricValue(t, "fastVIP_service_connections_total", svcLabels); !ok || v != 100 {
		t.Fatalf("after poll 1: expected service connections=100, got %v (found=%v)", v, ok)
	}

	// Polls 2-4: the backend is unhealthy, its IPVS destination (and the
	// service, for this test) is removed by the reconciler, so the key is
	// absent from the snapshot entirely - not present with a lower/zero
	// value, genuinely missing.
	for i := 0; i < 3; i++ {
		c.updateMetrics(&TrafficSnapshot{
			Services: map[string]ServiceTrafficStats{},
			Backends: map[string]BackendTrafficStats{},
		})
	}

	// Poll 5: the backend recovers. IPVS revives the destination from its
	// trash list with cumulative stats intact at 120 (not reset to 0).
	c.updateMetrics(&TrafficSnapshot{
		Services: map[string]ServiceTrafficStats{
			"10.99.0.5:80/tcp": {Connections: 120},
		},
		Backends: map[string]BackendTrafficStats{
			"10.99.0.5:80/tcp->10.99.0.6:8080": {ServiceKey: "10.99.0.5:80/tcp", Connections: 120},
		},
	})

	// Only the 20-connection difference since the retained value of 100
	// must be added, not the full 120 on top of the already-recorded 100
	// (which would wrongly total 220).
	if v, ok := gatherMetricValue(t, "fastVIP_service_connections_total", svcLabels); !ok || v != 120 {
		t.Fatalf("after revival: expected service connections=120 (100+20 delta), got %v (found=%v)", v, ok)
	}
	if v, ok := gatherMetricValue(t, "fastVIP_backend_connections_total", backendLabels); !ok || v != 120 {
		t.Fatalf("after revival: expected backend connections=120 (100+20 delta), got %v (found=%v)", v, ok)
	}
}

// TestCollector_UpdateMetrics_VanishedKeyGenuineReset verifies that a key
// which disappears and later reappears with a LOWER cumulative value than
// before it vanished (a genuine destroy-and-recreate-from-zero, as opposed
// to an IPVS trash-list revival) still goes through the existing
// deltaUint64 counter-reset guard and adds the new value directly.
func TestCollector_UpdateMetrics_VanishedKeyGenuineReset(t *testing.T) {
	services := []config.ServiceConfig{
		newTestServiceConfig("reset-svc2", "10.99.0.7:80", "tcp", "rr", nil),
	}
	c := NewCollector(&fakeLVSStatsProvider{}, zap.NewNop(), zap.NewNop(), services, newTestTrafficConfig(true, "15s"))

	svcLabels := map[string]string{"service": "reset-svc2", "listen": "10.99.0.7:80", "protocol": "tcp"}
	backendLabels := map[string]string{"service": "reset-svc2", "backend": "10.99.0.8:8080", "protocol": "tcp"}

	// Poll 1: baseline cumulative value of 100.
	c.updateMetrics(&TrafficSnapshot{
		Services: map[string]ServiceTrafficStats{
			"10.99.0.7:80/tcp": {Connections: 100},
		},
		Backends: map[string]BackendTrafficStats{
			"10.99.0.7:80/tcp->10.99.0.8:8080": {ServiceKey: "10.99.0.7:80/tcp", Connections: 100},
		},
	})
	if v, ok := gatherMetricValue(t, "fastVIP_service_connections_total", svcLabels); !ok || v != 100 {
		t.Fatalf("after poll 1: expected service connections=100, got %v (found=%v)", v, ok)
	}

	// Poll 2: key vanishes entirely.
	c.updateMetrics(&TrafficSnapshot{
		Services: map[string]ServiceTrafficStats{},
		Backends: map[string]BackendTrafficStats{},
	})

	// Poll 3: key reappears freshly created, cumulative value lower than
	// what was retained (25 < 100) - a genuine reset, not a revival.
	c.updateMetrics(&TrafficSnapshot{
		Services: map[string]ServiceTrafficStats{
			"10.99.0.7:80/tcp": {Connections: 25},
		},
		Backends: map[string]BackendTrafficStats{
			"10.99.0.7:80/tcp->10.99.0.8:8080": {ServiceKey: "10.99.0.7:80/tcp", Connections: 25},
		},
	})

	// The new value must be added as-is (100 + 25 = 125) via the
	// counter-reset guard, never a negative/underflowed delta.
	if v, ok := gatherMetricValue(t, "fastVIP_service_connections_total", svcLabels); !ok || v != 125 {
		t.Fatalf("after genuine reset: expected service connections=125, got %v (found=%v)", v, ok)
	}
	if v, ok := gatherMetricValue(t, "fastVIP_backend_connections_total", backendLabels); !ok || v != 125 {
		t.Fatalf("after genuine reset: expected backend connections=125, got %v (found=%v)", v, ok)
	}
}

// TestCollector_UpdateMetrics_VanishedKeyEvictedAfterThreshold verifies the
// memory bound: a key absent for more than maxAbsentPolls consecutive polls
// is evicted from the carried-forward prev maps, so a later reappearance is
// treated as a brand new key (its full value is added again), rather than
// retaining stale data forever for keys that are genuinely gone for good.
func TestCollector_UpdateMetrics_VanishedKeyEvictedAfterThreshold(t *testing.T) {
	services := []config.ServiceConfig{
		newTestServiceConfig("evict-svc", "10.99.0.9:80", "tcp", "rr", nil),
	}
	c := NewCollector(&fakeLVSStatsProvider{}, zap.NewNop(), zap.NewNop(), services, newTestTrafficConfig(true, "15s"))

	svcLabels := map[string]string{"service": "evict-svc", "listen": "10.99.0.9:80", "protocol": "tcp"}

	// Poll 1: baseline cumulative value of 100.
	c.updateMetrics(&TrafficSnapshot{
		Services: map[string]ServiceTrafficStats{
			"10.99.0.9:80/tcp": {Connections: 100},
		},
		Backends: map[string]BackendTrafficStats{},
	})
	if v, ok := gatherMetricValue(t, "fastVIP_service_connections_total", svcLabels); !ok || v != 100 {
		t.Fatalf("after poll 1: expected service connections=100, got %v (found=%v)", v, ok)
	}

	// Key is absent for more than maxAbsentPolls consecutive polls, so it
	// must be evicted from the carried-forward prev map.
	for i := 0; i < maxAbsentPolls+1; i++ {
		c.updateMetrics(&TrafficSnapshot{
			Services: map[string]ServiceTrafficStats{},
			Backends: map[string]BackendTrafficStats{},
		})
	}

	// Reappearance after eviction is treated as a brand new key: the full
	// current value (200) is added on top of the still-standing 100 from
	// poll 1, i.e. the Prometheus counter becomes 300, not diffed against
	// the long-evicted retained value of 100 (which would wrongly yield 200).
	c.updateMetrics(&TrafficSnapshot{
		Services: map[string]ServiceTrafficStats{
			"10.99.0.9:80/tcp": {Connections: 200},
		},
		Backends: map[string]BackendTrafficStats{},
	})

	if v, ok := gatherMetricValue(t, "fastVIP_service_connections_total", svcLabels); !ok || v != 300 {
		t.Fatalf("after eviction+reappearance: expected service connections=300 (100+200 full re-add), got %v (found=%v)", v, ok)
	}
}

func TestCollector_ServiceConfigRemoved_NoLog(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	trafficLogger := zap.New(core)

	lvsProvider := &fakeLVSStatsProvider{
		serviceStats: map[string]ServiceTrafficStats{
			"10.0.0.1:80/tcp": {Connections: 100, InBytes: 50000, OutBytes: 30000},
		},
	}

	// No matching service config — simulates a removed service
	services := []config.ServiceConfig{}
	trafficCfg := newTestTrafficConfig(true, "15s")

	c := NewCollector(lvsProvider, trafficLogger, zap.NewNop(), services, trafficCfg)
	c.collect()

	// Service config not found, should skip (not log)
	if logs.Len() != 0 {
		t.Errorf("expected 0 log entries for removed service, got %d", logs.Len())
	}
}
