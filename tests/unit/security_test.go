package unit

import (
	"context"
	"testing"
	"time"

	"example.com/embedded-loop-channel/core/security"
	"example.com/embedded-loop-channel/domain"
	"example.com/embedded-loop-channel/fake"
	"example.com/embedded-loop-channel/runtime"
)

func TestAuthenticator(t *testing.T) {
	a := security.NewAuthenticator()
	if err := a.RegisterToken("tok-1", security.Identity{Principal: "agent-1", Type: "agent"}); err != nil {
		t.Fatalf("register: %v", err)
	}

	id, err := a.Authenticate(context.Background(), "tok-1")
	if err != nil || id.Principal != "agent-1" || id.Type != "agent" {
		t.Fatalf("authenticate: id=%+v err=%v", id, err)
	}
	if _, err := a.Authenticate(context.Background(), "bad-token"); err == nil {
		t.Fatalf("expected invalid-token error")
	}

	a.RevokeToken("tok-1")
	if _, err := a.Authenticate(context.Background(), "tok-1"); err == nil {
		t.Fatalf("expected revoked-token error")
	}
}

func TestResourceScope(t *testing.T) {
	p := security.NewPolicy()
	p.GrantScope("agent", domain.CapabilityFlash, "dev-1")

	// In-scope HIGH gates on approval.
	if got := p.Evaluate("agent", domain.CapabilityFlash, "dev-1", domain.RiskHigh); got != security.RequireApproval {
		t.Fatalf("in-scope HIGH should REQUIRE_APPROVAL, got %s", got)
	}
	p.PreApprove("agent", domain.CapabilityFlash)
	if got := p.Evaluate("agent", domain.CapabilityFlash, "dev-1", domain.RiskHigh); got != security.Allow {
		t.Fatalf("in-scope approved should ALLOW, got %s", got)
	}
	// Out of scope is denied even with approval.
	if got := p.Evaluate("agent", domain.CapabilityFlash, "dev-2", domain.RiskHigh); got != security.Deny {
		t.Fatalf("out-of-scope should DENY, got %s", got)
	}
}

func TestResourceScopeWildcard(t *testing.T) {
	p := security.NewPolicy()
	p.Grant("agent", domain.CapabilityReboot) // "*" scope
	p.PreApprove("agent", domain.CapabilityReboot)
	if got := p.Evaluate("agent", domain.CapabilityReboot, "dev-anything", domain.RiskMedium); got != security.Allow {
		t.Fatalf("wildcard grant should ALLOW any device, got %s", got)
	}
}

func TestSecretRedaction(t *testing.T) {
	s := security.NewSecretStore()
	s.Set("device-pass", "sup3rsecret")

	v, err := s.Resolve(context.Background(), "device-pass")
	if err != nil || v != "sup3rsecret" {
		t.Fatalf("resolve: v=%q err=%v", v, err)
	}
	if _, err := s.Resolve(context.Background(), "missing"); err == nil {
		t.Fatalf("expected missing-secret error")
	}

	out := s.Redact("login with sup3rsecret now")
	if out != "login with *** now" {
		t.Fatalf("redact: got %q", out)
	}
}

// TestResourceScopeEndToEnd verifies resource scope is enforced through the
// full operation engine: a capability granted on device A is DENIED on
// device B.
func TestResourceScopeEndToEnd(t *testing.T) {
	ctx := context.Background()
	rt := runtime.Bootstrap(runtime.WithDevices(
		fake.NewDevice("DEV-1", "board", "1.0", "usb:1-1.1"),
		fake.NewDevice("DEV-2", "board", "1.0", "usb:1-1.2"),
	))
	c := rt.Client

	devices, _ := c.Discover(ctx)
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	// Scope info.get to devices[0] only.
	c.GrantScope("agent", domain.CapabilityInfoGet, devices[0].ID)

	sess0, _ := c.CreateSession("agent", devices[0].ID, time.Minute)
	res, err := c.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityInfoGet, Target: devices[0].ID, SessionID: sess0.ID,
	})
	if err != nil || res.State != domain.OperationSucceeded {
		t.Fatalf("in-scope execute should succeed: state=%v err=%v", res.State, err)
	}

	sess1, _ := c.CreateSession("agent", devices[1].ID, time.Minute)
	res2, err := c.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityInfoGet, Target: devices[1].ID, SessionID: sess1.ID,
	})
	if err == nil || res2.Error == nil || res2.Error.Category != domain.CategoryAuthorization {
		t.Fatalf("out-of-scope execute should DENY: err=%v", res2.Error)
	}
}
