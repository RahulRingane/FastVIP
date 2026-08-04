package trafficlog

import (
	"fmt"

	"github.com/RahulRingane/FastVIP/pkg/lvs"
)

// lvsStatsAdapter implements LVSStatsProvider by adapting lvs.Manager.
// It reuses GetServices() and GetDestinations() to retrieve statistics
// without modifying the IPVSHandle interface.
type lvsStatsAdapter struct {
	manager *lvs.Manager
}

// NewLVSStatsAdapter creates an LVSStatsProvider backed by lvs.Manager.
func NewLVSStatsAdapter(mgr *lvs.Manager) LVSStatsProvider {
	return &lvsStatsAdapter{manager: mgr}
}

// ServiceStats retrieves cumulative statistics for all IPVS services.
func (a *lvsStatsAdapter) ServiceStats() (map[string]ServiceTrafficStats, error) {
	services, err := a.manager.GetServices()
	if err != nil {
		return nil, fmt.Errorf("failed to get IPVS services: %w", err)
	}

	result := make(map[string]ServiceTrafficStats, len(services))
	for _, svc := range services {
		key := lvs.ServiceKeyFromIPVS(svc).String()
		result[key] = ServiceTrafficStats{
			Connections: uint64(svc.Stats.Connections),
			InPkts:      uint64(svc.Stats.PacketsIn),
			OutPkts:     uint64(svc.Stats.PacketsOut),
			InBytes:     svc.Stats.BytesIn,
			OutBytes:    svc.Stats.BytesOut,
		}
	}
	return result, nil
}

// BackendStats retrieves cumulative statistics for all IPVS backends (destinations).
// The key format is "svcKey->dstKey" to uniquely identify each backend across services.
func (a *lvsStatsAdapter) BackendStats() (map[string]BackendTrafficStats, error) {
	services, err := a.manager.GetServices()
	if err != nil {
		return nil, fmt.Errorf("failed to get IPVS services: %w", err)
	}

	result := make(map[string]BackendTrafficStats)
	for _, svc := range services {
		svcKey := lvs.ServiceKeyFromIPVS(svc).String()

		dests, err := a.manager.GetDestinations(svc)
		if err != nil {
			return nil, fmt.Errorf("failed to get destinations for service %s: %w", svcKey, err)
		}

		for _, dst := range dests {
			dstKey := lvs.DestinationKeyFromIPVS(dst).String()
			fullKey := fmt.Sprintf("%s->%s", svcKey, dstKey)
			activeConnections := connectionCountUint64(dst.ActiveConnections)
			inactiveConnections := connectionCountUint64(dst.InactiveConnections)
			result[fullKey] = BackendTrafficStats{
				ServiceKey:          svcKey,
				Connections:         uint64(dst.Stats.Connections),
				ActiveConnections:   activeConnections,
				InactiveConnections: inactiveConnections,
				CurrentConnections:  activeConnections + inactiveConnections,
				InPkts:              uint64(dst.Stats.PacketsIn),
				OutPkts:             uint64(dst.Stats.PacketsOut),
				InBytes:             dst.Stats.BytesIn,
				OutBytes:            dst.Stats.BytesOut,
			}
		}
	}
	return result, nil
}

func connectionCountUint64(n int) uint64 {
	if n < 0 {
		return 0
	}
	return uint64(n)
}
