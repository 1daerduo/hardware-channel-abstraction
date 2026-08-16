package recovery

import (
	"context"
	"testing"
	"time"

	"example.com/embedded-loop-channel/core/event"
	"example.com/embedded-loop-channel/core/registry"
	"example.com/embedded-loop-channel/core/resolver"
	"example.com/embedded-loop-channel/domain"
	pluginregistry "example.com/embedded-loop-channel/plugin/registry"
)

// TestReResolveSelectsAlternativeChannel verifies L3: a degraded channel is
// replaced by another READY channel for the same capability.
func TestReResolveSelectsAlternativeChannel(t *testing.T) {
	reg := registry.New()
	dev := domain.DeviceID("dev-1")

	degraded := domain.NewChannel("p1", "adb", "ep-1", dev)
	degraded.State = domain.ChannelDegraded
	degraded.Healthy = false
	degraded.Capabilities = []domain.CapabilityName{domain.CapabilityLog}
	reg.AddChannel(degraded)

	ready := domain.NewChannel("p2", "uart", "ep-2", dev)
	ready.State = domain.ChannelReady
	ready.Healthy = true
	ready.Capabilities = []domain.CapabilityName{domain.CapabilityLog}
	reg.AddChannel(ready)

	r := resolver.New(reg)
	m := NewManager(pluginregistry.New(), event.New(), DefaultBudget()).WithResolver(r)

	alt := m.reResolve(degraded, domain.CapabilityLog)
	if alt == nil || alt.ID != ready.ID {
		t.Fatalf("expected re-resolve to %s, got %v", ready.ID, alt)
	}
}

// TestReResolveNoAlternative verifies L3 returns nil when no other channel
// supports the capability.
func TestReResolveNoAlternative(t *testing.T) {
	reg := registry.New()
	dev := domain.DeviceID("dev-1")

	only := domain.NewChannel("p1", "adb", "ep-1", dev)
	only.State = domain.ChannelReady
	only.Capabilities = []domain.CapabilityName{domain.CapabilityLog}
	reg.AddChannel(only)

	r := resolver.New(reg)
	m := NewManager(pluginregistry.New(), event.New(), DefaultBudget()).WithResolver(r)

	if alt := m.reResolve(only, domain.CapabilityLog); alt != nil {
		t.Fatalf("expected nil (no alternative), got %v", alt.ID)
	}
}

// TestRecoveryLadderExhaustsToManual verifies the ladder terminates at L6
// MANUAL when reconnect, re-resolve and device recovery all fail (no resolver
// or discovery wired, device offline).
func TestRecoveryLadderExhaustsToManual(t *testing.T) {
	m := NewManager(pluginregistry.New(), event.New(), Budget{
		MaxAttempts:     1,
		MaxDeviceResets: 0, // disable device recovery so we fall through to manual
		BaseBackoff:     time.Millisecond,
	})

	ch := domain.NewChannel("p1", "adb", "ep-1", "dev-1")
	ch.State = domain.ChannelReconnecting

	derr := domain.NewError(domain.CodeChannelLost, domain.CategoryConnection, "channel lost")
	rr := m.Recover(context.Background(), ch, domain.CapabilityLog, derr)

	if rr.Recovered {
		t.Fatalf("expected no recovery (manual), got %+v", rr)
	}
	if rr.Level != L6ManualIntervention || rr.State != StateManual {
		t.Fatalf("expected L6 MANUAL, got level=%s state=%s", rr.Level, rr.State)
	}
}
