package trafficlog

// ServiceTrafficStats holds cumulative IPVS service-level statistics.
type ServiceTrafficStats struct {
	Connections uint64
	InPkts      uint64
	OutPkts     uint64
	InBytes     uint64
	OutBytes    uint64
}

// BackendTrafficStats holds IPVS backend-level traffic and current connection statistics.
type BackendTrafficStats struct {
	ServiceKey          string
	Connections         uint64
	ActiveConnections   uint64
	InactiveConnections uint64
	CurrentConnections  uint64
	InPkts              uint64
	OutPkts             uint64
	InBytes             uint64
	OutBytes            uint64
}

// TrafficSnapshot holds a point-in-time snapshot of all statistics.
type TrafficSnapshot struct {
	Services map[string]ServiceTrafficStats
	Backends map[string]BackendTrafficStats
}

// LVSStatsProvider abstracts IPVS statistics retrieval.
type LVSStatsProvider interface {
	ServiceStats() (map[string]ServiceTrafficStats, error)
	BackendStats() (map[string]BackendTrafficStats, error)
}
