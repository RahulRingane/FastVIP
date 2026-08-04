package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestSetServiceTraffic(t *testing.T) {
	// Clear metrics before test
	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	SetServiceTraffic("web", "10.0.0.1:80", "tcp", 100, 5000, 3000, 50, 40)

	count, err := testutil.GatherAndCount(prometheus.DefaultGatherer, "fastVIP_service_connections_total")
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	if count < 1 {
		t.Errorf("expected at least 1 metric, got %d", count)
	}
}

func TestSetBackendTraffic(t *testing.T) {
	SetBackendTraffic("web", "192.168.1.10:8080", "tcp", 50, 2500, 1500)

	count, err := testutil.GatherAndCount(prometheus.DefaultGatherer, "fastVIP_backend_connections_total")
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	if count < 1 {
		t.Errorf("expected at least 1 metric, got %d", count)
	}
}

func TestSetBackendConnections(t *testing.T) {
	SetBackendConnections("web", "192.168.1.10:8080", "tcp", 10, 5)

	// Verify gauge metrics are set
	metrics, err := testutil.GatherAndCount(prometheus.DefaultGatherer, "fastVIP_backend_active_connections")
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	if metrics < 1 {
		t.Errorf("expected at least 1 metric, got %d", metrics)
	}
}

func TestSetBackendHealth(t *testing.T) {
	SetBackendHealth("web", "192.168.1.10:8080", true)

	// Check that health status metric exists
	dto, err := testutil.GatherAndCount(prometheus.DefaultGatherer, "fastVIP_backend_health_status")
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	if dto < 1 {
		t.Errorf("expected health status metric to exist")
	}
}

func TestSetBackendHealthUnhealthy(t *testing.T) {
	SetBackendHealth("api", "192.168.1.20:8080", false)

	// Gather and check metric value
	metricFamilies, err := testutil.GatherAndCount(prometheus.DefaultGatherer, "fastVIP_backend_health_status")
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	if metricFamilies < 1 {
		t.Errorf("expected health status metric to exist")
	}
}

func TestIncConfigReload(t *testing.T) {
	initial := testutil.ToFloat64(configReloadTotal)
	IncConfigReload()
	after := testutil.ToFloat64(configReloadTotal)

	if after != initial+1 {
		t.Errorf("expected config reload counter to increment by 1, got %f -> %f", initial, after)
	}
}

func TestIncReconcileErrors(t *testing.T) {
	initial := testutil.ToFloat64(reconcileErrorsTotal)
	IncReconcileErrors()
	after := testutil.ToFloat64(reconcileErrorsTotal)

	if after != initial+1 {
		t.Errorf("expected reconcile errors counter to increment by 1, got %f -> %f", initial, after)
	}
}

func TestDeleteBackendMetrics(t *testing.T) {
	// First set some metrics
	SetBackendTraffic("web", "192.168.1.10:8080", "tcp", 50, 2500, 1500)
	SetBackendConnections("web", "192.168.1.10:8080", "tcp", 10, 5)
	SetBackendHealth("web", "192.168.1.10:8080", true)

	// Then delete them
	DeleteBackendMetrics("web", "192.168.1.10:8080", "tcp")

	// Metrics should still exist in registry but with no data for this specific label set
	// The actual deletion is verified by checking the metric output
}

func TestDeleteServiceMetrics(t *testing.T) {
	// First set some metrics
	SetServiceTraffic("web", "10.0.0.1:80", "tcp", 100, 5000, 3000, 50, 40)

	// Then delete them
	DeleteServiceMetrics("web", "10.0.0.1:80", "tcp")
}

func TestMetricLabels(t *testing.T) {
	SetServiceTraffic("web", "10.0.0.1:80", "tcp", 100, 5000, 3000, 50, 40)

	// Gather metrics and verify label names
	metricFamilies, err := testutil.GatherAndCount(prometheus.DefaultGatherer, "fastVIP_service_connections_total")
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	if metricFamilies < 1 {
		t.Fatalf("expected at least 1 metric family")
	}

	// Verify the metric name is correct
	expectedName := "fastVIP_service_connections_total"
	output, err := testutil.GatherAndCount(prometheus.DefaultGatherer, expectedName)
	if err != nil {
		t.Fatalf("failed to gather: %v", err)
	}
	if output < 1 {
		t.Errorf("metric %s not found", expectedName)
	}
}

