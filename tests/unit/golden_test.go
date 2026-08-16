package unit

import (
	"context"
	"testing"
	"time"

	"github.com/1daerduo/hardware-channel-abstraction/core/event"
	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/fake"
	"github.com/1daerduo/hardware-channel-abstraction/runtime"
)

// TestGoldenPath runs the reference flow end-to-end and asserts the
// invariants: discovery, session, info/reboot, flash approval gate, and
// deny-by-default.
func TestGoldenPath(t *testing.T) {
	ctx := context.Background()
	rt := runtime.Bootstrap(runtime.WithDevices(
		fake.NewDevice("ABC123", "eval-board", "1.2.3", "usb:1-1.2"),
	))
	c := rt.Client

	c.Grant("agent", domain.CapabilityInfoGet)
	c.Grant("agent", domain.CapabilityReboot)
	c.Grant("agent", domain.CapabilityFlash)

	devices, err := c.Discover(ctx)
	if err != nil || len(devices) != 1 {
		t.Fatalf("discovery failed: devices=%d err=%v", len(devices), err)
	}
	dev := devices[0]

	sess, err := c.CreateSession("agent", dev.ID, time.Minute)
	if err != nil {
		t.Fatalf("session failed: %v", err)
	}

	// info.get → SUCCEEDED with evidence.
	info, err := c.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityInfoGet, Target: dev.ID, SessionID: sess.ID,
	})
	if err != nil || info.State != domain.OperationSucceeded {
		t.Fatalf("info.get failed: state=%v err=%v", info.State, err)
	}
	if len(info.EvidenceRefs) == 0 {
		t.Fatalf("info.get should produce evidence")
	}

	// flash → approval required before approval; SUCCESS after.
	flashReq := domain.OperationRequest{
		Capability: domain.CapabilityFlash, Target: dev.ID, SessionID: sess.ID,
		Parameters: map[string]string{"partition": "boot", "image": "b.img", "version": "2.0.0"},
	}
	first, err := c.Execute(ctx, flashReq)
	if err == nil || first.State != domain.OperationValidationFailed {
		t.Fatalf("flash should be approval-gated before approval: state=%v err=%v", first.State, err)
	}
	c.PreApprove("agent", domain.CapabilityFlash)
	flash, err := c.Execute(ctx, flashReq)
	if err != nil || flash.State != domain.OperationSucceeded {
		t.Fatalf("flash after approval failed: state=%v err=%v", flash.State, err)
	}
	// Evidence should include the flash version postcondition.
	foundVersion := false
	for _, id := range flash.EvidenceRefs {
		ev, ok := c.GetEvidence(id)
		if !ok {
			continue
		}
		if ev.Name == "flash.version" && ev.Value == "2.0.0" {
			foundVersion = true
		}
	}
	if !foundVersion {
		t.Fatalf("flash evidence missing version postcondition")
	}

	// deny-by-default: a viewer with no grants cannot execute.
	vs, _ := c.CreateSession("viewer", dev.ID, time.Minute)
	execRes, err := c.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityExecute, Target: dev.ID, SessionID: vs.ID,
		Parameters: map[string]string{"command": "echo hi"},
	})
	if err == nil || execRes.Error == nil || execRes.Error.Category != domain.CategoryAuthorization {
		t.Fatalf("execute should be denied for viewer: %v", execRes.Error)
	}
}

// TestIdempotency verifies that a repeated idempotency_key does not create a
// duplicate operation.
func TestIdempotency(t *testing.T) {
	ctx := context.Background()
	rt := runtime.Bootstrap(runtime.WithDevices(
		fake.NewDevice("ABC123", "eval-board", "1.2.3", "usb:1-1.2"),
	))
	c := rt.Client
	c.Grant("agent", domain.CapabilityInfoGet)

	devices, _ := c.Discover(ctx)
	dev := devices[0]
	sess, _ := c.CreateSession("agent", dev.ID, time.Minute)

	req := domain.OperationRequest{
		Capability: domain.CapabilityInfoGet, Target: dev.ID, SessionID: sess.ID,
		IdempotencyKey: "key-1",
	}
	op1, err := c.CreateOperation(req)
	if err != nil {
		t.Fatalf("create op1: %v", err)
	}
	op2, err := c.CreateOperation(req)
	if err != nil {
		t.Fatalf("create op2: %v", err)
	}
	if op1.ID != op2.ID {
		t.Fatalf("same idempotency_key must return the same operation: %s != %s", op1.ID, op2.ID)
	}
}

