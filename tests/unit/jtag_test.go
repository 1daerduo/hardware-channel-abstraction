package unit

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/1daerduo/hardware-channel-abstraction/domain"
	"github.com/1daerduo/hardware-channel-abstraction/fake"
	"github.com/1daerduo/hardware-channel-abstraction/runtime"
)

// TestJTAGDebug verifies the debug control plane (halt/read memory/write
// memory) plugs in through the SAME Unified API.
func TestJTAGDebug(t *testing.T) {
	ctx := context.Background()
	rt := runtime.Bootstrap(runtime.WithDevices(
		fake.NewDevice("DEV-1", "board", "1.0", "usb:1-1.1").WithJTAGLocator("jtag:1"),
	))
	defer rt.Close()
	for _, cap := range []domain.CapabilityName{
		domain.CapabilityDebugHalt, domain.CapabilityDebugResume,
		domain.CapabilityDebugReadMemory, domain.CapabilityDebugWriteMemory,
	} {
		rt.Client.Grant("agent", cap)
		rt.Client.PreApprove("agent", cap)
	}

	devices, _ := rt.Client.Discover(ctx)
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	d := devices[0]
	caps, _ := rt.Client.ListCapabilities(d.ID)
	if !containsName(caps, domain.CapabilityDebugReadMemory) {
		t.Fatalf("debug.read_memory missing from %v", caps)
	}

	sess, _ := rt.Client.CreateSession("agent", d.ID, time.Minute)

	// halt
	if res, err := rt.Client.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityDebugHalt, Target: d.ID, SessionID: sess.ID,
	}); err != nil || res.State != domain.OperationSucceeded {
		t.Fatalf("halt: %v %v", res.State, err)
	}

	// read memory @ 0x20000000 → initial 0xdeadbeef
	res, err := rt.Client.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityDebugReadMemory, Target: d.ID, SessionID: sess.ID,
		Parameters: map[string]string{"address": "0x20000000", "count": "1"},
	})
	if err != nil || res.State != domain.OperationSucceeded {
		t.Fatalf("read_memory: %v %v", res.State, err)
	}
	if !strings.Contains(res.Output, "0xdeadbeef") {
		t.Fatalf("read_memory output = %q, want 0xdeadbeef", res.Output)
	}

	// write memory then read back
	if _, err := rt.Client.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityDebugWriteMemory, Target: d.ID, SessionID: sess.ID,
		Parameters: map[string]string{"address": "0x20000000", "values": "0x12345678"},
	}); err != nil {
		t.Fatalf("write_memory: %v", err)
	}
	res, _ = rt.Client.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityDebugReadMemory, Target: d.ID, SessionID: sess.ID,
		Parameters: map[string]string{"address": "0x20000000", "count": "1"},
	})
	if !strings.Contains(res.Output, "0x12345678") {
		t.Fatalf("after write, read_memory output = %q, want 0x12345678", res.Output)
	}
}

func containsName(names []domain.CapabilityName, want domain.CapabilityName) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}
