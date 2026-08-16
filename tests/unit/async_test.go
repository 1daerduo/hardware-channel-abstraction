package unit

import (
	"context"
	"testing"
	"time"

	"example.com/embedded-loop-channel/domain"
	"example.com/embedded-loop-channel/fake"
	"example.com/embedded-loop-channel/runtime"
)

// TestAsyncOperation verifies the long-operation path: Start returns an
// operation handle immediately and Wait polls it to a terminal state.
func TestAsyncOperation(t *testing.T) {
	ctx := context.Background()
	rt := runtime.Bootstrap(runtime.WithDevices(
		fake.NewDevice("ABC123", "eval-board", "1.2.3", "usb:1-1.2"),
	))
	c := rt.Client
	c.Grant("agent", domain.CapabilityInfoGet)

	devices, _ := c.Discover(ctx)
	sess, _ := c.CreateSession("agent", devices[0].ID, time.Minute)

	op, err := c.Start(ctx, domain.OperationRequest{
		Capability: domain.CapabilityInfoGet, Target: devices[0].ID, SessionID: sess.ID,
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if op.CurrentState().IsTerminal() {
		t.Fatalf("operation should not be terminal immediately after Start, got %s", op.CurrentState())
	}

	res, err := c.Wait(ctx, op.ID, 5*time.Millisecond)
	if err != nil {
		t.Fatalf("wait failed: %v", err)
	}
	if res == nil || res.State != domain.OperationSucceeded {
		t.Fatalf("expected SUCCEEDED, got %+v", res)
	}
}

// TestCancelBeforeExecute verifies cooperative cancellation: cancelling a
// created operation makes it transition to CANCELLED when executed.
func TestCancelBeforeExecute(t *testing.T) {
	ctx := context.Background()
	rt := runtime.Bootstrap(runtime.WithDevices(
		fake.NewDevice("ABC123", "eval-board", "1.2.3", "usb:1-1.2"),
	))
	c := rt.Client
	c.Grant("agent", domain.CapabilityInfoGet)

	devices, _ := c.Discover(ctx)
	sess, _ := c.CreateSession("agent", devices[0].ID, time.Minute)

	op, err := c.CreateOperation(domain.OperationRequest{
		Capability: domain.CapabilityInfoGet, Target: devices[0].ID, SessionID: sess.ID,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if err := c.Cancel(ctx, op.ID); err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	res, err := c.ExecuteOperation(ctx, op.ID)
	if err == nil {
		t.Fatalf("expected a cancellation error, got %v", res.State)
	}
	if res.State != domain.OperationCancelled {
		t.Fatalf("expected CANCELLED, got %s", res.State)
	}
}

// TestCancelTerminalIsRejected verifies a terminal operation cannot be
// cancelled (NOT_CANCELLABLE).
func TestCancelTerminalIsRejected(t *testing.T) {
	ctx := context.Background()
	rt := runtime.Bootstrap(runtime.WithDevices(
		fake.NewDevice("ABC123", "eval-board", "1.2.3", "usb:1-1.2"),
	))
	c := rt.Client
	c.Grant("agent", domain.CapabilityInfoGet)

	devices, _ := c.Discover(ctx)
	sess, _ := c.CreateSession("agent", devices[0].ID, time.Minute)
	res, err := c.Execute(ctx, domain.OperationRequest{
		Capability: domain.CapabilityInfoGet, Target: devices[0].ID, SessionID: sess.ID,
	})
	if err != nil || res == nil || !res.State.IsTerminal() {
		t.Fatalf("expected terminal result, got %+v err=%v", res, err)
	}
	if cerr := c.Cancel(ctx, res.OperationID); cerr == nil {
		t.Fatalf("expected NOT_CANCELLABLE on a terminal operation")
	}
}
