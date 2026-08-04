package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Service-level traffic metrics (Counter)
	serviceConnectionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fastVIP_service_connections_total",
			Help: "Total number of connections for a service",
		},
		[]string{"service", "listen", "protocol"},
	)

	serviceBytesInTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fastVIP_service_bytes_in_total",
			Help: "Total incoming bytes for a service",
		},
		[]string{"service", "listen", "protocol"},
	)

	serviceBytesOutTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fastVIP_service_bytes_out_total",
			Help: "Total outgoing bytes for a service",
		},
		[]string{"service", "listen", "protocol"},
	)

	servicePacketsInTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fastVIP_service_packets_in_total",
			Help: "Total incoming packets for a service",
		},
		[]string{"service", "listen", "protocol"},
	)

	servicePacketsOutTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fastVIP_service_packets_out_total",
			Help: "Total outgoing packets for a service",
		},
		[]string{"service", "listen", "protocol"},
	)

	// Backend-level traffic metrics (Counter)
	backendConnectionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fastVIP_backend_connections_total",
			Help: "Total number of connections for a backend",
		},
		[]string{"service", "backend", "protocol"},
	)

	backendBytesInTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fastVIP_backend_bytes_in_total",
			Help: "Total incoming bytes for a backend",
		},
		[]string{"service", "backend", "protocol"},
	)

	backendBytesOutTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fastVIP_backend_bytes_out_total",
			Help: "Total outgoing bytes for a backend",
		},
		[]string{"service", "backend", "protocol"},
	)

	// Backend-level connection metrics (Gauge)
	backendActiveConnections = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fastVIP_backend_active_connections",
			Help: "Number of active connections for a backend",
		},
		[]string{"service", "backend", "protocol"},
	)

	backendInactiveConnections = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fastVIP_backend_inactive_connections",
			Help: "Number of inactive connections for a backend",
		},
		[]string{"service", "backend", "protocol"},
	)

	// Health check metrics (Gauge)
	backendHealthStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fastVIP_backend_health_status",
			Help: "Health status of a backend (1=healthy, 0=unhealthy)",
		},
		[]string{"service", "backend"},
	)

	// Config reload metrics (Counter)
	configReloadTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "fastVIP_config_reload_total",
			Help: "Total number of config reloads",
		},
	)

	// Reconcile error metrics (Counter)
	reconcileErrorsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "fastVIP_reconcile_errors_total",
			Help: "Total number of reconcile errors",
		},
	)
)

// SetServiceTraffic updates service-level traffic counters.
// Note: Prometheus Counter.Add() accepts float64, we convert from uint64.
func SetServiceTraffic(service, listen, protocol string, connections, bytesIn, bytesOut, packetsIn, packetsOut uint64) {
	labels := prometheus.Labels{
		"service":  service,
		"listen":   listen,
		"protocol": protocol,
	}
	serviceConnectionsTotal.With(labels).Add(float64(connections))
	serviceBytesInTotal.With(labels).Add(float64(bytesIn))
	serviceBytesOutTotal.With(labels).Add(float64(bytesOut))
	servicePacketsInTotal.With(labels).Add(float64(packetsIn))
	servicePacketsOutTotal.With(labels).Add(float64(packetsOut))
}

// SetBackendTraffic updates backend-level traffic counters.
func SetBackendTraffic(service, backend, protocol string, connections, bytesIn, bytesOut uint64) {
	labels := prometheus.Labels{
		"service":  service,
		"backend":  backend,
		"protocol": protocol,
	}
	backendConnectionsTotal.With(labels).Add(float64(connections))
	backendBytesInTotal.With(labels).Add(float64(bytesIn))
	backendBytesOutTotal.With(labels).Add(float64(bytesOut))
}

// SetBackendConnections updates backend-level connection gauges.
func SetBackendConnections(service, backend, protocol string, active, inactive uint64) {
	labels := prometheus.Labels{
		"service":  service,
		"backend":  backend,
		"protocol": protocol,
	}
	backendActiveConnections.With(labels).Set(float64(active))
	backendInactiveConnections.With(labels).Set(float64(inactive))
}

// SetBackendHealth updates backend health status gauge.
func SetBackendHealth(service, backend string, healthy bool) {
	labels := prometheus.Labels{
		"service": service,
		"backend": backend,
	}
	value := float64(0)
	if healthy {
		value = 1
	}
	backendHealthStatus.With(labels).Set(value)
}

// IncConfigReload increments the config reload counter.
func IncConfigReload() {
	configReloadTotal.Inc()
}

// IncReconcileErrors increments the reconcile error counter.
func IncReconcileErrors() {
	reconcileErrorsTotal.Inc()
}

// DeleteBackendMetrics removes all metrics for a specific backend.
func DeleteBackendMetrics(service, backend, protocol string) {
	backendLabels := prometheus.Labels{
		"service":  service,
		"backend":  backend,
		"protocol": protocol,
	}
	backendConnectionsTotal.Delete(backendLabels)
	backendBytesInTotal.Delete(backendLabels)
	backendBytesOutTotal.Delete(backendLabels)
	backendActiveConnections.Delete(backendLabels)
	backendInactiveConnections.Delete(backendLabels)

	healthLabels := prometheus.Labels{
		"service": service,
		"backend": backend,
	}
	backendHealthStatus.Delete(healthLabels)
}

// DeleteServiceMetrics removes all metrics for a specific service.
func DeleteServiceMetrics(service, listen, protocol string) {
	labels := prometheus.Labels{
		"service":  service,
		"listen":   listen,
		"protocol": protocol,
	}
	serviceConnectionsTotal.Delete(labels)
	serviceBytesInTotal.Delete(labels)
	serviceBytesOutTotal.Delete(labels)
	servicePacketsInTotal.Delete(labels)
	servicePacketsOutTotal.Delete(labels)
}
