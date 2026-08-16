package recovery

import (
	"example.com/embedded-loop-channel/domain"
	"example.com/embedded-loop-channel/plugin/sdk"
)

// ReconciliationResult decides what to do with an interrupted operation after
// an Observe-first check (Design doc 09 §7, §29, §31).
type ReconciliationResult struct {
	// State is the reconciled final operation state (SUCCEEDED / retry target
	// / UNKNOWN). A zero State means "retry" (no terminal reconciliation).
	State    domain.OperationState
	Decision string // SUCCESS / RETRY / UNKNOWN / MANUAL
	Reason   string
}

const (
	DecisionSuccess = "SUCCESS"
	DecisionRetry   = "RETRY"
	DecisionUnknown = "UNKNOWN"
	DecisionManual  = "MANUAL"
)

// Reconciler reconciles an interrupted operation against a device observation.
// The rule (Design doc 09 §29): if the postcondition is provably satisfied,
// reconcile to SUCCESS; if the device is online and a retry is safe, RETRY;
// otherwise UNKNOWN/MANUAL — never fabricate a success or a failure.
type Reconciler struct{}

// NewReconciler builds a Reconciler.
func NewReconciler() *Reconciler { return &Reconciler{} }

// Reconcile returns the decision for op given obs.
func (r *Reconciler) Reconcile(op *domain.Operation, obs *sdk.Observation) ReconciliationResult {
	if op.Request.Capability == domain.CapabilityFlash {
		want := op.Request.Parameters["version"]
		got := obs.Facts["flash.version"]
		if want != "" && got == want {
			return ReconciliationResult{
				State:    domain.OperationSucceeded,
				Decision: DecisionSuccess,
				Reason:   "postcondition satisfied: flash.version=" + got,
			}
		}
		// Flash is high-risk and non-idempotent: an unsatisfied postcondition
		// must NOT be blindly replayed.
		if obs.Online {
			return ReconciliationResult{
				Decision: DecisionManual,
				Reason:   "flash version mismatch (want " + want + " got " + got + "), manual required",
			}
		}
		return ReconciliationResult{
			State:    domain.OperationUnknown,
			Decision: DecisionUnknown,
			Reason:   "device unreachable, flash result unknown",
		}
	}

	// Generic observe-first: if the device is reachable again, a retry is the
	// natural next step; otherwise the outcome stays UNKNOWN.
	if obs.Online {
		return ReconciliationResult{Decision: DecisionRetry, Reason: "device reachable, safe to retry"}
	}
	return ReconciliationResult{
		State:    domain.OperationUnknown,
		Decision: DecisionUnknown,
		Reason:   "device unreachable, result unknown",
	}
}
