package lvs

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/RahulRingane/FastVIP/pkg/config"
	"github.com/RahulRingane/FastVIP/pkg/metrics"
	"github.com/RahulRingane/FastVIP/pkg/snat"
	"go.uber.org/zap"
)

// HealthChecker is the interface used by Reconciler to query backend health status.
// This decouples the lvs package from the healthcheck package.
type HealthChecker interface {
	IsHealthy(address string) bool
}

// Reconciler implements declarative reconciliation between desired state (config + health)
// and actual state (IPVS kernel rules + iptables SNAT rules).
type Reconciler struct {
	manager   *Manager
	healthMgr HealthChecker
	snatMgr   snat.Manager
	logger    *zap.Logger
	managed   map[ServiceKey]bool // tracks services managed by fastVIP
	mu        sync.Mutex
}

// NewReconciler creates a new Reconciler.
func NewReconciler(manager *Manager, healthMgr HealthChecker, snatMgr snat.Manager, logger *zap.Logger) *Reconciler {
	return &Reconciler{
		manager:   manager,
		healthMgr: healthMgr,
		snatMgr:   snatMgr,
		logger:    logger,
		managed:   make(map[ServiceKey]bool),
	}
}

// desiredService holds the desired IPVS service and its destinations after health filtering.
type desiredService struct {
	service      *Service
	destinations []*Destination
	config       config.ServiceConfig
}

// Reconcile compares the desired state (from config + health check) with the actual IPVS state
// and applies the necessary changes to bring the kernel in sync.
func (r *Reconciler) Reconcile(desiredConfigs []config.ServiceConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger.Info("starting reconcile", zap.Int("desired_services", len(desiredConfigs)))

	// Phase 1: Build desired state
	desiredMap, err := r.buildDesiredState(desiredConfigs)
	if err != nil {
		return fmt.Errorf("failed to build desired state: %w", err)
	}

	// Phase 2: Get actual state from IPVS kernel
	actualServices, err := r.manager.GetServices()
	if err != nil {
		return fmt.Errorf("failed to get current IPVS services: %w", err)
	}

	actualMap := make(map[ServiceKey]*Service)
	for _, svc := range actualServices {
		key := ServiceKeyFromIPVS(svc)
		// Include services that are either managed by fastVIP or present in the
		// desired state. This ensures that `once` mode (fresh Reconciler with
		// empty managed map) can still detect and update pre-existing IPVS
		// services that match the current config, avoiding duplicate creation.
		if r.managed[key] || desiredMap[key] != nil {
			actualMap[key] = svc
		}
	}

	var reconcileErrors []error

	// Phase 3: Service-level diff
	// Create or update services that are in desired but missing or different in actual
	for key, desired := range desiredMap {
		actual, exists := actualMap[key]
		if !exists {
			// Service does not exist in IPVS -> create it
			if err := r.manager.CreateService(desired.service); err != nil {
				reconcileErrors = append(reconcileErrors, fmt.Errorf("create service %s: %w", key, err))
				continue
			}
			r.managed[key] = true
		} else {
			// Service exists -> mark as managed and check if scheduler needs update
			r.managed[key] = true
			if actual.SchedName != desired.service.SchedName {
				if err := r.manager.UpdateService(desired.service); err != nil {
					reconcileErrors = append(reconcileErrors, fmt.Errorf("update service %s: %w", key, err))
					continue
				}
			}
		}

		// Phase 4: Destination-level diff for this service
		if err := r.reconcileDestinations(desired); err != nil {
			reconcileErrors = append(reconcileErrors, err)
		}
	}

	// Delete services that are in actual (and managed by fastVIP) but not in desired
	for key, actual := range actualMap {
		if _, exists := desiredMap[key]; !exists {
			if err := r.manager.DeleteService(actual); err != nil {
				reconcileErrors = append(reconcileErrors, fmt.Errorf("delete service %s: %w", key, err))
			} else {
				delete(r.managed, key)
			}
		}
	}

	// Phase 5: Reconcile SNAT rules for services with full_nat enabled
	if err := r.reconcileSNAT(desiredConfigs); err != nil {
		reconcileErrors = append(reconcileErrors, fmt.Errorf("snat reconcile: %w", err))
	}

	if len(reconcileErrors) > 0 {
		r.logger.Error("reconcile completed with errors", zap.Int("error_count", len(reconcileErrors)))
		// Increment error counter for each error
		for range reconcileErrors {
			metrics.IncReconcileErrors()
		}
		return errors.Join(reconcileErrors...)
	}

	r.logger.Info("reconcile completed successfully")
	return nil
}

