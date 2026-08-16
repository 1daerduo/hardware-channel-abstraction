package unit

import (
	"testing"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
)

func TestOperationStateMachineTerminalIsFinal(t *testing.T) {
	op := domain.NewOperation(domain.OperationRequest{Capability: domain.CapabilityReboot, Target: "dev-1"})

	if err := op.Transition(domain.OperationRunning); err != nil {
		t.Fatalf("transition to RUNNING failed: %v", err)
	}
	if err := op.Transition(domain.OperationSucceeded); err != nil {
		t.Fatalf("transition to SUCCEEDED failed: %v", err)
	}
	// A terminal state must never be overwritten by an ordinary retry.
	if err := op.Transition(domain.OperationRunning); err == nil {
		t.Fatalf("terminal SUCCEEDED must not transition back to RUNNING")
	}
	if op.CurrentState() != domain.OperationSucceeded {
		t.Fatalf("state changed illegally to %s", op.CurrentState())
	}
}

func TestOperationUNKNOWNIsTerminalAndFailure(t *testing.T) {
	if !domain.OperationUnknown.IsTerminal() {
		t.Fatalf("UNKNOWN must be terminal")
	}
	if !domain.OperationUnknown.IsFailure() {
		t.Fatalf("UNKNOWN must be a failure state (not success)")
	}
}

func TestCapabilityRiskAndIdempotency(t *testing.T) {
	c := domain.Capability{Name: domain.CapabilityFlash, RiskLevel: domain.RiskHigh, Idempotent: false}
	if c.RiskLevel != domain.RiskHigh {
		t.Fatalf("risk level mismatch")
	}
	if c.Idempotent {
		t.Fatalf("flash must not be idempotent")
	}
}

func TestEventCorrelation(t *testing.T) {
	e := domain.NewEvent(domain.EventOperationStarted, "test", "operation").
		WithOperation("op-1").WithDevice("dev-1").WithChannel("ch-1")
	if e.OperationID != "op-1" || e.DeviceID != "dev-1" || e.ChannelID != "ch-1" {
		t.Fatalf("correlation fields not set: %+v", e)
	}
}
