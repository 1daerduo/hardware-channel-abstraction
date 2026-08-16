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

// TestTCPDeviceEndToEnd verifies a simulated TCP device plugs in through the
// SAME Unified API with zero Core change — the "portable extension" promise.
func TestTCPDeviceEndToEnd(t *testing.T) {
	ctx := context.Background()

	// Simulated TCP console device.
	dev, err := fake.NewConsoleServer("127.0.0.1:0")
	if err != nil {
		t.Fatalf("start tcp server: %v", err)
	}
	defer dev.Close()

	rt := runtime.Bootstrap(runtime.WithTCPDevice(dev.Addr()))
	defer rt.Close()
	c := rt.Client
	c.Grant("agent", domain.CapabilityInfoGet)
	c.PreApprove("agent", domain.CapabilityInfoGet)

	devices, err := c.Discover(ctx)
	if err != nil || len(devices) != 1 {
		t.Fatalf("discover: devices=%d err=%v", len(devices), err)
	}
	if devices[0].Model != "tcp-device" {
		t.Fatalf("device model = %q, want %q", devices[0].Model, "tcp-device")
	}

	sess, _ := c.CreateSession("agent", devices[0].ID, time.Minute)
	res, err := c.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityInfoGet, Target: devices[0].ID, SessionID: sess.ID,
	})
	if err != nil || res.State != domain.OperationSucceeded {
		t.Fatalf("info.get: state=%v err=%v", res.State, err)
	}
	if !strings.Contains(res.Output, "TCP-Device") {
		t.Fatalf("info.get output missing device info: %q", res.Output)
	}
}

// TestTCPExecuteEcho verifies device.execute round-trips a command over TCP.
func TestTCPExecuteEcho(t *testing.T) {
	ctx := context.Background()
	dev, _ := fake.NewConsoleServer("127.0.0.1:0")
	defer dev.Close()

	rt := runtime.Bootstrap(runtime.WithTCPDevice(dev.Addr()))
	defer rt.Close()
	c := rt.Client
	c.Grant("agent", domain.CapabilityExecute)
	c.PreApprove("agent", domain.CapabilityExecute)

	devices, _ := c.Discover(ctx)
	sess, _ := c.CreateSession("agent", devices[0].ID, time.Minute)

	res, err := c.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityExecute, Target: devices[0].ID, SessionID: sess.ID,
		Parameters: map[string]string{"command": "echo hello-over-tcp"},
	})
	if err != nil || res.State != domain.OperationSucceeded {
		t.Fatalf("execute: state=%v err=%v", res.State, err)
	}
	if res.Output != "hello-over-tcp" {
		t.Fatalf("echo output = %q, want %q", res.Output, "hello-over-tcp")
	}
}
