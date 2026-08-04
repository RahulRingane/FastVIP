package lvs

import (
	"fmt"

	"go.uber.org/zap"
)

// Manager wraps the IPVSHandle and provides IPVS CRUD operations with logging.
type Manager struct {
	handle IPVSHandle
	logger *zap.Logger
}

// NewManager creates a new IPVS Manager by initializing a platform-specific handle.
func NewManager(logger *zap.Logger) (*Manager, error) {
	handle, err := NewIPVSHandle("")
	if err != nil {
		return nil, fmt.Errorf("failed to create ipvs handle: %w", err)
	}

	logger.Info("IPVS manager initialized")
	return &Manager{
		handle: handle,
		logger: logger,
	}, nil
}

// newManagerWithHandle creates a Manager with a pre-initialized IPVSHandle.
// This is used in tests to inject a specific handle implementation.
func newManagerWithHandle(handle IPVSHandle, logger *zap.Logger) *Manager {
	return &Manager{
		handle: handle,
		logger: logger,
	}
}

// Close releases the IPVS handle.
func (m *Manager) Close() {
	m.handle.Close()
	m.logger.Info("IPVS manager closed")
}

// GetServices returns all IPVS virtual services currently configured.
func (m *Manager) GetServices() ([]*Service, error) {
	services, err := m.handle.GetServices()
	if err != nil {
		return nil, fmt.Errorf("failed to get ipvs services: %w", err)
	}
	return services, nil
}

// GetDestinations returns all real servers (destinations) for the given IPVS service.
func (m *Manager) GetDestinations(svc *Service) ([]*Destination, error) {
	destinations, err := m.handle.GetDestinations(svc)
	if err != nil {
		return nil, fmt.Errorf("failed to get destinations for service %s:%d: %w",
			svc.Address, svc.Port, err)
	}
	return destinations, nil
}

// CreateService creates a new IPVS virtual service.
func (m *Manager) CreateService(svc *Service) error {
	if err := m.handle.NewService(svc); err != nil {
		return fmt.Errorf("failed to create service %s:%d: %w",
			svc.Address, svc.Port, err)
	}
	m.logger.Info("created IPVS service",
		zap.String("address", svc.Address.String()),
		zap.Uint16("port", svc.Port),
		zap.String("scheduler", svc.SchedName),
	)
	return nil
}

// UpdateService updates an existing IPVS virtual service.
func (m *Manager) UpdateService(svc *Service) error {
	if err := m.handle.UpdateService(svc); err != nil {
		return fmt.Errorf("failed to update service %s:%d: %w",
			svc.Address, svc.Port, err)
	}
	m.logger.Info("updated IPVS service",
		zap.String("address", svc.Address.String()),
		zap.Uint16("port", svc.Port),
		zap.String("scheduler", svc.SchedName),
	)
	return nil
}

// DeleteService removes an IPVS virtual service.
func (m *Manager) DeleteService(svc *Service) error {
	if err := m.handle.DelService(svc); err != nil {
		return fmt.Errorf("failed to delete service %s:%d: %w",
			svc.Address, svc.Port, err)
	}
	m.logger.Info("deleted IPVS service",
		zap.String("address", svc.Address.String()),
		zap.Uint16("port", svc.Port),
	)
	return nil
}

// CreateDestination adds a new real server to the given IPVS service.
func (m *Manager) CreateDestination(svc *Service, dst *Destination) error {
	if err := m.handle.NewDestination(svc, dst); err != nil {
		return fmt.Errorf("failed to create destination %s:%d for service %s:%d: %w",
			dst.Address, dst.Port, svc.Address, svc.Port, err)
	}
	m.logger.Info("created IPVS destination",
		zap.String("service", fmt.Sprintf("%s:%d", svc.Address, svc.Port)),
		zap.String("destination", fmt.Sprintf("%s:%d", dst.Address, dst.Port)),
		zap.Int("weight", dst.Weight),
	)
	return nil
}

// UpdateDestination updates an existing real server in the given IPVS service.
func (m *Manager) UpdateDestination(svc *Service, dst *Destination) error {
	if err := m.handle.UpdateDestination(svc, dst); err != nil {
		return fmt.Errorf("failed to update destination %s:%d for service %s:%d: %w",
			dst.Address, dst.Port, svc.Address, svc.Port, err)
	}
	m.logger.Info("updated IPVS destination",
		zap.String("service", fmt.Sprintf("%s:%d", svc.Address, svc.Port)),
		zap.String("destination", fmt.Sprintf("%s:%d", dst.Address, dst.Port)),
		zap.Int("weight", dst.Weight),
	)
	return nil
}

// DeleteDestination removes a real server from the given IPVS service.
func (m *Manager) DeleteDestination(svc *Service, dst *Destination) error {
	if err := m.handle.DelDestination(svc, dst); err != nil {
		return fmt.Errorf("failed to delete destination %s:%d for service %s:%d: %w",
			dst.Address, dst.Port, svc.Address, svc.Port, err)
	}
	m.logger.Info("deleted IPVS destination",
		zap.String("service", fmt.Sprintf("%s:%d", svc.Address, svc.Port)),
		zap.String("destination", fmt.Sprintf("%s:%d", dst.Address, dst.Port)),
	)
	return nil
}

// Flush removes all IPVS services and destinations.
func (m *Manager) Flush() error {
	if err := m.handle.Flush(); err != nil {
		return fmt.Errorf("failed to flush IPVS rules: %w", err)
	}
	m.logger.Info("flushed all IPVS rules")
	return nil
}
