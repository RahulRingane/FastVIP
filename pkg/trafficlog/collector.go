package trafficlog

import (
	"strings"
	"sync"
	"time"

	"github.com/RahulRingane/FastVIP/pkg/config"
	"github.com/RahulRingane/FastVIP/pkg/logutil"
	"github.com/RahulRingane/FastVIP/pkg/metrics"
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

	// prevServices and prevBackends hold the raw cumulative IPVS snapshot from
	// the previous poll, keyed the same way as TrafficSnapshot.Services and
	// TrafficSnapshot.Backends. Prometheus counters only support Add(), so
	// updateMetrics diffs the current snapshot against these to derive a
	// per-poll delta instead of re-adding the whole cumulative total on every
	// tick. Entries are carried forward across polls where their key is
	// missing from the snapshot (see updateMetrics for why), so these maps
	// can hold keys not present in the most recent snapshot. Guarded by mu.
	prevServices map[string]ServiceTrafficStats
	prevBackends map[string]BackendTrafficStats

	// absentServicePolls and absentBackendPolls count, per key, how many
	// consecutive polls it has been missing from the snapshot while its
	// entry in prevServices/prevBackends is still being carried forward.
	// Used to evict long-gone keys; see maxAbsentPolls. Guarded by mu.
	absentServicePolls map[string]int
	absentBackendPolls map[string]int
}

// maxAbsentPolls bounds how many consecutive polls a vanished service or
// backend key's previous cumulative value is retained for revival-diffing
// (see updateMetrics) before it is evicted from the prev maps. This keeps
// real config churn - a service or backend removed for good - from leaking
// memory forever. ~1 hour of polls at the default 5s traffic-log interval.
const maxAbsentPolls = 720

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
		prevServices:  make(map[string]ServiceTrafficStats),
		prevBackends:  make(map[string]BackendTrafficStats),

		absentServicePolls: make(map[string]int),
		absentBackendPolls: make(map[string]int),
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
// snapshot holds raw cumulative IPVS counters (the same numbers `ipvsadm -Ln
// --stats` prints), but Prometheus Counter.Add() must only ever be given the
// amount observed since the last call. This diffs the current snapshot
// against the previous poll's snapshot and adds only the delta.
func (c *Collector) updateMetrics(snapshot *TrafficSnapshot) {
	c.mu.Lock()
	services := c.services
	prevServices := c.prevServices
	prevBackends := c.prevBackends
	absentServicePolls := c.absentServicePolls
	absentBackendPolls := c.absentBackendPolls
	c.mu.Unlock()

	// Build service key -> config lookup
	svcConfigMap := buildServiceConfigMap(services)

	// A key missing from the current snapshot is NOT dropped here. IPVS does
	// not destroy a removed service/destination outright: the kernel moves it
	// onto an internal trash list and revives it - cumulative statistics
	// intact - if the same destination is added back (e.g. a backend recovers
	// its health check and the reconciler re-adds it). If the vanished key's
	// previous value were dropped, the revived entry would diff against a
	// zero-valued prev and its entire retained history would be added as a
	// single poll's delta. Measured: fastVIP_backend_connections_total jumped
	// 15219 -> 22915 (+7696) in one 5s scrape on backend recovery, versus a
	// real cumulative Conns of 7918 per `ipvsadm -Ln --stats` and ~16 req/s of
	// actual traffic. So vanished keys are carried forward into the new prev
	// maps instead, to be diffed correctly against their retained value if
	// and when they reappear. The existing deltaUint64 counter-reset guard
	// still handles the other case, where a destination really was destroyed
	// and recreated from zero (its revived/new value is lower than what was
	// last seen).
	//
	// Carried-forward entries are bounded by maxAbsentPolls so a key that is
	// genuinely gone for good (real config removal) doesn't leak memory
	// forever; the absence counter resets whenever the key is seen again.
	newPrevServices := make(map[string]ServiceTrafficStats, len(snapshot.Services))
	newPrevBackends := make(map[string]BackendTrafficStats, len(snapshot.Backends))
	newAbsentServicePolls := make(map[string]int, len(absentServicePolls))
	newAbsentBackendPolls := make(map[string]int, len(absentBackendPolls))

	// Carry forward service keys absent from this snapshot, evicting any
	// that have exceeded maxAbsentPolls consecutive absences.
	for key, stats := range prevServices {
		if _, present := snapshot.Services[key]; present {
			continue
		}
		if absentServicePolls[key]+1 > maxAbsentPolls {
			continue
		}
		newPrevServices[key] = stats
		newAbsentServicePolls[key] = absentServicePolls[key] + 1
	}

	// Update service-level metrics
	for key, stats := range snapshot.Services {
		newPrevServices[key] = stats

		svcCfg, ok := svcConfigMap[key]
		if !ok {
			continue
		}

		// prevServices[key] is the zero value if this is the first observation
		// of this service, which makes delta equal to the full current value -
		// exactly the "add it once" behavior wanted on first sight of a key.
		delta := diffServiceStats(prevServices[key], stats)

		metrics.SetServiceTraffic(
			svcCfg.Name,
			svcCfg.Listen,
			svcCfg.Protocol,
			delta.Connections,
			delta.InBytes,
			delta.OutBytes,
			delta.InPkts,
			delta.OutPkts,
		)
	}

	// Carry forward backend keys absent from this snapshot, mirroring the
	// service-key handling above.
	for key, stats := range prevBackends {
		if _, present := snapshot.Backends[key]; present {
			continue
		}
		if absentBackendPolls[key]+1 > maxAbsentPolls {
			continue
		}
		newPrevBackends[key] = stats
		newAbsentBackendPolls[key] = absentBackendPolls[key] + 1
	}

	// Update backend-level metrics
	for backendKey, stats := range snapshot.Backends {
		newPrevBackends[backendKey] = stats

		svcCfg, ok := svcConfigMap[stats.ServiceKey]
		if !ok {
			continue
		}

		// Extract backend address from the full key (format: "svcKey->dstKey")
		// The dstKey format is "ip:port"
		backendAddr := extractBackendAddress(backendKey)

		delta := diffBackendStats(prevBackends[backendKey], stats)

		metrics.SetBackendTraffic(
			svcCfg.Name,
			backendAddr,
			svcCfg.Protocol,
			delta.Connections,
			delta.InBytes,
			delta.OutBytes,
		)

		// Active/inactive connection counts are current-state gauges, not
		// cumulative counters, so they are set directly from the raw
		// snapshot - no delta involved.
		metrics.SetBackendConnections(
			svcCfg.Name,
			backendAddr,
			svcCfg.Protocol,
			stats.ActiveConnections,
			stats.InactiveConnections,
		)
	}

	c.mu.Lock()
	c.prevServices = newPrevServices
	c.prevBackends = newPrevBackends
	c.absentServicePolls = newAbsentServicePolls
	c.absentBackendPolls = newAbsentBackendPolls
	c.mu.Unlock()
}

