package healthcheck

import (
	"context"
	"sync"
	"time"

	"github.com/RahulRingane/FastVIP/pkg/config"
	"go.uber.org/zap"
)

// backendStatus tracks the health state and consecutive check results for a single backend.
type backendStatus struct {
	cancel           context.CancelFunc
	address          string
	consecutiveFails int
	consecutiveOK    int
	healthy          bool
}

// serviceCheckConfig holds the health check parameters for a specific service's backends.
type serviceCheckConfig struct {
	checker   Checker
	interval  time.Duration
	failCount int
	riseCount int
	enabled   bool
}

// Manager orchestrates health checks for all backends across all services.
type Manager struct {
	services map[string]*serviceCheckConfig
	statuses map[string]*backendStatus
	onChange func()
	logger   *zap.Logger
	mu       sync.RWMutex
}

// NewManager creates a new health check Manager.
// The onChange callback is invoked whenever a backend's health status changes.
func NewManager(onChange func(), logger *zap.Logger) *Manager {
	return &Manager{
		services: make(map[string]*serviceCheckConfig),
		statuses: make(map[string]*backendStatus),
		onChange: onChange,
		logger:   logger,
	}
}

// IsHealthy returns whether the given backend address is considered healthy.
// Backends belonging to services with health check disabled always return true.
// Backends not tracked (unknown) are considered healthy by default.
func (m *Manager) IsHealthy(address string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status, exists := m.statuses[address]
	if !exists {
		return true
	}
	return status.healthy
}

// UpdateTargets synchronizes the health check targets with the current configuration.
// It starts checks for new backends, stops checks for removed backends,
// and handles enable/disable transitions for each service.
func (m *Manager) UpdateTargets(ctx context.Context, services []config.ServiceConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Build the new desired state
	newServiceNames := make(map[string]bool)
	newBackendAddresses := make(map[string]bool)

	for _, svcCfg := range services {
		newServiceNames[svcCfg.Name] = true

		if !svcCfg.HealthCheck.IsEnabled() {
			// Service has health check disabled
			oldSvcCheck, existed := m.services[svcCfg.Name]
			if existed && oldSvcCheck.enabled {
				// Transition: enabled -> disabled, stop all checks for this service's backends
				m.stopServiceBackendsLocked(svcCfg)
			}
			m.services[svcCfg.Name] = &serviceCheckConfig{
				enabled: false,
			}
			// Mark backends as not tracked (will return healthy by default)
			for _, backend := range svcCfg.Backends {
				newBackendAddresses[backend.Address] = true
			}
			continue
		}

		// Service has health check enabled — select checker by type
		var checker Checker
		switch svcCfg.HealthCheck.GetType() {
		case "http":
			checker = NewHTTPChecker(
				svcCfg.HealthCheck.GetTimeout(),
				svcCfg.HealthCheck.GetHTTPPath(),
				svcCfg.HealthCheck.GetHTTPExpectedStatus(),
			)
		default:
			checker = NewTCPChecker(svcCfg.HealthCheck.GetTimeout())
		}
		svcCheck := &serviceCheckConfig{
			checker:   checker,
			interval:  svcCfg.HealthCheck.GetInterval(),
			failCount: svcCfg.HealthCheck.GetFailCount(),
			riseCount: svcCfg.HealthCheck.GetRiseCount(),
			enabled:   true,
		}
		m.services[svcCfg.Name] = svcCheck

		for _, backend := range svcCfg.Backends {
			newBackendAddresses[backend.Address] = true

			if _, exists := m.statuses[backend.Address]; !exists {
				// New backend: start health check, initial state is healthy
				m.startBackendCheckLocked(ctx, backend.Address, svcCheck)
			}
		}
	}

	// Stop checks for removed services
	for svcName := range m.services {
		if !newServiceNames[svcName] {
			delete(m.services, svcName)
		}
	}

	// Stop checks for removed backends
	for address, status := range m.statuses {
		if !newBackendAddresses[address] {
			if status.cancel != nil {
				status.cancel()
			}
			delete(m.statuses, address)
			m.logger.Info("stopped health check for removed backend", zap.String("address", address))
		}
	}
}

// stopServiceBackendsLocked stops health checks for all backends of a service.
// Must be called with m.mu held.
func (m *Manager) stopServiceBackendsLocked(svcCfg config.ServiceConfig) {
	for _, backend := range svcCfg.Backends {
		if status, exists := m.statuses[backend.Address]; exists {
			if status.cancel != nil {
				status.cancel()
			}
			delete(m.statuses, backend.Address)
			m.logger.Info("stopped health check (service disabled)",
				zap.String("service", svcCfg.Name),
				zap.String("address", backend.Address),
			)
		}
	}
}

// startBackendCheckLocked starts a health check goroutine for a single backend.
// Must be called with m.mu held.
func (m *Manager) startBackendCheckLocked(ctx context.Context, address string, svcCheck *serviceCheckConfig) {
	checkCtx, cancel := context.WithCancel(ctx)
	status := &backendStatus{
		address: address,
		healthy: true,
		cancel:  cancel,
	}
	m.statuses[address] = status

	m.logger.Info("started health check for backend", zap.String("address", address))

	go m.runCheck(checkCtx, address, svcCheck)
}

// runCheck is the health check loop for a single backend.
// It periodically probes the backend and updates its health status.
func (m *Manager) runCheck(ctx context.Context, address string, svcCheck *serviceCheckConfig) {
	ticker := time.NewTicker(svcCheck.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := svcCheck.checker.Check(address)
			m.handleCheckResult(address, err, svcCheck)
		}
	}
}

// handleCheckResult processes a single health check result and updates the backend status.
// Triggers onChange callback if the health status transitions.
func (m *Manager) handleCheckResult(address string, checkErr error, svcCheck *serviceCheckConfig) {
	m.mu.Lock()

	status, exists := m.statuses[address]
	if !exists {
		m.mu.Unlock()
		return
	}

	previouslyHealthy := status.healthy

	if checkErr != nil {
		// Check failed
		status.consecutiveFails++
		status.consecutiveOK = 0

		if status.healthy && status.consecutiveFails >= svcCheck.failCount {
			status.healthy = false
			m.logger.Warn("backend marked unhealthy",
				zap.String("address", address),
				zap.Int("consecutive_fails", status.consecutiveFails),
				zap.Error(checkErr),
			)
		}
	} else {
		// Check succeeded
		status.consecutiveOK++
		status.consecutiveFails = 0

		if !status.healthy && status.consecutiveOK >= svcCheck.riseCount {
			status.healthy = true
			m.logger.Info("backend marked healthy",
				zap.String("address", address),
				zap.Int("consecutive_ok", status.consecutiveOK),
			)
		}
	}

	statusChanged := previouslyHealthy != status.healthy
	m.mu.Unlock()

	if statusChanged && m.onChange != nil {
		m.onChange()
	}
}

// GetAllStatuses returns a copy of all backend health statuses.
// The key format is "serviceName/backendAddress".
func (m *Manager) GetAllStatuses() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]bool, len(m.statuses))
	for address, status := range m.statuses {
		result[address] = status.healthy
	}
	return result
}

// Stop cancels all running health check goroutines and clears state.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for address, status := range m.statuses {
		if status.cancel != nil {
			status.cancel()
		}
		m.logger.Debug("stopped health check", zap.String("address", address))
	}

	m.statuses = make(map[string]*backendStatus)
	m.services = make(map[string]*serviceCheckConfig)
	m.logger.Info("all health checks stopped")
}
