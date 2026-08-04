package trafficlog

import (
	"fmt"
	"testing"
	"time"

	"github.com/fastVIP/fastVIP/pkg/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

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
