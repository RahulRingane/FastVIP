package snat

// StatsProvider provides SNAT rule statistics.
// This is a separate interface from Manager to avoid mixing query and reconcile concerns.
// Implementations that support statistics (e.g. linuxManager) can implement both interfaces.
// The traffic collector uses type assertion to check if the Manager also implements StatsProvider:
//
//	if sp, ok := snatMgr.(snat.StatsProvider); ok { ... }
type StatsProvider interface {
	Stats() (map[string]SNATRuleStats, error)
}

// SNATRuleStats holds cumulative statistics for a single SNAT rule.
type SNATRuleStats struct {
	Packets uint64
	Bytes   uint64
}
