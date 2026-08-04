//go:build integration

package snat

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/coreos/go-iptables/iptables"
	"go.uber.org/zap"
)

const (
	natTable     = "nat"
	filterTable  = "filter"
	snatChain    = "EZLB-SNAT"
	forwardChain = "EZLB-FORWARD"
)

// linuxManager manages iptables SNAT and FORWARD rules on Linux using coreos/go-iptables.
type linuxManager struct {
	ipt            *iptables.IPTables
	managed        map[string]SNATRule
	managedForward map[string]ForwardRule
	mu             sync.Mutex
	logger         *zap.Logger
}

// NewManager creates a new SNAT Manager backed by real iptables operations.
func NewManager(logger *zap.Logger) (Manager, error) {
	ipt, err := iptables.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create iptables handle: %w", err)
	}

	mgr := &linuxManager{
		ipt:            ipt,
		managed:        make(map[string]SNATRule),
		managedForward: make(map[string]ForwardRule),
		logger:         logger,
	}

	if err := mgr.ensureChain(); err != nil {
		return nil, fmt.Errorf("failed to initialize SNAT chain: %w", err)
	}

	if err := mgr.ensureForwardChain(); err != nil {
		return nil, fmt.Errorf("failed to initialize FORWARD chain: %w", err)
	}

	return mgr, nil
}

// ensureChain creates the EZLB-SNAT chain and adds a jump rule from POSTROUTING.
func (m *linuxManager) ensureChain() error {
	exists, err := m.ipt.ChainExists(natTable, snatChain)
	if err != nil {
		return fmt.Errorf("failed to check chain existence: %w", err)
	}
	if !exists {
		if err := m.ipt.NewChain(natTable, snatChain); err != nil {
			return fmt.Errorf("failed to create chain %s: %w", snatChain, err)
		}
		m.logger.Debug("created iptables chain", zap.String("chain", snatChain))
	}

	jumpRule := []string{"-j", snatChain}
	if err := m.ipt.AppendUnique(natTable, "POSTROUTING", jumpRule...); err != nil {
		return fmt.Errorf("failed to add jump rule to POSTROUTING: %w", err)
	}

	return nil
}

// ensureForwardChain creates the EZLB-FORWARD chain in the filter table and adds
// a jump rule from FORWARD, plus a conntrack ESTABLISHED,RELATED accept rule.
func (m *linuxManager) ensureForwardChain() error {
	exists, err := m.ipt.ChainExists(filterTable, forwardChain)
	if err != nil {
		return fmt.Errorf("failed to check chain existence: %w", err)
	}
	if !exists {
		if err := m.ipt.NewChain(filterTable, forwardChain); err != nil {
			return fmt.Errorf("failed to create chain %s: %w", forwardChain, err)
		}
		m.logger.Debug("created iptables chain", zap.String("chain", forwardChain))
	}

	// Insert jump rule at the top of FORWARD chain so it takes priority.
	// Use Exists + Insert for idempotency since go-iptables has no InsertUnique.
	jumpRule := []string{"-j", forwardChain}
	jumpExists, err := m.ipt.Exists(filterTable, "FORWARD", jumpRule...)
	if err != nil {
		return fmt.Errorf("failed to check jump rule in FORWARD: %w", err)
	}
	if !jumpExists {
		if err := m.ipt.Insert(filterTable, "FORWARD", 1, jumpRule...); err != nil {
			return fmt.Errorf("failed to add jump rule to FORWARD: %w", err)
		}
	}

	// Add a conntrack rule to accept ESTABLISHED,RELATED packets (return traffic)
	conntrackRule := []string{"-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"}
	if err := m.ipt.AppendUnique(filterTable, forwardChain, conntrackRule...); err != nil {
		return fmt.Errorf("failed to add conntrack rule to %s: %w", forwardChain, err)
	}

	return nil
}

// Reconcile compares desired SNAT rules with the currently managed set,
// adding missing rules and removing stale ones.
func (m *linuxManager) Reconcile(desired []SNATRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	desiredMap := make(map[string]SNATRule, len(desired))
	for _, rule := range desired {
		desiredMap[rule.Key()] = rule
	}

	// Remove rules that are no longer desired
	for key, rule := range m.managed {
		if _, exists := desiredMap[key]; !exists {
			if err := m.deleteRule(rule); err != nil {
				m.logger.Error("failed to delete SNAT rule", zap.String("key", key), zap.Error(err))
			} else {
				delete(m.managed, key)
				m.logger.Debug("deleted SNAT rule", zap.String("key", key))
			}
		}
	}

	// Add rules that are missing or have changed snat_ip
	for key, rule := range desiredMap {
		existing, exists := m.managed[key]
		if exists && existing.SnatIP == rule.SnatIP {
			continue
		}
		// If snat_ip changed, remove the old rule first
		if exists {
			if err := m.deleteRule(existing); err != nil {
				m.logger.Error("failed to delete old SNAT rule for update", zap.String("key", key), zap.Error(err))
				continue
			}
		}
		if err := m.addRule(rule); err != nil {
			m.logger.Error("failed to add SNAT rule", zap.String("key", key), zap.Error(err))
		} else {
			m.managed[key] = rule
			m.logger.Debug("added SNAT rule", zap.String("key", key), zap.String("snat_ip", rule.SnatIP))
		}
	}

	return nil
}

