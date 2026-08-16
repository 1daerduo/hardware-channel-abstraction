package domain

import (
	"sync"
	"time"
)

// OperationState is the lifecycle of a single Operation (Design doc 06).
// UNKNOWN is a first-class terminal state: "sent but result unobservable" is
// not FAILED and not SUCCEEDED.
type OperationState string

const (
	OperationCreated         OperationState = "CREATED"
	OperationValidating      OperationState = "VALIDATING"
	OperationWaitingResource OperationState = "WAITING_RESOURCE"
	OperationResolving       OperationState = "RESOLVING"
	OperationRunning         OperationState = "RUNNING"
	OperationVerifying       OperationState = "VERIFYING"
	OperationSucceeded       OperationState = "SUCCEEDED"

	OperationValidationFailed OperationState = "VALIDATION_FAILED"
	OperationResourceFailed   OperationState = "RESOURCE_FAILED"
	OperationResolveFailed    OperationState = "RESOLVE_FAILED"
	OperationRunFailed        OperationState = "RUN_FAILED"
	OperationVerifyFailed     OperationState = "VERIFY_FAILED"
	OperationCancelled        OperationState = "CANCELLED"
	OperationTimedOut         OperationState = "TIMEOUT"
	OperationUnknown          OperationState = "UNKNOWN"
)

// terminalOperationStates are states from which an Operation never leaves.
var terminalOperationStates = map[OperationState]bool{
	OperationSucceeded:        true,
	OperationValidationFailed: true,
	OperationResourceFailed:   true,
	OperationResolveFailed:    true,
	OperationRunFailed:        true,
	OperationVerifyFailed:     true,
	OperationCancelled:        true,
	OperationTimedOut:         true,
	OperationUnknown:          true,
}

// IsTerminal reports whether s is a final state.
func (s OperationState) IsTerminal() bool { return terminalOperationStates[s] }

// IsFailure reports whether s is a terminal failure state.
func (s OperationState) IsFailure() bool {
	switch s {
	case OperationValidationFailed, OperationResourceFailed, OperationResolveFailed,
		OperationRunFailed, OperationVerifyFailed, OperationCancelled,
		OperationTimedOut, OperationUnknown:
		return true
	}
	return false
}

// OperationRequest is the unified request envelope (Design docs 03, 06).
// Upper layers express intent; Core resolves and executes it.
//
// ChannelType is an optional advanced override (Design doc 11 §15); empty
// means the Resolver chooses.
type OperationRequest struct {
	Capability     CapabilityName
	Target         DeviceID
	Parameters     map[string]string
	SessionID      SessionID
	IdempotencyKey string
	ChannelType    string
	Deadline       time.Time
}

// Operation is the persisted runtime state of one execution. State, Result and
// cancellation are guarded by mu so the same Operation can be started
// asynchronously, polled and cancelled from other goroutines.
type Operation struct {
	ID          OperationID
	Request     OperationRequest
	ChannelID   ChannelID
	Attempts    int
	StartedAt   time.Time
	CompletedAt time.Time

	mu              sync.Mutex
	state           OperationState
	result          *OperationResult
	cancelRequested bool
}

// NewOperation creates an Operation in the CREATED state.
func NewOperation(req OperationRequest) *Operation {
	return &Operation{
		ID:      NewOperationID(),
		Request: req,
		state:   OperationCreated,
	}
}

// Transition moves the operation to next after validating it is legal. The
// invariant is: terminal states are never overwritten by an ordinary retry.
func (o *Operation) Transition(next OperationState) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.state.IsTerminal() && next != o.state {
		return &OpTransitionError{From: o.state, To: next}
	}
	o.state = next
	if next.IsTerminal() {
		o.CompletedAt = time.Now()
	}
	return nil
}

// CurrentState returns the operation state (goroutine-safe).
func (o *Operation) CurrentState() OperationState {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.state
}

// SetResult records the terminal result (called once, before the terminal
// transition).
func (o *Operation) SetResult(r *OperationResult) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.result = r
}

// CurrentResult returns the terminal result, if set.
func (o *Operation) CurrentResult() *OperationResult {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.result
}

// RequestCancel flags the operation for cooperative cancellation.
func (o *Operation) RequestCancel() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.cancelRequested = true
}

// CancelRequested reports whether cancellation was requested.
func (o *Operation) CancelRequested() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.cancelRequested
}

// OperationResult is the stable final result. Progress and logs flow through
// Event/Stream, not through Result.
type OperationResult struct {
	OperationID  OperationID
	State        OperationState
	Output       string
	Error        *Error
	EvidenceRefs []EvidenceID
	ArtifactRefs []ArtifactID
	StartedAt    time.Time
	CompletedAt  time.Time
}

// OpTransitionError describes an illegal state transition.
type OpTransitionError struct{ From, To OperationState }

func (e *OpTransitionError) Error() string {
	return "illegal operation transition: " + string(e.From) + " -> " + string(e.To)
}
