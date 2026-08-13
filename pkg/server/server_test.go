//go:build !integration

package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/RahulRingane/FastVIP/pkg/config"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestServerSyncTrafficCollectorStartsOnHotEnable(t *testing.T) {
	configYAML := `
global:
  log:
    level: info
    traffic:
      enabled: false
services:
  - name: web-service
    listen: 10.0.0.1:80
    protocol: tcp
    scheduler: rr
    health_check:
      enabled: false
    backends:
      - address: 192.168.1.10:8080
        weight: 1
`
	configPath := writeYAMLFile(t, t.TempDir(), configYAML)

	srv := newTestServer(t, configPath)
	t.Cleanup(func() {
		srv.shutdown()
	})

	initialCfg := srv.configMgr.GetConfig()
	srv.syncTrafficCollector(initialCfg)
	if srv.collector != nil {
		t.Fatal("expected collector to remain nil while traffic logging is disabled")
	}

	enabledCfg := cloneConfig(initialCfg)
	enabledCfg.Global.Log.Traffic.Enabled = boolPtr(true)

	srv.syncTrafficCollector(enabledCfg)
	if srv.collector == nil {
		t.Fatal("expected collector to be created when traffic logging is hot-enabled")
	}
}

func TestRunOnceLogsKernelParameterMismatches(t *testing.T) {
	configYAML := `
global:
  log:
    level: info
services:
  - name: web-service
    listen: 10.0.0.1:80
    protocol: tcp
    scheduler: rr
    health_check:
      enabled: false
    backends:
      - address: 192.168.1.10:8080
        weight: 1
`
	configPath := writeYAMLFile(t, t.TempDir(), configYAML)

	oldEnabled := kernelParamCheckEnabled
	oldReader := readKernelParamFile
	kernelParamCheckEnabled = true
	readKernelParamFile = func(path string) ([]byte, error) {
		switch path {
		case "/proc/sys/net/ipv4/ip_forward":
			return []byte("0\n"), nil
		case "/proc/sys/net/ipv4/vs/conntrack":
			return []byte("1\n"), nil
		case "/proc/sys/net/ipv4/conf/all/rp_filter":
			return []byte("1\n"), nil
		case "/proc/sys/net/ipv4/conf/default/rp_filter":
			return []byte("0\n"), nil
		default:
			return nil, errors.New("unexpected kernel parameter path")
		}
	}
	t.Cleanup(func() {
		kernelParamCheckEnabled = oldEnabled
		readKernelParamFile = oldReader
	})

	core, logs := observer.New(zapcore.ErrorLevel)
	lvsMgr := newTestLVSManager(t)
	srv, err := newServerWithManager(configPath, lvsMgr, zap.New(core), zap.NewNop())
	if err != nil {
		t.Fatalf("newServerWithManager failed: %v", err)
	}

	if err := srv.RunOnce(); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	entries := logs.FilterMessage("kernel parameter mismatch").All()
	if len(entries) != 2 {
		t.Fatalf("expected 2 kernel parameter mismatch logs, got %d", len(entries))
	}

	got := make(map[string]string, len(entries))
	for _, entry := range entries {
		fields := entry.ContextMap()
		name, _ := fields["name"].(string)
		actual, _ := fields["actual"].(string)
		got[name] = actual
	}

	if got["net.ipv4.ip_forward"] != "0" {
		t.Fatalf("expected ip_forward actual value 0, got %q", got["net.ipv4.ip_forward"])
	}
	if got["net.ipv4.conf.all.rp_filter"] != "1" {
		t.Fatalf("expected all.rp_filter actual value 1, got %q", got["net.ipv4.conf.all.rp_filter"])
	}
}

func TestLogKernelParamPreflightLogsReadFailures(t *testing.T) {
	oldEnabled := kernelParamCheckEnabled
	oldReader := readKernelParamFile
	kernelParamCheckEnabled = true
	readKernelParamFile = func(path string) ([]byte, error) {
		if path == "/proc/sys/net/ipv4/ip_forward" {
			return nil, errors.New("permission denied")
		}
		return []byte("1\n"), nil
	}
	t.Cleanup(func() {
		kernelParamCheckEnabled = oldEnabled
		readKernelParamFile = oldReader
	})

	core, logs := observer.New(zapcore.ErrorLevel)
	srv := &Server{logger: zap.New(core)}

	srv.logKernelParamPreflight()

	entries := logs.FilterMessage("failed to read kernel parameter").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 kernel parameter read failure log, got %d", len(entries))
	}

	fields := entries[0].ContextMap()
	if fields["name"] != "net.ipv4.ip_forward" {
		t.Fatalf("expected read failure for ip_forward, got %v", fields["name"])
	}
}