// diffServiceStats computes the per-poll delta of cumulative service counters
// between two raw snapshots.
func diffServiceStats(prev, current ServiceTrafficStats) ServiceTrafficStats {
	return ServiceTrafficStats{
		Connections: deltaUint64(current.Connections, prev.Connections),
		InPkts:      deltaUint64(current.InPkts, prev.InPkts),
		OutPkts:     deltaUint64(current.OutPkts, prev.OutPkts),
		InBytes:     deltaUint64(current.InBytes, prev.InBytes),
		OutBytes:    deltaUint64(current.OutBytes, prev.OutBytes),
	}
}

// diffBackendStats computes the per-poll delta of cumulative backend counters
// between two raw snapshots.
func diffBackendStats(prev, current BackendTrafficStats) BackendTrafficStats {
	return BackendTrafficStats{
		ServiceKey:  current.ServiceKey,
		Connections: deltaUint64(current.Connections, prev.Connections),
		InPkts:      deltaUint64(current.InPkts, prev.InPkts),
		OutPkts:     deltaUint64(current.OutPkts, prev.OutPkts),
		InBytes:     deltaUint64(current.InBytes, prev.InBytes),
		OutBytes:    deltaUint64(current.OutBytes, prev.OutBytes),
	}
}

// deltaUint64 returns the per-poll delta between a cumulative counter's
// current and previous value. IPVS resets a service/destination's counters
// to zero when it is recreated by a reconcile, so if current has gone
// backwards relative to previous, current is used as the delta directly
// (standard counter-reset handling) instead of underflowing a uint64.
func deltaUint64(current, previous uint64) uint64 {
	if current < previous {
		return current
	}
	return current - previous
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
