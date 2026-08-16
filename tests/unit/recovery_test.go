package unit

import (
	"testing"

	"example.com/embedded-loop-channel/core/recovery"
	"example.com/embedded-loop-channel/domain"
	"example.com/embedded-loop-channel/plugin/sdk"
)

func opFor(cap domain.CapabilityName, params map[string]string) *domain.Operation {
	return domain.NewOperation(domain.OperationRequest{
		Capability: cap,
		Target:     domain.DeviceID("dev-1"),
		Parameters: params,
	})
}

func TestReconcilerFlashPostconditionSatisfied(t *testing.T) {
	r := recovery.NewReconciler()
	op := opFor(domain.CapabilityFlash, map[string]string{"partition": "boot", "version": "2.0.0"})
	obs := &sdk.Observation{Online: true, Facts: map[string]string{"flash.version": "2.0.0"}}

	rec := r.Reconcile(op, obs)
	if rec.Decision != recovery.DecisionSuccess || rec.State != domain.OperationSucceeded {
		t.Fatalf("expected SUCCESS reconcile, got %+v", rec)
	}
}

func TestReconcilerFlashMismatchRequiresManual(t *testing.T) {
	r := recovery.NewReconciler()
	op := opFor(domain.CapabilityFlash, map[string]string{"partition": "boot", "version": "2.0.0"})
	obs := &sdk.Observation{Online: true, Facts: map[string]string{"flash.version": "1.9.9"}}

	rec := r.Reconcile(op, obs)
	// High-risk non-idempotent flash must NOT be blindly replayed.
	if rec.Decision != recovery.DecisionManual {
		t.Fatalf("expected MANUAL reconcile, got %+v", rec)
	}
}

func TestReconcilerOfflineIsUnknown(t *testing.T) {
	r := recovery.NewReconciler()
	op := opFor(domain.CapabilityReboot, nil)
	obs := &sdk.Observation{Online: false, Facts: map[string]string{}}

	rec := r.Reconcile(op, obs)
	if rec.Decision != recovery.DecisionUnknown || rec.State != domain.OperationUnknown {
		t.Fatalf("expected UNKNOWN reconcile, got %+v", rec)
	}
}

func TestReconcilerOnlineRetry(t *testing.T) {
	r := recovery.NewReconciler()
	op := opFor(domain.CapabilityReboot, nil)
	obs := &sdk.Observation{Online: true, Facts: map[string]string{"boot.state": "system"}}

	rec := r.Reconcile(op, obs)
	if rec.Decision != recovery.DecisionRetry {
		t.Fatalf("expected RETRY reconcile, got %+v", rec)
	}
}
