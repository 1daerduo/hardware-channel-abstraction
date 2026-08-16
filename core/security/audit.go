package security

import (
	"example.com/embedded-loop-channel/core/event"
	"example.com/embedded-loop-channel/domain"
)

// AuditService appends tamper-evident-ish audit facts to the EventBus. Audit
// events record who/what/target/when and the policy decision (Design doc 10
// §24). They are distinct from ordinary runtime events.
type AuditService struct {
	bus *event.Bus
}

// NewAuditService builds an AuditService backed by bus.
func NewAuditService(bus *event.Bus) *AuditService {
	return &AuditService{bus: bus}
}

// Record emits an audit event.
func (a *AuditService) Record(principal string, action string, target domain.DeviceID, op domain.OperationID, decision Decision, details map[string]string) {
	e := domain.NewEvent("Audit."+action, "core.security", "audit")
	e.WithOperation(op).WithDevice(target)
	if details == nil {
		details = map[string]string{}
	}
	details["principal"] = principal
	details["decision"] = string(decision)
	e.Payload = details
	a.bus.Publish(e)
}

// RecordAccess emits an audit event for a policy decision on an operation.
func (a *AuditService) RecordAccess(principal string, cap domain.CapabilityName, target domain.DeviceID, op domain.OperationID, decision Decision) {
	a.Record(principal, "OperationAccess", target, op, decision, map[string]string{
		"capability": string(cap),
	})
}
