//go:build !integration

package snat

import (
	"sync"

	"go.uber.org/zap"
)

// FakeManager provides an in-memory SNAT and FORWARD rule manager for non-Linux systems.
// It simulates iptables behavior for development and testing on macOS.
type FakeManager struct {
	managed        map[string]SNATRule
	managedForward map[string]ForwardRule
	logger         *zap.Logger
	mu             sync.Mutex
}

// NewManager creates a fake in-memory SNAT Manager for non-Linux systems.
func NewManager(logger *zap.Logger) (Manager, error) {
	return &FakeManager{
		managed:        make(map[string]SNATRule),
		managedForward: make(map[string]ForwardRule),
		logger:         logger,
	}, nil
}

// Reconcile compares desired SNAT rules with the currently managed set in memory.
func (m *FakeManager) Reconcile(desired []SNATRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	desiredMap := make(map[string]SNATRule, len(desired))
	for _, rule := range desired {
		desiredMap[rule.Key()] = rule
	}

	// Remove stale rules
	for key := range m.managed {
		if _, exists := desiredMap[key]; !exists {
			delete(m.managed, key)
			m.logger.Debug("fake: deleted SNAT rule", zap.String("key", key))
		}
	}

	// Add or update rules
	for key, rule := range desiredMap {
		existing, exists := m.managed[key]
		if exists && existing.SnatIP == rule.SnatIP {
			continue
		}
		m.managed[key] = rule
		m.logger.Debug("fake: added SNAT rule", zap.String("key", key), zap.String("snat_ip", rule.SnatIP))
	}

	return nil
}

// ReconcileForward compares desired FORWARD rules with the currently managed set in memory.
func (m *FakeManager) ReconcileForward(desired []ForwardRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	desiredMap := make(map[string]ForwardRule, len(desired))
	for _, rule := range desired {
		desiredMap[rule.Key()] = rule
	}

	// Remove stale rules
	for key := range m.managedForward {
		if _, exists := desiredMap[key]; !exists {
			delete(m.managedForward, key)
			m.logger.Debug("fake: deleted FORWARD rule", zap.String("key", key))
		}
	}

	// Add missing rules
	for key, rule := range desiredMap {
		if _, exists := m.managedForward[key]; exists {
			continue
		}
		m.managedForward[key] = rule
		m.logger.Debug("fake: added FORWARD rule", zap.String("key", key))
	}

	return nil
}

// Cleanup removes all managed SNAT and FORWARD rules from memory.
func (m *FakeManager) Cleanup() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.managed = make(map[string]SNATRule)
	m.managedForward = make(map[string]ForwardRule)
	m.logger.Debug("fake: cleaned up all SNAT and FORWARD rules")
	return nil
}

// GetManaged returns a copy of the currently managed SNAT rules (for testing).
func (m *FakeManager) GetManaged() map[string]SNATRule {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make(map[string]SNATRule, len(m.managed))
	for k, v := range m.managed {
		result[k] = v
	}
	return result
}

// GetManagedForward returns a copy of the currently managed FORWARD rules (for testing).
func (m *FakeManager) GetManagedForward() map[string]ForwardRule {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make(map[string]ForwardRule, len(m.managedForward))
	for k, v := range m.managedForward {
		result[k] = v
	}
	return result
}
