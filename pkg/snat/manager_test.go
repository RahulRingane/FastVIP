//go:build !integration

package snat

import (
	"testing"

	"go.uber.org/zap"
)

func TestSNATRuleKey(t *testing.T) {
	rule := SNATRule{
		BackendIP:   "192.168.1.1",
		BackendPort: 8080,
		Protocol:    "tcp",
		SnatIP:      "10.0.0.1",
	}
	expected := "192.168.1.1:8080/tcp"
	if rule.Key() != expected {
		t.Errorf("expected key %q, got %q", expected, rule.Key())
	}
}

func TestFakeManager_ReconcileAddRules(t *testing.T) {
	mgr, err := NewManager(zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	desired := []SNATRule{
		{BackendIP: "192.168.1.1", BackendPort: 8080, Protocol: "tcp", SnatIP: "10.0.0.1"},
		{BackendIP: "192.168.1.2", BackendPort: 8080, Protocol: "tcp", SnatIP: ""},
	}

	if err := mgr.Reconcile(desired); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	fakeMgr := mgr.(*FakeManager)
	managed := fakeMgr.GetManaged()
	if len(managed) != 2 {
		t.Fatalf("expected 2 managed rules, got %d", len(managed))
	}

	rule1, exists := managed["192.168.1.1:8080/tcp"]
	if !exists {
		t.Fatal("expected rule 192.168.1.1:8080/tcp to exist")
	}
	if rule1.SnatIP != "10.0.0.1" {
		t.Errorf("expected snat_ip '10.0.0.1', got %q", rule1.SnatIP)
	}

	rule2, exists := managed["192.168.1.2:8080/tcp"]
	if !exists {
		t.Fatal("expected rule 192.168.1.2:8080/tcp to exist")
	}
	if rule2.SnatIP != "" {
		t.Errorf("expected empty snat_ip (MASQUERADE), got %q", rule2.SnatIP)
	}
}

func TestFakeManager_ReconcileRemoveStaleRules(t *testing.T) {
	mgr, err := NewManager(zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// First reconcile: add 2 rules
	initial := []SNATRule{
		{BackendIP: "192.168.1.1", BackendPort: 8080, Protocol: "tcp", SnatIP: "10.0.0.1"},
		{BackendIP: "192.168.1.2", BackendPort: 8080, Protocol: "tcp", SnatIP: "10.0.0.1"},
	}
	if err := mgr.Reconcile(initial); err != nil {
		t.Fatalf("first Reconcile failed: %v", err)
	}

	// Second reconcile: only 1 rule desired
	desired := []SNATRule{
		{BackendIP: "192.168.1.1", BackendPort: 8080, Protocol: "tcp", SnatIP: "10.0.0.1"},
	}
	if err := mgr.Reconcile(desired); err != nil {
		t.Fatalf("second Reconcile failed: %v", err)
	}

	fakeMgr := mgr.(*FakeManager)
	managed := fakeMgr.GetManaged()
	if len(managed) != 1 {
		t.Fatalf("expected 1 managed rule after removal, got %d", len(managed))
	}
	if _, exists := managed["192.168.1.2:8080/tcp"]; exists {
		t.Error("expected rule 192.168.1.2:8080/tcp to be removed")
	}
}

func TestFakeManager_ReconcileUpdateSnatIP(t *testing.T) {
	mgr, err := NewManager(zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// First reconcile with SNAT IP
	initial := []SNATRule{
		{BackendIP: "192.168.1.1", BackendPort: 8080, Protocol: "tcp", SnatIP: "10.0.0.1"},
	}
	if err := mgr.Reconcile(initial); err != nil {
		t.Fatalf("first Reconcile failed: %v", err)
	}

	// Second reconcile: change to MASQUERADE
	updated := []SNATRule{
		{BackendIP: "192.168.1.1", BackendPort: 8080, Protocol: "tcp", SnatIP: ""},
	}
	if err := mgr.Reconcile(updated); err != nil {
		t.Fatalf("second Reconcile failed: %v", err)
	}

	fakeMgr := mgr.(*FakeManager)
	managed := fakeMgr.GetManaged()
	rule := managed["192.168.1.1:8080/tcp"]
	if rule.SnatIP != "" {
		t.Errorf("expected empty snat_ip after update, got %q", rule.SnatIP)
	}
}

func TestFakeManager_Cleanup(t *testing.T) {
	mgr, err := NewManager(zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	desired := []SNATRule{
		{BackendIP: "192.168.1.1", BackendPort: 8080, Protocol: "tcp", SnatIP: "10.0.0.1"},
	}
	if err := mgr.Reconcile(desired); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	if err := mgr.Cleanup(); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	fakeMgr := mgr.(*FakeManager)
	managed := fakeMgr.GetManaged()
	if len(managed) != 0 {
		t.Fatalf("expected 0 managed rules after cleanup, got %d", len(managed))
	}
}

func TestFakeManager_ReconcileEmptyDesired(t *testing.T) {
	mgr, err := NewManager(zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Add some rules first
	initial := []SNATRule{
		{BackendIP: "192.168.1.1", BackendPort: 8080, Protocol: "tcp", SnatIP: "10.0.0.1"},
	}
	if err := mgr.Reconcile(initial); err != nil {
		t.Fatalf("first Reconcile failed: %v", err)
	}

	// Reconcile with empty desired: all rules should be removed
	if err := mgr.Reconcile(nil); err != nil {
		t.Fatalf("second Reconcile with nil failed: %v", err)
	}

	fakeMgr := mgr.(*FakeManager)
	managed := fakeMgr.GetManaged()
	if len(managed) != 0 {
		t.Fatalf("expected 0 managed rules after empty reconcile, got %d", len(managed))
	}
}

func TestForwardRuleKey(t *testing.T) {
	rule := ForwardRule{
		BackendIP:   "192.168.1.1",
		BackendPort: 8080,
		Protocol:    "tcp",
	}
	expected := "192.168.1.1:8080/tcp"
	if rule.Key() != expected {
		t.Errorf("expected key %q, got %q", expected, rule.Key())
	}
}

func TestFakeManager_ReconcileForwardAddRules(t *testing.T) {
	mgr, err := NewManager(zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	desired := []ForwardRule{
		{BackendIP: "192.168.1.1", BackendPort: 8080, Protocol: "tcp"},
		{BackendIP: "192.168.1.2", BackendPort: 8080, Protocol: "tcp"},
	}

	if err := mgr.ReconcileForward(desired); err != nil {
		t.Fatalf("ReconcileForward failed: %v", err)
	}

	fakeMgr := mgr.(*FakeManager)
	managed := fakeMgr.GetManagedForward()
	if len(managed) != 2 {
		t.Fatalf("expected 2 managed FORWARD rules, got %d", len(managed))
	}

	if _, exists := managed["192.168.1.1:8080/tcp"]; !exists {
		t.Fatal("expected FORWARD rule 192.168.1.1:8080/tcp to exist")
	}
	if _, exists := managed["192.168.1.2:8080/tcp"]; !exists {
		t.Fatal("expected FORWARD rule 192.168.1.2:8080/tcp to exist")
	}
}

func TestFakeManager_ReconcileForwardRemoveStaleRules(t *testing.T) {
	mgr, err := NewManager(zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// First reconcile: add 2 rules
	initial := []ForwardRule{
		{BackendIP: "192.168.1.1", BackendPort: 8080, Protocol: "tcp"},
		{BackendIP: "192.168.1.2", BackendPort: 8080, Protocol: "tcp"},
	}
	if err := mgr.ReconcileForward(initial); err != nil {
		t.Fatalf("first ReconcileForward failed: %v", err)
	}

	// Second reconcile: only 1 rule desired
	desired := []ForwardRule{
		{BackendIP: "192.168.1.1", BackendPort: 8080, Protocol: "tcp"},
	}
	if err := mgr.ReconcileForward(desired); err != nil {
		t.Fatalf("second ReconcileForward failed: %v", err)
	}

	fakeMgr := mgr.(*FakeManager)
	managed := fakeMgr.GetManagedForward()
	if len(managed) != 1 {
		t.Fatalf("expected 1 managed FORWARD rule after removal, got %d", len(managed))
	}
	if _, exists := managed["192.168.1.2:8080/tcp"]; exists {
		t.Error("expected FORWARD rule 192.168.1.2:8080/tcp to be removed")
	}
}

func TestFakeManager_CleanupIncludesForwardRules(t *testing.T) {
	mgr, err := NewManager(zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Add SNAT and FORWARD rules
	snatRules := []SNATRule{
		{BackendIP: "192.168.1.1", BackendPort: 8080, Protocol: "tcp", SnatIP: "10.0.0.1"},
	}
	if err := mgr.Reconcile(snatRules); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	forwardRules := []ForwardRule{
		{BackendIP: "192.168.1.1", BackendPort: 8080, Protocol: "tcp"},
	}
	if err := mgr.ReconcileForward(forwardRules); err != nil {
		t.Fatalf("ReconcileForward failed: %v", err)
	}

	if err := mgr.Cleanup(); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	fakeMgr := mgr.(*FakeManager)
	if len(fakeMgr.GetManaged()) != 0 {
		t.Fatalf("expected 0 SNAT rules after cleanup, got %d", len(fakeMgr.GetManaged()))
	}
	if len(fakeMgr.GetManagedForward()) != 0 {
		t.Fatalf("expected 0 FORWARD rules after cleanup, got %d", len(fakeMgr.GetManagedForward()))
	}
}
