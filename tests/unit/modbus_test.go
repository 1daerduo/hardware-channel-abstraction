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

// TestModbus verifies a structured (non-byte-stream) register protocol plugs
// in through the SAME Unified API.
func TestModbus(t *testing.T) {
	ctx := context.Background()
	srv, err := fake.NewModbusServer("127.0.0.1:0")
	if err != nil {
		t.Fatalf("modbus server: %v", err)
	}
	defer srv.Close()

	rt := runtime.Bootstrap(runtime.WithModbusDevice(srv.Addr()))
	defer rt.Close()
	rt.Client.Grant("agent", domain.CapabilityModbusReadHolding)
	rt.Client.Grant("agent", domain.CapabilityModbusReadInput)
	rt.Client.Grant("agent", domain.CapabilityModbusWriteReg)
	rt.Client.PreApprove("agent", domain.CapabilityModbusWriteReg)

	devices, _ := rt.Client.Discover(ctx)
	if len(devices) != 1 {
		t.Fatalf("expected 1 modbus device, got %d", len(devices))
	}
	d := devices[0]
	sess, _ := rt.Client.CreateSession("agent", d.ID, time.Minute)

	// read holding register 0 → 0x1234
	res, err := rt.Client.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityModbusReadHolding, Target: d.ID, SessionID: sess.ID,
		Parameters: map[string]string{"address": "0", "quantity": "3"},
	})
	if err != nil || res.State != domain.OperationSucceeded {
		t.Fatalf("read_holding: %v %v", res.State, err)
	}
	if !strings.Contains(res.Output, "0x1234") || !strings.Contains(res.Output, "0x5678") {
		t.Fatalf("read_holding output = %q", res.Output)
	}

	// write register 0 = 0xabcd, then verify server state
	if _, err := rt.Client.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityModbusWriteReg, Target: d.ID, SessionID: sess.ID,
		Parameters: map[string]string{"address": "0", "value": "0xabcd"},
	}); err != nil {
		t.Fatalf("write_reg: %v", err)
	}
	if got := srv.Holding(0); got != 0xabcd {
		t.Fatalf("holding[0] = 0x%04x, want 0xabcd", got)
	}
}