// ReconcileForward compares desired FORWARD rules with the currently managed set,
// adding missing rules and removing stale ones. These rules allow IPVS NAT
// traffic to pass through the FORWARD chain even when the default policy is DROP.
func (m *linuxManager) ReconcileForward(desired []ForwardRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	desiredMap := make(map[string]ForwardRule, len(desired))
	for _, rule := range desired {
		desiredMap[rule.Key()] = rule
	}

	// Remove rules that are no longer desired
	for key, rule := range m.managedForward {
		if _, exists := desiredMap[key]; !exists {
			if err := m.deleteForwardRule(rule); err != nil {
				m.logger.Error("failed to delete FORWARD rule", zap.String("key", key), zap.Error(err))
			} else {
				delete(m.managedForward, key)
				m.logger.Debug("deleted FORWARD rule", zap.String("key", key))
			}
		}
	}

	// Add rules that are missing
	for key, rule := range desiredMap {
		if _, exists := m.managedForward[key]; exists {
			continue
		}
		if err := m.addForwardRule(rule); err != nil {
			m.logger.Error("failed to add FORWARD rule", zap.String("key", key), zap.Error(err))
		} else {
			m.managedForward[key] = rule
			m.logger.Debug("added FORWARD rule", zap.String("key", key))
		}
	}

	return nil
}

// Cleanup removes all managed SNAT/FORWARD rules, jump rules, and custom chains.
func (m *linuxManager) Cleanup() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Clean up SNAT chain
	if err := m.ipt.ClearChain(natTable, snatChain); err != nil {
		m.logger.Error("failed to clear SNAT chain", zap.Error(err))
	}

	jumpRule := []string{"-j", snatChain}
	if err := m.ipt.DeleteIfExists(natTable, "POSTROUTING", jumpRule...); err != nil {
		m.logger.Error("failed to delete jump rule from POSTROUTING", zap.Error(err))
	}

	if err := m.ipt.DeleteChain(natTable, snatChain); err != nil {
		m.logger.Error("failed to delete SNAT chain", zap.Error(err))
	}

	m.managed = make(map[string]SNATRule)
	m.logger.Debug("cleaned up all SNAT rules")

	// Clean up FORWARD chain
	if err := m.ipt.ClearChain(filterTable, forwardChain); err != nil {
		m.logger.Error("failed to clear FORWARD chain", zap.Error(err))
	}

	forwardJumpRule := []string{"-j", forwardChain}
	if err := m.ipt.DeleteIfExists(filterTable, "FORWARD", forwardJumpRule...); err != nil {
		m.logger.Error("failed to delete jump rule from FORWARD", zap.Error(err))
	}

	if err := m.ipt.DeleteChain(filterTable, forwardChain); err != nil {
		m.logger.Error("failed to delete FORWARD chain", zap.Error(err))
	}

	m.managedForward = make(map[string]ForwardRule)
	m.logger.Debug("cleaned up all FORWARD rules")

	return nil
}

// buildRuleSpec constructs the iptables rule arguments for a given SNATRule.
func buildRuleSpec(rule SNATRule) []string {
	portStr := strconv.Itoa(int(rule.BackendPort))
	spec := []string{
		"-d", rule.BackendIP,
		"-p", rule.Protocol,
		"--dport", portStr,
	}
	if rule.SnatIP != "" {
		spec = append(spec, "-j", "SNAT", "--to-source", rule.SnatIP)
	} else {
		spec = append(spec, "-j", "MASQUERADE")
	}
	return spec
}

func (m *linuxManager) addRule(rule SNATRule) error {
	spec := buildRuleSpec(rule)
	return m.ipt.AppendUnique(natTable, snatChain, spec...)
}

func (m *linuxManager) deleteRule(rule SNATRule) error {
	spec := buildRuleSpec(rule)
	return m.ipt.DeleteIfExists(natTable, snatChain, spec...)
}

// buildForwardRuleSpec constructs the iptables rule arguments for a FORWARD accept rule.
func buildForwardRuleSpec(rule ForwardRule) []string {
	portStr := strconv.Itoa(int(rule.BackendPort))
	return []string{
		"-d", rule.BackendIP,
		"-p", rule.Protocol,
		"--dport", portStr,
		"-j", "ACCEPT",
	}
}

func (m *linuxManager) addForwardRule(rule ForwardRule) error {
	spec := buildForwardRuleSpec(rule)
	return m.ipt.AppendUnique(filterTable, forwardChain, spec...)
}

func (m *linuxManager) deleteForwardRule(rule ForwardRule) error {
	spec := buildForwardRuleSpec(rule)
	return m.ipt.DeleteIfExists(filterTable, forwardChain, spec...)
}

// Stats implements StatsProvider by parsing iptables -t nat -vnL EZLB-SNAT output.
// It returns cumulative packet/byte counts keyed by rule key (backendIP:port/protocol).
func (m *linuxManager) Stats() (map[string]SNATRuleStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats, err := m.ipt.Stats(natTable, snatChain)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats for chain %s: %w", snatChain, err)
	}

	result := make(map[string]SNATRuleStats)
	for _, stat := range stats {
		ruleKey, ruleStats, ok := parseSNATStatsRow(stat)
		if !ok {
			continue
		}
		result[ruleKey] = ruleStats
	}

	return result, nil
}
