// Package security provides the minimal MVP security model: Principal,
// deny-by-default Capability Policy with risk-gated approval, and Audit
// (Design doc 10). It is deliberately simple; the full RBAC/tenant model is a
// Beta/Production concern.
package security

import (
	"sync"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
)

// Decision is the outcome of a policy evaluation.
type Decision string

const (
	Allow           Decision = "ALLOW"
	Deny            Decision = "DENY"
	RequireApproval Decision = "REQUIRE_APPROVAL"
)

// Principal is the acting subject (User, Service, Agent, CLI, Loop...).
type Principal struct {
	Name string
}

// Policy grants capabilities to principals, optionally scoped to specific
// devices. It is deny-by-default: a capability is DENIED unless explicitly
// granted for the capability AND the target device (Design doc 10 §28, §7).
type Policy struct {
	mu sync.RWMutex
	// grants maps principal → capability → allowed device IDs. The literal
	// "*" means "all devices" (unscoped grant).
	grants map[string]map[domain.CapabilityName]map[domain.DeviceID]bool
	// approved records pre-approved (principal, capability) pairs for
	// HIGH/CRITICAL actions, modelling an approval workflow shortcut.
	approved map[string]map[domain.CapabilityName]bool
}

// NewPolicy builds an empty, deny-by-default Policy.
func NewPolicy() *Policy {
	return &Policy{
		grants:   map[string]map[domain.CapabilityName]map[domain.DeviceID]bool{},
		approved: map[string]map[domain.CapabilityName]bool{},
	}
}

// Grant allows principal to use capability across ALL devices.
func (p *Policy) Grant(principal string, cap domain.CapabilityName) {
	p.GrantScope(principal, cap, "*")
}

// GrantScope allows principal to use capability on a SPECIFIC device (resource
// scope). Use "*" to grant all devices.
func (p *Policy) GrantScope(principal string, cap domain.CapabilityName, device domain.DeviceID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.grants[principal] == nil {
		p.grants[principal] = map[domain.CapabilityName]map[domain.DeviceID]bool{}
	}
	if p.grants[principal][cap] == nil {
		p.grants[principal][cap] = map[domain.DeviceID]bool{}
	}
	p.grants[principal][cap][device] = true
}

// PreApprove allows principal to bypass the approval gate for a HIGH/CRITICAL
// capability.
func (p *Policy) PreApprove(principal string, cap domain.CapabilityName) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.approved[principal] == nil {
		p.approved[principal] = map[domain.CapabilityName]bool{}
	}
	p.approved[principal][cap] = true
}

// Evaluate returns the decision for principal acting on capability targeting
// device with the given risk. Deny-by-default; resource scope; HIGH/CRITICAL
// gate on approval.
func (p *Policy) Evaluate(principal string, cap domain.CapabilityName, target domain.DeviceID, risk domain.RiskLevel) Decision {
	p.mu.RLock()
	defer p.mu.RUnlock()
	devices := p.grants[principal][cap]
	if devices == nil {
		return Deny
	}
	if !devices["*"] && !devices[target] {
		return Deny // out of resource scope
	}
	if (risk == domain.RiskHigh || risk == domain.RiskCritical) && !p.approved[principal][cap] {
		return RequireApproval
	}
	return Allow
}

// Approval records an approval for an operation (binds the decision to the
// operation fingerprint; a real implementation would hash the request).
type Approval struct {
	OperationID domain.OperationID
	Principal   string
	Capability  domain.CapabilityName
}

// ApprovalService tracks one-shot operation approvals.
type ApprovalService struct {
	mu        sync.RWMutex
	approvals map[domain.OperationID]bool
}

// NewApprovalService builds an empty ApprovalService.
func NewApprovalService() *ApprovalService {
	return &ApprovalService{approvals: map[domain.OperationID]bool{}}
}

// Approve records approval for an operation.
func (a *ApprovalService) Approve(op domain.OperationID) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.approvals[op] = true
}

// IsApproved reports whether the operation was approved.
func (a *ApprovalService) IsApproved(op domain.OperationID) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.approvals[op]
}
