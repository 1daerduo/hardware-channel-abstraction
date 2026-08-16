package unit

import (
	"testing"
	"time"

	"example.com/embedded-loop-channel/core/resource"
	"example.com/embedded-loop-channel/core/security"
	"example.com/embedded-loop-channel/domain"
)

// Mutation tests (Design doc 17 §14) actively attack the invariants and assert
// the system rejects them — so a future refactor that silently weakens a
// contract is caught.

// TestMutation_TerminalStateImmutable: a terminal state can never be
// overwritten by an ordinary retry (skip-verification attack).
func TestMutation_TerminalStateImmutable(t *testing.T) {
	op := domain.NewOperation(domain.OperationRequest{Capability: domain.CapabilityReboot, Target: "dev-1"})
	_ = op.Transition(domain.OperationSucceeded)
	if err := op.Transition(domain.OperationRunning); err == nil {
		t.Fatalf("mutation: terminal SUCCEEDED was overwritten")
	}
}

// TestMutation_DoubleLeaseRelease: releasing an already-released lease must
// fail (double-free attack).
func TestMutation_DoubleLeaseRelease(t *testing.T) {
	m := resource.NewManager()
	l, ok := m.TryAcquire("a", "res-1", domain.LockExclusive, time.Minute)
	if !ok {
		t.Fatalf("acquire failed")
	}
	if err := m.Release(l.ID); err != nil {
		t.Fatalf("first release: %v", err)
	}
	if err := m.Release(l.ID); err == nil {
		t.Fatalf("mutation: double release should fail")
	}
}

// TestMutation_DenyByDefault: an un-granted capability must be DENIED
// (privilege-escalation attack).
func TestMutation_DenyByDefault(t *testing.T) {
	p := security.NewPolicy()
	if got := p.Evaluate("nobody", domain.CapabilityFlash, "dev-1", domain.RiskHigh); got != security.Deny {
		t.Fatalf("mutation: missing grant evaluated to %s, want DENY", got)
	}
}

// TestMutation_ExclusiveLockConflict: two exclusive leases must never coexist
// (lock-bypass attack).
func TestMutation_ExclusiveLockConflict(t *testing.T) {
	m := resource.NewManager()
	if _, ok := m.TryAcquire("a", "res-1", domain.LockExclusive, time.Minute); !ok {
		t.Fatalf("first acquire failed")
	}
	if _, ok := m.TryAcquire("b", "res-1", domain.LockExclusive, time.Minute); ok {
		t.Fatalf("mutation: two exclusive leases coexisted")
	}
}