// TestMultiChannelResolution verifies the acceptance criterion: one capability
// provided by two different protocol channels is auto-selected deterministically
// (cheaper wins), and a channel override can pin a specific protocol.
func TestMultiChannelResolution(t *testing.T) {
	ctx := context.Background()
	rt := runtime.Bootstrap(runtime.WithDevices(
		fake.NewDevice("ABC123", "eval-board", "1.2.3", "usb:1-1.2").
			WithSerialPort("ttyUSB0"),
	))
	c := rt.Client
	c.Grant("agent", domain.CapabilityLog)

	devices, err := c.Discover(ctx)
	if err != nil || len(devices) != 1 {
		t.Fatalf("expected 1 correlated device, got %d (err=%v)", len(devices), err)
	}
	if n := len(rt.Registry.ChannelsByDevice(devices[0].ID)); n != 2 {
		t.Fatalf("expected 2 channels for one board, got %d", n)
	}
	sess, _ := c.CreateSession("agent", devices[0].ID, time.Minute)

	evidenceValue := func(r *domain.OperationResult) string {
		for _, id := range r.EvidenceRefs {
			if ev, ok := c.GetEvidence(id); ok && ev.Name == "log.source" {
				return ev.Value
			}
		}
		return ""
	}

	// Auto-select: UART (cost 5) beats ADB (cost 10).
	auto, err := c.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityLog, Target: devices[0].ID, SessionID: sess.ID,
	})
	if err != nil || auto.State != domain.OperationSucceeded {
		t.Fatalf("device.log auto failed: %v", err)
	}
	if v := evidenceValue(auto); v != "uart" {
		t.Fatalf("expected UART to win device.log, got source=%q", v)
	}

	// Channel override pins ADB.
	override, err := c.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityLog, Target: devices[0].ID, SessionID: sess.ID,
		ChannelType: "adb",
	})
	if err != nil || override.State != domain.OperationSucceeded {
		t.Fatalf("device.log override failed: %v", err)
	}
	if v := evidenceValue(override); v != "adb" {
		t.Fatalf("expected ADB via override, got source=%q", v)
	}
}

// TestThreeProtocolResolution verifies three protocols (ADB, UART, MCP) plug
// in without Core change, and the Resolver ranks them deterministically for an
// overlapping capability (device.info.get: ADB cost 10 < MCP cost 20).
func TestThreeProtocolResolution(t *testing.T) {
	ctx := context.Background()
	rt := runtime.Bootstrap(runtime.WithDevices(
		fake.NewDevice("ABC123", "eval-board", "1.2.3", "usb:1-1.2").
			WithSerialPort("ttyUSB0").
			WithMCPURL("mcp://ABC123:8080"),
	))
	c := rt.Client
	c.Grant("agent", domain.CapabilityInfoGet)

	devices, err := c.Discover(ctx)
	if err != nil || len(devices) != 1 {
		t.Fatalf("expected 1 correlated device, got %d", len(devices))
	}
	if n := len(rt.Registry.ChannelsByDevice(devices[0].ID)); n != 3 {
		t.Fatalf("expected 3 channels (adb+uart+mcp), got %d", n)
	}
	sess, _ := c.CreateSession("agent", devices[0].ID, time.Minute)

	// Auto-select: ADB (10) beats MCP (20); neither provides info via UART.
	auto, err := c.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityInfoGet, Target: devices[0].ID, SessionID: sess.ID,
	})
	if err != nil || auto.State != domain.OperationSucceeded {
		t.Fatalf("info.get auto failed: %v", err)
	}
	if hasEvidence(auto, c, "mcp.tool") {
		t.Fatalf("auto-select should prefer ADB, not MCP")
	}

	// Channel override pins MCP.
	ovr, err := c.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityInfoGet, Target: devices[0].ID, SessionID: sess.ID,
		ChannelType: "mcp",
	})
	if err != nil || ovr.State != domain.OperationSucceeded {
		t.Fatalf("info.get via mcp override failed: %v", err)
	}
	if !hasEvidence(ovr, c, "mcp.tool") {
		t.Fatalf("expected mcp.tool evidence via override")
	}
}

// hasEvidence reports whether the result carries evidence with the given name.
func hasEvidence(r *domain.OperationResult, c interface {
	GetEvidence(domain.EvidenceID) (*domain.Evidence, bool)
}, name string) bool {
	for _, id := range r.EvidenceRefs {
		if ev, ok := c.GetEvidence(id); ok && ev.Name == name {
			return true
		}
	}
	return false
}

// TestRecoveryPowerCycle verifies the full recovery ladder: a device that
// goes offline mid-run is recovered via L5 device recovery (power cycle) and
// the operation completes successfully.
func TestRecoveryPowerCycle(t *testing.T) {
	ctx := context.Background()
	dev := fake.NewDevice("ABC123", "eval-board", "1.2.3", "usb:1-1.2")
	rt := runtime.Bootstrap(runtime.WithDevices(dev))
	c := rt.Client
	c.Grant("agent", domain.CapabilityReboot)

	devices, _ := c.Discover(ctx)
	sess, _ := c.CreateSession("agent", devices[0].ID, time.Minute)

	dev.SetOnline(false) // fault injection: device drops offline
	res, err := c.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityReboot, Target: devices[0].ID, SessionID: sess.ID,
	})
	if err != nil || res.State != domain.OperationSucceeded {
		t.Fatalf("expected recovery to power-cycle and succeed, got state=%v err=%v", res.State, err)
	}

	// The recovery ladder must have emitted recovery events.
	var sawRecovery bool
	for _, e := range c.Events(event.Filter{DeviceID: devices[0].ID}) {
		if e.Type == domain.EventRecoveryStarted || e.Type == domain.EventRecoveryCompleted {
			sawRecovery = true
		}
	}
	if !sawRecovery {
		t.Fatalf("expected recovery events in the stream")
	}
}