func TestLogKernelParamPreflightLogsInfoWhenAllMatch(t *testing.T) {
	oldEnabled := kernelParamCheckEnabled
	oldReader := readKernelParamFile
	kernelParamCheckEnabled = true
	readKernelParamFile = func(path string) ([]byte, error) {
		switch path {
		case "/proc/sys/net/ipv4/ip_forward":
			return []byte("1\n"), nil
		case "/proc/sys/net/ipv4/vs/conntrack":
			return []byte("1\n"), nil
		case "/proc/sys/net/ipv4/conf/all/rp_filter":
			return []byte("0\n"), nil
		case "/proc/sys/net/ipv4/conf/default/rp_filter":
			return []byte("0\n"), nil
		default:
			return nil, errors.New("unexpected kernel parameter path")
		}
	}
	t.Cleanup(func() {
		kernelParamCheckEnabled = oldEnabled
		readKernelParamFile = oldReader
	})

	core, logs := observer.New(zapcore.InfoLevel)
	srv := &Server{logger: zap.New(core)}

	srv.logKernelParamPreflight()

	if logs.FilterLevelExact(zapcore.ErrorLevel).Len() != 0 {
		t.Fatalf("expected no error logs, got %d", logs.FilterLevelExact(zapcore.ErrorLevel).Len())
	}
	if logs.FilterMessage("kernel parameter preflight passed").Len() != 1 {
		t.Fatalf("expected 1 kernel parameter preflight success log, got %d", logs.FilterMessage("kernel parameter preflight passed").Len())
	}
}

// gatherGaugeValue scans the default Prometheus registry (what pkg/metrics's
// promauto-registered collectors publish to) for a single metric/label-set
// combination. pkg/metrics keeps its GaugeVec variables unexported, so this
// is how an external package's test reads back the exact value recorded for
// a specific label set.
func gatherGaugeValue(t *testing.T, name string, labels map[string]string) (float64, bool) {
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
			if matched && m.Gauge != nil {
				return m.Gauge.GetValue(), true
			}
		}
	}
	return 0, false
}

// TestUpdateHealthMetrics_UnhealthyBackendEmitsExplicitZero verifies that a
// configured-but-unhealthy backend gets an explicit 0 series on
// fastVIP_backend_health_status, rather than being silently absent. Before
// the fix, updateHealthMetrics iterated healthMgr.GetAllStatuses() (only
// tracked backends); it now iterates cfg.Services so a downstream consumer
// can rely on the gauge going 1 -> 0 instead of a series vanishing.
func TestUpdateHealthMetrics_UnhealthyBackendEmitsExplicitZero(t *testing.T) {
	configYAML := `
global:
  log:
    level: info
services:
  - name: web-service
    listen: 10.0.0.1:80
    protocol: tcp
    scheduler: rr
    health_check:
      enabled: true
      interval: 10ms
      timeout: 50ms
      fail_count: 1
      rise_count: 1
    backends:
      - address: 127.0.0.1:1
        weight: 1
`
	configPath := writeYAMLFile(t, t.TempDir(), configYAML)

	srv := newTestServer(t, configPath)
	t.Cleanup(func() {
		srv.shutdown()
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	cfg := srv.configMgr.GetConfig()
	// Nothing listens on 127.0.0.1:1, so the TCP check fails immediately and
	// (with fail_count=1) the backend flips unhealthy on the first check.
	srv.healthMgr.UpdateTargets(ctx, cfg.Services)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && srv.healthMgr.IsHealthy("127.0.0.1:1") {
		time.Sleep(5 * time.Millisecond)
	}
	if srv.healthMgr.IsHealthy("127.0.0.1:1") {
		t.Fatal("expected backend 127.0.0.1:1 to become unhealthy")
	}

	srv.updateHealthMetrics()

	labels := map[string]string{"service": "web-service", "backend": "127.0.0.1:1"}
	value, found := gatherGaugeValue(t, "fastVIP_backend_health_status", labels)
	if !found {
		t.Fatal("expected an explicit fastVIP_backend_health_status series for the unhealthy backend, found none")
	}
	if value != 0 {
		t.Fatalf("expected fastVIP_backend_health_status=0 for unhealthy backend, got %v", value)
	}
}

func cloneConfig(cfg *config.Config) *config.Config {
	if cfg == nil {
		return nil
	}

	cloned := *cfg
	cloned.Services = append([]config.ServiceConfig(nil), cfg.Services...)
	for i := range cfg.Services {
		cloned.Services[i].Backends = append([]config.BackendConfig(nil), cfg.Services[i].Backends...)
	}
	return &cloned
}

func boolPtr(v bool) *bool {
	return &v
}
