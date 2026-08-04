package trafficlog

import (
	"strings"
	"sync"
	"time"

	"github.com/fastVIP/fastVIP/pkg/config"
	"github.com/fastVIP/fastVIP/pkg/logutil"
	"github.com/fastVIP/fastVIP/pkg/metrics"
	"go.uber.org/zap"
)

// Collector periodically collects IPVS statistics
// and writes raw cumulative data as debug-level traffic logs.
// Traffic logging is disabled by default per service; only services with
// traffic_log explicitly set to true will be logged.
type Collector struct {
	trafficCfg    config.TrafficLogConfig
	lvsStats      LVSStatsProvider
	trafficLogger *zap.Logger
	systemLogger  *zap.Logger
	stopCh        chan struct{}
	stopped       chan struct{}
	services      []config.ServiceConfig
	mu            sync.RWMutex
}

// NewCollector creates a new traffic statistics collector.
func NewCollector(
	lvsStats LVSStatsProvider,
	trafficLogger *zap.Logger,
	systemLogger *zap.Logger,
	services []config.ServiceConfig,
	trafficCfg config.TrafficLogConfig,
) *Collector {
	return &Collector{
		lvsStats:      lvsStats,
		trafficLogger: trafficLogger,
		systemLogger:  systemLogger,
		services:      services,
		trafficCfg:    trafficCfg,
		stopCh:        make(chan struct{}),
		stopped:       make(chan struct{}),
	}
}

// Start begins periodic collection in a background goroutine.
func (c *Collector) Start() {
	go c.run()
}

// Stop stops the collector goroutine and waits for it to finish.
func (c *Collector) Stop() {
	close(c.stopCh)
	<-c.stopped
}

// UpdateConfig dynamically updates the collector's configuration.
// Called by Server when config hot-reload is detected.
func (c *Collector) UpdateConfig(services []config.ServiceConfig, trafficCfg config.TrafficLogConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.services = services
	c.trafficCfg = trafficCfg
}

// run is the main collection loop.
func (c *Collector) run() {
	defer close(c.stopped)

	c.mu.RLock()
	interval := c.trafficCfg.GetInterval()
	c.mu.RUnlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.mu.RLock()
			newInterval := c.trafficCfg.GetInterval()
			enabled := c.trafficCfg.IsEnabled()
			c.mu.RUnlock()

			// Adjust ticker if interval changed
			if newInterval != interval {
				ticker.Reset(newInterval)
				interval = newInterval
			}

			if !enabled {
				continue
			}

			c.collect()
		}
	}
}

// collect performs a single collection cycle: gather stats and write raw data logs.
func (c *Collector) collect() {
	snapshot := c.gatherSnapshot()
	if snapshot == nil {
		return
	}

	c.logRawStats(snapshot)
	c.updateMetrics(snapshot)
}

// gatherSnapshot collects current statistics from all providers.
func (c *Collector) gatherSnapshot() *TrafficSnapshot {
	snapshot := &TrafficSnapshot{
		Services: make(map[string]ServiceTrafficStats),
		Backends: make(map[string]BackendTrafficStats),
	}

	// Collect LVS service stats
	svcStats, err := c.lvsStats.ServiceStats()
	if err != nil {
		c.systemLogger.Warn("failed to collect IPVS service stats", zap.Error(err))
	} else {
		snapshot.Services = svcStats
	}

	// Collect LVS backend stats
	backendStats, err := c.lvsStats.BackendStats()
	if err != nil {
		c.systemLogger.Warn("failed to collect IPVS backend stats", zap.Error(err))
	} else {
		snapshot.Backends = backendStats
	}

	return snapshot
}