// Cleanup removes all IPVS services currently managed by this Reconciler.
// It only deletes services tracked in the managed map, leaving other IPVS
// rules untouched.
func (r *Reconciler) Cleanup() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	actualServices, err := r.manager.GetServices()
	if err != nil {
		return fmt.Errorf("failed to get IPVS services for cleanup: %w", err)
	}

	actualMap := make(map[ServiceKey]*Service)
	for _, svc := range actualServices {
		actualMap[ServiceKeyFromIPVS(svc)] = svc
	}

	var errs []error
	for key := range r.managed {
		svc, exists := actualMap[key]
		if !exists {
			delete(r.managed, key)
			continue
		}
		if err := r.manager.DeleteService(svc); err != nil {
			errs = append(errs, fmt.Errorf("delete service %s: %w", key, err))
		} else {
			delete(r.managed, key)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	r.logger.Info("cleaned up all managed IPVS services")
	return nil
}

// reconcileSNAT builds the desired SNAT and FORWARD rules from configs with
// full_nat enabled and delegates to the SNAT manager for declarative reconciliation.
// FORWARD rules are needed because IPVS NAT mode requires packets to traverse
// the FORWARD chain, which may have a DROP policy (e.g. Docker environments).
func (r *Reconciler) reconcileSNAT(configs []config.ServiceConfig) error {
	var desiredSNATRules []snat.SNATRule
	var desiredForwardRules []snat.ForwardRule

	for _, svcCfg := range configs {
		if !svcCfg.FullNAT {
			continue
		}

		for _, backendCfg := range svcCfg.Backends {
			// Only create rules for healthy backends
			if svcCfg.HealthCheck.IsEnabled() && !r.healthMgr.IsHealthy(backendCfg.Address) {
				continue
			}

			backendHost, backendPortStr, err := net.SplitHostPort(backendCfg.Address)
			if err != nil {
				return fmt.Errorf("service %q, backend %q: invalid address: %w", svcCfg.Name, backendCfg.Address, err)
			}
			backendPort, err := strconv.Atoi(backendPortStr)
			if err != nil {
				return fmt.Errorf("service %q, backend %q: invalid port: %w", svcCfg.Name, backendCfg.Address, err)
			}

			protocol := svcCfg.Protocol
			if protocol == "" {
				protocol = "tcp"
			}

			desiredSNATRules = append(desiredSNATRules, snat.SNATRule{
				BackendIP:   backendHost,
				BackendPort: uint16(backendPort),
				Protocol:    protocol,
				SnatIP:      svcCfg.SnatIP,
			})

			desiredForwardRules = append(desiredForwardRules, snat.ForwardRule{
				BackendIP:   backendHost,
				BackendPort: uint16(backendPort),
				Protocol:    protocol,
			})
		}
	}

	if err := r.snatMgr.Reconcile(desiredSNATRules); err != nil {
		return fmt.Errorf("snat rules: %w", err)
	}

	if err := r.snatMgr.ReconcileForward(desiredForwardRules); err != nil {
		return fmt.Errorf("forward rules: %w", err)
	}

	return nil
}

// buildDesiredState converts config services into the desired IPVS state,
// filtering out unhealthy backends.
func (r *Reconciler) buildDesiredState(configs []config.ServiceConfig) (map[ServiceKey]*desiredService, error) {
	result := make(map[ServiceKey]*desiredService)

	for _, svcCfg := range configs {
		ipvsSvc, err := ConfigToIPVSService(svcCfg)
		if err != nil {
			return nil, fmt.Errorf("service %q: %w", svcCfg.Name, err)
		}

		key, err := ServiceKeyFromConfig(svcCfg)
		if err != nil {
			return nil, fmt.Errorf("service %q: %w", svcCfg.Name, err)
		}

		var destinations []*Destination
		for _, backendCfg := range svcCfg.Backends {
			// Filter out unhealthy backends (only when health check is enabled)
			if svcCfg.HealthCheck.IsEnabled() && !r.healthMgr.IsHealthy(backendCfg.Address) {
				r.logger.Info("skipping unhealthy backend",
					zap.String("service", svcCfg.Name),
					zap.String("backend", backendCfg.Address),
				)
				continue
			}

			dst, err := ConfigToIPVSDestination(backendCfg)
			if err != nil {
				return nil, fmt.Errorf("service %q, backend %q: %w", svcCfg.Name, backendCfg.Address, err)
			}
			destinations = append(destinations, dst)
		}

		result[key] = &desiredService{
			service:      ipvsSvc,
			destinations: destinations,
			config:       svcCfg,
		}
	}

	return result, nil
}

// reconcileDestinations performs a diff on destinations for a single service.
func (r *Reconciler) reconcileDestinations(desired *desiredService) error {
	// Get actual destinations from IPVS
	actualDests, err := r.manager.GetDestinations(desired.service)
	if err != nil {
		return fmt.Errorf("get destinations for %s:%d: %w",
			desired.service.Address, desired.service.Port, err)
	}

	// Build maps for comparison
	actualDestMap := make(map[DestinationKey]*Destination)
	for _, dst := range actualDests {
		key := DestinationKeyFromIPVS(dst)
		actualDestMap[key] = dst
	}

	desiredDestMap := make(map[DestinationKey]*Destination)
	for _, dst := range desired.destinations {
		key := DestinationKey{
			Address: dst.Address.String(),
			Port:    dst.Port,
		}
		desiredDestMap[key] = dst
	}

	var reconcileErrors []error

	// Create or update destinations
	for key, desiredDst := range desiredDestMap {
		actualDst, exists := actualDestMap[key]
		if !exists {
			// Destination does not exist -> create
			if err := r.manager.CreateDestination(desired.service, desiredDst); err != nil {
				reconcileErrors = append(reconcileErrors, fmt.Errorf("create destination %s: %w", key, err))
			}
		} else {
			// Destination exists -> check if weight needs update
			if actualDst.Weight != desiredDst.Weight {
				if err := r.manager.UpdateDestination(desired.service, desiredDst); err != nil {
					reconcileErrors = append(reconcileErrors, fmt.Errorf("update destination %s: %w", key, err))
				}
			}
		}
	}

	// Delete destinations that are in actual but not in desired
	for key, actualDst := range actualDestMap {
		if _, exists := desiredDestMap[key]; !exists {
			if err := r.manager.DeleteDestination(desired.service, actualDst); err != nil {
				reconcileErrors = append(reconcileErrors, fmt.Errorf("delete destination %s: %w", key, err))
			}
		}
	}

	if len(reconcileErrors) > 0 {
		return errors.Join(reconcileErrors...)
	}
	return nil
}
