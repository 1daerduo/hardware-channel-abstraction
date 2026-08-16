// Package operation implements the OperationEngine: the lifecycle that turns
// a unified OperationRequest into a verified result (Design docs 06, 13).
//
//	Policy → Capability validation → Resource acquire → Resolve → Plugin
//	Invoke → Verify → Result/Event/Evidence/Artifact → Release.
package operation

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/1daerduo/hardware-channel-abstraction/core/artifact"
	"github.com/1daerduo/hardware-channel-abstraction/core/event"
	"github.com/1daerduo/hardware-channel-abstraction/core/recovery"
	"github.com/1daerduo/hardware-channel-abstraction/core/registry"
	"github.com/1daerduo/hardware-channel-abstraction/core/resolver"
	"github.com/1daerduo/hardware-channel-abstraction/core/resource"
	"github.com/1daerduo/hardware-channel-abstraction/core/security"
	"github.com/1daerduo/hardware-channel-abstraction/core/session"
	"github.com/1daerduo/hardware-channel-abstraction/domain"
	pluginregistry "github.com/1daerduo/hardware-channel-abstraction/plugin/registry"
	"github.com/1daerduo/hardware-channel-abstraction/plugin/sdk"
)

// Engine executes Operations. It is the single orchestration point and the
// only layer that combines Policy, Resource, Resolver, Plugin and Recovery.
type Engine struct {
	reg        *registry.Registry
	resolver   *resolver.Resolver
	sessions   *session.Manager
	resources  *resource.Registry
	locks      *resource.Manager
	plugins    *pluginregistry.Registry
	bus        *event.Bus
	policy     *security.Policy
	approval   *security.ApprovalService
	audit      *security.AuditService
	recovery   *recovery.Manager
	reconciler *recovery.Reconciler
	classifier *recovery.Classifier
	artifacts  *artifact.Service

	maxRetries int
	lockTTL    time.Duration

	mu           sync.RWMutex
	operations   map[domain.OperationID]*domain.Operation
	idempotency  map[string]domain.OperationID
	evidence     map[domain.EvidenceID]*domain.Evidence
	artifactRefs map[domain.ArtifactID]*domain.Artifact
}

// Deps wires the Engine's collaborators.
type Deps struct {
	Registry   *registry.Registry
	Resolver   *resolver.Resolver
	Sessions   *session.Manager
	Resources  *resource.Registry
	Locks      *resource.Manager
	Plugins    *pluginregistry.Registry
	Bus        *event.Bus
	Policy     *security.Policy
	Approval   *security.ApprovalService
	Audit      *security.AuditService
	Recovery   *recovery.Manager
	Reconciler *recovery.Reconciler
	Classifier *recovery.Classifier
	Artifacts  *artifact.Service

	MaxRetries int
	LockTTL    time.Duration
}

// New builds an Engine. Zero-value optional fields get safe defaults.
func New(d Deps) *Engine {
	if d.MaxRetries <= 0 {
		d.MaxRetries = 1
	}
	if d.LockTTL <= 0 {
		d.LockTTL = 30 * time.Second
	}
	if d.Classifier == nil {
		d.Classifier = recovery.NewClassifier()
	}
	if d.Reconciler == nil {
		d.Reconciler = recovery.NewReconciler()
	}
	if d.Artifacts == nil {
		d.Artifacts = artifact.New()
	}
	return &Engine{
		reg:          d.Registry,
		resolver:     d.Resolver,
		sessions:     d.Sessions,
		resources:    d.Resources,
		locks:        d.Locks,
		plugins:      d.Plugins,
		bus:          d.Bus,
		policy:       d.Policy,
		approval:     d.Approval,
		audit:        d.Audit,
		recovery:     d.Recovery,
		reconciler:   d.Reconciler,
		classifier:   d.Classifier,
		artifacts:    d.Artifacts,
		maxRetries:   d.MaxRetries,
		lockTTL:      d.LockTTL,
		operations:   map[domain.OperationID]*domain.Operation{},
		idempotency:  map[string]domain.OperationID{},
		evidence:     map[domain.EvidenceID]*domain.Evidence{},
		artifactRefs: map[domain.ArtifactID]*domain.Artifact{},
	}
}