// logRawStats writes raw cumulative statistics as debug-level log entries.
// Only services with traffic_log explicitly set to true are logged.
func (c *Collector) logRawStats(snapshot *TrafficSnapshot) {
	c.mu.RLock()
	services := c.services
	c.mu.RUnlock()

	// Build service key -> config lookup
	svcConfigMap := buildServiceConfigMap(services)
	serviceConnectionCounts := aggregateServiceConnectionCounts(snapshot.Backends)

	// Log service-level raw stats
	for key, stats := range snapshot.Services {
		svcCfg, ok := svcConfigMap[key]
		if !ok {
			// Service config not found (may have been removed), skip
			continue
		}

		// Default behavior: traffic_log is nil or false means disabled
		if !isTrafficLogEnabled(svcCfg.TrafficLog) {
			continue
		}

		fields := append(logutil.ServiceFields(svcCfg),
			zap.String("source", "ipvs"),
			zap.String("type", "service"),
			zap.Uint64("connections", stats.Connections),
			zap.Uint64("bytes_in", stats.InBytes),
			zap.Uint64("bytes_out", stats.OutBytes),
			zap.Uint64("packets_in", stats.InPkts),
			zap.Uint64("packets_out", stats.OutPkts),
		)
		if counts, ok := serviceConnectionCounts[key]; ok {
			fields = append(fields,
				zap.Uint64("active_connections", counts.ActiveConnections),
				zap.Uint64("inactive_connections", counts.InactiveConnections),
				zap.Uint64("current_connections", counts.CurrentConnections),
			)
		}
		c.trafficLogger.Debug("traffic raw stats", fields...)
	}

	// Log backend-level raw stats
	for key, stats := range snapshot.Backends {
		svcCfg, ok := svcConfigMap[stats.ServiceKey]
		if !ok {
			continue
		}

		if !isTrafficLogEnabled(svcCfg.TrafficLog) {
			continue
		}

		currentConnections := stats.CurrentConnections
		if currentConnections == 0 {
			currentConnections = stats.ActiveConnections + stats.InactiveConnections
		}

		fields := append(logutil.ServiceFields(svcCfg),
			zap.String("source", "ipvs"),
			zap.String("type", "backend"),
			zap.String("backend_key", key),
			zap.Uint64("connections", stats.Connections),
			zap.Uint64("bytes_in", stats.InBytes),
			zap.Uint64("bytes_out", stats.OutBytes),
			zap.Uint64("packets_in", stats.InPkts),
			zap.Uint64("packets_out", stats.OutPkts),
			zap.Uint64("active_connections", stats.ActiveConnections),
			zap.Uint64("inactive_connections", stats.InactiveConnections),
			zap.Uint64("current_connections", currentConnections),
		)
		c.trafficLogger.Debug("traffic raw stats", fields...)
	}

}

type serviceConnectionCounts struct {
	ActiveConnections   uint64
	InactiveConnections uint64
	CurrentConnections  uint64
}

func aggregateServiceConnectionCounts(backends map[string]BackendTrafficStats) map[string]serviceConnectionCounts {
	result := make(map[string]serviceConnectionCounts)
	for _, stats := range backends {
		counts := result[stats.ServiceKey]
		counts.ActiveConnections += stats.ActiveConnections
		counts.InactiveConnections += stats.InactiveConnections
		counts.CurrentConnections += backendCurrentConnections(stats)
		result[stats.ServiceKey] = counts
	}
	return result
}

func backendCurrentConnections(stats BackendTrafficStats) uint64 {
	if stats.CurrentConnections != 0 {
		return stats.CurrentConnections
	}
	return stats.ActiveConnections + stats.InactiveConnections
}

// buildServiceConfigMap builds a lookup map from service key (listen/protocol format)
// to ServiceConfig. The key format matches ServiceKeyFromIPVS().String().
func buildServiceConfigMap(services []config.ServiceConfig) map[string]config.ServiceConfig {
	result := make(map[string]config.ServiceConfig, len(services))
	for _, svc := range services {
		// Build key matching IPVS format: "ip:port/protocol"
		key := svc.Listen + "/" + svc.Protocol
		result[key] = svc
	}
	return result
}

// isTrafficLogEnabled returns true if the per-service traffic log flag
// is explicitly set to true. A nil pointer (default) or false means disabled.
func isTrafficLogEnabled(trafficLog *bool) bool {
	return trafficLog != nil && *trafficLog
}

// updateMetrics updates Prometheus metrics with the collected snapshot.
func (c *Collector) updateMetrics(snapshot *TrafficSnapshot) {
	c.mu.RLock()
	services := c.services
	c.mu.RUnlock()

	// Build service key -> config lookup
	svcConfigMap := buildServiceConfigMap(services)

	// Update service-level metrics
	for key, stats := range snapshot.Services {
		svcCfg, ok := svcConfigMap[key]
		if !ok {
			continue
		}

		metrics.SetServiceTraffic(
			svcCfg.Name,
			svcCfg.Listen,
			svcCfg.Protocol,
			stats.Connections,
			stats.InBytes,
			stats.OutBytes,
			stats.InPkts,
			stats.OutPkts,
		)
	}

	// Update backend-level metrics
	for backendKey, stats := range snapshot.Backends {
		svcCfg, ok := svcConfigMap[stats.ServiceKey]
		if !ok {
			continue
		}

		// Extract backend address from the full key (format: "svcKey->dstKey")
		// The dstKey format is "ip:port"
		backendAddr := extractBackendAddress(backendKey)

		metrics.SetBackendTraffic(
			svcCfg.Name,
			backendAddr,
			svcCfg.Protocol,
			stats.Connections,
			stats.InBytes,
			stats.OutBytes,
		)

		metrics.SetBackendConnections(
			svcCfg.Name,
			backendAddr,
			svcCfg.Protocol,
			stats.ActiveConnections,
			stats.InactiveConnections,
		)
	}
}

// extractBackendAddress extracts the backend address from the full key.
// The full key format is "svcKey->dstKey" where dstKey is "ip:port".
func extractBackendAddress(fullKey string) string {
	// Split by "->" to get the dstKey part
	parts := strings.Split(fullKey, "->")
	if len(parts) == 2 {
		return parts[1] // Return the dstKey (ip:port)
	}
	return fullKey // Fallback to full key if format is unexpected
}