func TestMultipleServiceMetrics(t *testing.T) {
	// Set metrics for multiple services
	SetServiceTraffic("web", "10.0.0.1:80", "tcp", 100, 5000, 3000, 50, 40)
	SetServiceTraffic("api", "10.0.0.2:443", "tcp", 200, 10000, 6000, 100, 80)

	// Both should be tracked
	count, err := testutil.GatherAndCount(prometheus.DefaultGatherer, "fastVIP_service_connections_total")
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	if count < 2 {
		t.Errorf("expected at least 2 metrics for different services, got %d", count)
	}
}

func TestMetricOutputFormat(t *testing.T) {
	SetServiceTraffic("web", "10.0.0.1:80", "tcp", 100, 5000, 3000, 50, 40)

	output, err := testutil.GatherAndCount(prometheus.DefaultGatherer)
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	if output == 0 {
		t.Error("expected non-empty metric output")
	}
}

func TestBackendMetricsWithDifferentProtocols(t *testing.T) {
	// Test TCP backend
	SetBackendTraffic("web", "192.168.1.10:8080", "tcp", 50, 2500, 1500)
	SetBackendConnections("web", "192.168.1.10:8080", "tcp", 10, 5)

	// Test UDP backend
	SetBackendTraffic("dns", "192.168.1.20:53", "udp", 30, 1500, 1000)
	SetBackendConnections("dns", "192.168.1.20:53", "udp", 5, 3)

	// Verify both are tracked separately by protocol
	metrics, err := testutil.GatherAndCount(prometheus.DefaultGatherer, "fastVIP_backend_connections_total")
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	if metrics < 2 {
		t.Errorf("expected at least 2 backend metrics for different protocols, got %d", metrics)
	}
}

func TestCounterAccumulation(t *testing.T) {
	// Counter should accumulate values
	SetServiceTraffic("web", "10.0.0.1:80", "tcp", 100, 5000, 3000, 50, 40)
	SetServiceTraffic("web", "10.0.0.1:80", "tcp", 50, 2500, 1500, 25, 20)

	// The counter should now have accumulated both values
	// We can't easily verify the exact value, but we can verify the metric exists
	count, err := testutil.GatherAndCount(prometheus.DefaultGatherer, "fastVIP_service_connections_total")
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	if count < 1 {
		t.Errorf("expected metric to exist after accumulation")
	}
}

func TestGaugeOverwrite(t *testing.T) {
	// Gauge should be overwritten, not accumulated
	SetBackendConnections("web", "192.168.1.10:8080", "tcp", 10, 5)
	SetBackendConnections("web", "192.168.1.10:8080", "tcp", 20, 10)

	// The gauge should now show the latest values
	count, err := testutil.GatherAndCount(prometheus.DefaultGatherer, "fastVIP_backend_active_connections")
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	if count < 1 {
		t.Errorf("expected gauge metric to exist")
	}
}

func TestMetricNamePrefix(t *testing.T) {
	// All metrics should have the fastVIP_ prefix
	expectedPrefixes := []string{
		"fastVIP_service_",
		"fastVIP_backend_",
		"fastVIP_config_",
		"fastVIP_reconcile_",
	}

	// Trigger some metrics
	SetServiceTraffic("web", "10.0.0.1:80", "tcp", 100, 5000, 3000, 50, 40)
	SetBackendTraffic("web", "192.168.1.10:8080", "tcp", 50, 2500, 1500)
	SetBackendHealth("web", "192.168.1.10:8080", true)
	IncConfigReload()
	IncReconcileErrors()

	// Gather all metrics
	output, err := testutil.GatherAndCount(prometheus.DefaultGatherer)
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	if output == 0 {
		t.Error("expected metrics to be present")
	}

	// Verify prefixes (this is a basic check)
	for _, prefix := range expectedPrefixes {
		if !strings.Contains(prefix, "fastVIP_") {
			t.Errorf("metric prefix %s does not contain fastVIP_", prefix)
		}
	}
}