// Create registers an Operation in CREATED state, honoring idempotency keys.
// An existing operation with the same key is returned instead of duplicated.
func (e *Engine) Create(req domain.OperationRequest) (*domain.Operation, error) {
	if req.Capability == "" || req.Target == "" {
		return nil, domain.NewError(domain.CodeInvalidInput, domain.CategoryValidation,
			"capability and target are required")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if req.IdempotencyKey != "" {
		if id, ok := e.idempotency[req.IdempotencyKey]; ok {
			return e.operations[id], nil
		}
	}
	op := domain.NewOperation(req)
	e.operations[op.ID] = op
	if req.IdempotencyKey != "" {
		e.idempotency[req.IdempotencyKey] = op.ID
	}
	return op, nil
}

// Get returns an operation by ID.
func (e *Engine) Get(id domain.OperationID) (*domain.Operation, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	op, ok := e.operations[id]
	if !ok {
		return nil, domain.NewError(domain.CodeUnknown, domain.CategoryUnknown, "operation "+string(id)+" not found")
	}
	return op, nil
}

// Start creates an operation and begins executing it asynchronously, returning
// the operation immediately (Design doc 03 §9: long operations return an
// operation_id and the caller polls for completion).
func (e *Engine) Start(ctx context.Context, req domain.OperationRequest) (*domain.Operation, error) {
	op, err := e.Create(req)
	if err != nil {
		return nil, err
	}
	go func() {
		_, _ = e.Execute(ctx, op.ID)
	}()
	return op, nil
}

// Wait polls an operation until it reaches a terminal state or ctx is done,
// returning the final result.
func (e *Engine) Wait(ctx context.Context, id domain.OperationID, interval time.Duration) (*domain.OperationResult, error) {
	if interval <= 0 {
		interval = 20 * time.Millisecond
	}
	for {
		op, err := e.Get(id)
		if err != nil {
			return nil, err
		}
		if op.CurrentState().IsTerminal() {
			return op.CurrentResult(), nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

// Cancel requests cooperative cancellation of a running operation. It flags
// the operation and notifies the plugin (if it supports cancellation). The
// operation transitions to CANCELLED at the next lifecycle check.
func (e *Engine) Cancel(ctx context.Context, id domain.OperationID) error {
	op, err := e.Get(id)
	if err != nil {
		return err
	}
	if op.CurrentState().IsTerminal() {
		return domain.NewError("NOT_CANCELLABLE", domain.CategoryCancellation,
			"operation already terminal: "+string(op.CurrentState()))
	}
	op.RequestCancel()
	if ch, ok := e.reg.GetChannel(op.ChannelID); ok {
		if plugin, err := e.plugins.Get(ch.PluginID); err == nil {
			if canceller, ok := plugin.(sdk.Canceller); ok {
				_ = canceller.Cancel(ctx, ch, op.ID)
			}
		}
	}
	return nil
}

// Execute runs the operation lifecycle to a terminal state and returns the
// final result. It is safe to call once per operation.
func (e *Engine) Execute(ctx context.Context, id domain.OperationID) (*domain.OperationResult, error) {
	op, err := e.Get(id)
	if err != nil {
		return nil, err
	}

	if err := op.Transition(domain.OperationValidating); err != nil {
		return op.CurrentResult(), err
	}

	principal := e.principalFor(op)
	capDesc, capErr := e.capabilityDescriptor(op.Request.Capability)
	if capErr != nil {
		return e.fail(op, domain.OperationValidationFailed, capErr)
	}
	if _, ok := e.reg.GetDevice(op.Request.Target); !ok {
		return e.fail(op, domain.OperationValidationFailed,
			domain.NewError(domain.CodeInvalidInput, domain.CategoryValidation, "unknown target device"))
	}

	// Policy decision (deny-by-default, risk-gated approval).
	decision := e.policy.Evaluate(principal, op.Request.Capability, op.Request.Target, capDesc.RiskLevel)
	e.audit.RecordAccess(principal, op.Request.Capability, op.Request.Target, op.ID, decision)
	switch decision {
	case security.Deny:
		return e.fail(op, domain.OperationValidationFailed,
			domain.NewError(domain.CodePermissionDenied, domain.CategoryAuthorization,
				fmt.Sprintf("principal %q is not allowed %s", principal, op.Request.Capability)))
	case security.RequireApproval:
		if !e.approval.IsApproved(op.ID) {
			return e.fail(op, domain.OperationValidationFailed,
				domain.NewError(domain.CodePermissionDenied, domain.CategoryAuthorization,
					"approval required for "+string(op.Request.Capability)))
		}
	}

	// Acquire required resources (exclusive), in a global order to avoid
	// deadlock (Design doc 07 §10).
	resourceIDs := e.resolveResourceIDs(op.Request.Target, capDesc.ResourceRequirements)
	var leases []*domain.Lease
	if len(resourceIDs) > 0 {
		if err := op.Transition(domain.OperationWaitingResource); err != nil {
			return op.CurrentResult(), err
		}
		leases, err = e.locks.AcquireAll(ctx, string(op.ID), resourceIDs, domain.LockExclusive, e.lockTTL)
		if err != nil {
			return e.fail(op, domain.OperationResourceFailed, e.classifier.Classify(err))
		}
	}
	defer e.locks.ReleaseAll(leases)

	// Resolve a channel.
	if err := op.Transition(domain.OperationResolving); err != nil {
		return op.CurrentResult(), err
	}
	resolution, err := e.resolver.ResolveChannel(op.Request.Target, op.Request.Capability, op.Request.ChannelType)
	if err != nil {
		return e.fail(op, domain.OperationResolveFailed, e.classifier.Classify(err))
	}
	channel := resolution.Channel
	op.ChannelID = channel.ID
	e.bus.Publish(domain.NewEvent(domain.EventOperationStarted, "core.operation", "operation").
		WithOperation(op.ID).WithDevice(op.Request.Target).WithChannel(channel.ID))

	if err := op.Transition(domain.OperationRunning); err != nil {
		return op.CurrentResult(), err
	}

	out := e.runWithRecovery(ctx, op, channel, capDesc)
	if out.err != nil {
		return e.fail(op, out.final, out.err)
	}

	if err := op.Transition(domain.OperationVerifying); err != nil {
		return op.CurrentResult(), err
	}
	if verr := e.verify(op, capDesc, out.result); verr != nil {
		return e.fail(op, domain.OperationVerifyFailed, verr)
	}

	return e.succeed(op, channel, out.result), nil
}

// invokeOutcome carries the result of runWithRecovery: either a successful
// invoke result, or a terminal failure with the state to transition to.
type invokeOutcome struct {
	result *sdk.InvokeResult
	final  domain.OperationState // terminal failure state, if err != nil
	err    *domain.Error
}

// runWithRecovery invokes the plugin and, on failure, runs the Observe-first
// reconciliation then the recovery ladder (Design doc 09):
//
//	Invoke → L0 Observe → Reconcile (SUCCESS / retry / UNKNOWN / MANUAL)
//	      → L2 reconnect → L5 device recovery → L6 manual.
func (e *Engine) runWithRecovery(ctx context.Context, op *domain.Operation, channel *domain.Channel, capDesc *domain.Capability) invokeOutcome {
	req := sdk.InvokeRequest{
		Capability:  op.Request.Capability,
		Target:      op.Request.Target,
		Parameters:  op.Request.Parameters,
		SessionID:   op.Request.SessionID,
		OperationID: op.ID,
	}

	for attempt := 0; ; attempt++ {
		if op.CancelRequested() {
			return invokeOutcome{
				final: domain.OperationCancelled,
				err:   domain.NewError("CANCELLED", domain.CategoryCancellation, "operation cancelled"),
			}
		}
		plugin, err := e.plugins.Get(channel.PluginID)
		if err != nil {
			return invokeOutcome{final: domain.OperationRunFailed, err: domain.NewError(domain.CodeInternal, domain.CategoryInternal, err.Error())}
		}
		if channel.State != domain.ChannelReady {
			if err := plugin.Open(ctx, channel, op.Request.SessionID); err != nil {
				return invokeOutcome{final: domain.OperationRunFailed, err: e.classifier.Classify(err)}
			}
		}
		res, err := plugin.Invoke(ctx, channel, req)
		if err == nil {
			return invokeOutcome{result: res}
		}
		derr := e.classifier.Classify(err)

		// L0 Observe-first: reconcile against the device's actual state.
		recDecision := recovery.DecisionRetry // default: safe to retry
		if obs, oerr := e.recovery.Observe(ctx, channel, req); oerr == nil {
			rec := e.reconciler.Reconcile(op, obs)
			switch rec.Decision {
			case recovery.DecisionSuccess:
				// Postcondition provably satisfied → reconcile to SUCCESS.
				result := &sdk.InvokeResult{Output: rec.Reason}
				for k, v := range obs.Facts {
					ev := domain.NewEvidence(k, v)
					ev.OperationID = op.ID
					result.Evidence = append(result.Evidence, *ev)
				}
				return invokeOutcome{result: result}
			case recovery.DecisionManual:
				// High-risk postcondition mismatch: never auto-replay.
				return invokeOutcome{
					final: domain.OperationUnknown,
					err:   domain.NewError(domain.CodeUnknown, domain.CategoryUnknown, rec.Reason).WithDetail("manual_required", "true"),
				}
			}
			recDecision = rec.Decision // RETRY or UNKNOWN → run the recovery ladder
		}

		// Recovery ladder (L2 reconnect → L3 re-resolve → L4 rediscover →
		// L5 device recovery → L6 manual).
		if attempt >= e.maxRetries {
			return invokeOutcome{final: e.finalState(recDecision, derr), err: derr}
		}
		rr := e.recovery.Recover(ctx, channel, op.Request.Capability, derr)
		if rr.Recovered {
			if rr.Channel != nil && rr.Channel.ID != channel.ID {
				// L3/L4 selected a different channel; rebind the operation.
				channel = rr.Channel
				op.ChannelID = channel.ID
			}
			continue
		}
		return invokeOutcome{final: e.finalState(recDecision, derr), err: derr}
	}
}

// finalState chooses the terminal operation state when recovery is exhausted:
// an UNKNOWN observation maps to OperationUnknown, everything else to
// OperationRunFailed.
func (e *Engine) finalState(decision string, derr *domain.Error) domain.OperationState {
	if decision == recovery.DecisionUnknown {
		return domain.OperationUnknown
	}
	return domain.OperationRunFailed
}

// verify applies postcondition checks (Design doc 06 §5). For flash it
// verifies the reported partition version matches the request.
func (e *Engine) verify(op *domain.Operation, capDesc *domain.Capability, res *sdk.InvokeResult) *domain.Error {
	if op.Request.Capability == domain.CapabilityFlash {
		want := op.Request.Parameters["version"]
		for _, ev := range res.Evidence {
			if ev.Name == "flash.version" && ev.Value != want {
				return domain.NewError(domain.CodeVerificationFailed, domain.CategoryVerification,
					fmt.Sprintf("flash version mismatch: want %s got %s", want, ev.Value))
			}
		}
	}
	return nil
}

// succeed records evidence/artifacts, emits events and builds the final result.
// Artifacts are ingested through the Artifact Service, which computes their
// checksum and marks them AVAILABLE before they are referenced.
func (e *Engine) succeed(op *domain.Operation, channel *domain.Channel, res *sdk.InvokeResult) *domain.OperationResult {
	e.mu.Lock()
	for i := range res.Evidence {
		res.Evidence[i].OperationID = op.ID
		e.evidence[res.Evidence[i].ID] = &res.Evidence[i]
	}
	e.mu.Unlock()

	result := &domain.OperationResult{
		OperationID: op.ID,
		State:       domain.OperationSucceeded,
		Output:      res.Output,
		StartedAt:   op.StartedAt,
		CompletedAt: time.Now(),
	}
	for _, ev := range res.Evidence {
		result.EvidenceRefs = append(result.EvidenceRefs, ev.ID)
	}
	for i := range res.Artifacts {
		src := res.Artifacts[i]
		src.OperationID = op.ID
		final, err := e.artifacts.Ingest(&src)
		if err != nil {
			continue
		}
		e.mu.Lock()
		e.artifactRefs[final.ID] = final
		e.mu.Unlock()
		result.ArtifactRefs = append(result.ArtifactRefs, final.ID)
	}

	op.SetResult(result)
	_ = op.Transition(domain.OperationSucceeded)

	e.bus.Publish(domain.NewEvent(domain.EventOperationSucceeded, "core.operation", "operation").
		WithOperation(op.ID).WithDevice(op.Request.Target).WithChannel(channel.ID))
	return result
}

// fail transitions the operation to a terminal failure state and records the
// error, emitting an OperationFailed event.
func (e *Engine) fail(op *domain.Operation, state domain.OperationState, err *domain.Error) (*domain.OperationResult, error) {
	if err == nil {
		err = domain.NewError(domain.CodeInternal, domain.CategoryInternal, "operation failed")
	}
	result := &domain.OperationResult{
		OperationID: op.ID,
		State:       state,
		Error:       err,
		StartedAt:   op.StartedAt,
		CompletedAt: time.Now(),
	}
	op.SetResult(result)
	_ = op.Transition(state)

	e.bus.Publish(domain.NewEvent(domain.EventOperationFailed, "core.operation", "operation").
		WithOperation(op.ID).WithDevice(op.Request.Target))
	return result, err
}

// GetEvidence returns recorded evidence for an operation result.
func (e *Engine) GetEvidence(id domain.EvidenceID) (*domain.Evidence, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	ev, ok := e.evidence[id]
	return ev, ok
}

// GetArtifact returns a recorded artifact (metadata + content resolved from
// the Artifact Service).
func (e *Engine) GetArtifact(id domain.ArtifactID) (*domain.Artifact, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	a, ok := e.artifactRefs[id]
	if ok && a != nil {
		if svc, ok2 := e.artifacts.Get(id); ok2 {
			a = svc
		}
	}
	return a, ok
}

func (e *Engine) principalFor(op *domain.Operation) string {
	if op.Request.SessionID == "" {
		return "anonymous"
	}
	if s, err := e.sessions.Get(op.Request.SessionID); err == nil {
		return s.Principal
	}
	return "anonymous"
}

// capabilityDescriptor returns the capability descriptor from the first
// plugin that advertises it.
func (e *Engine) capabilityDescriptor(name domain.CapabilityName) (*domain.Capability, *domain.Error) {
	plugins := e.plugins.FindByCapability(name)
	if len(plugins) == 0 {
		return nil, domain.NewError(domain.CodeUnsupportedCap, domain.CategoryValidation,
			"no plugin advertises capability "+string(name))
	}
	for _, cap := range plugins[0].Capabilities(nil) {
		if cap.Name == name {
			cap := cap
			return &cap, nil
		}
	}
	return nil, domain.NewError(domain.CodeUnsupportedCap, domain.CategoryValidation,
		"capability "+string(name)+" has no descriptor")
}

// resolveResourceIDs maps resource types to concrete resource IDs, in a
// stable global order to prevent deadlock (Device → Subsystem → ...).
func (e *Engine) resolveResourceIDs(deviceID domain.DeviceID, types []string) []domain.ResourceID {
	ordered := append([]string(nil), types...)
	sort.Strings(ordered)
	var ids []domain.ResourceID
	for _, t := range ordered {
		res := e.resources.Ensure(deviceID, t)
		ids = append(ids, res.ID)
	}
	return ids
}
